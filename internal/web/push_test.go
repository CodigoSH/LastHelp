package web

import (
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/account"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/config"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/notify"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/webpush"
)

// newPushInstance is an instance whose Server a test can reach, because what is
// under test is who a message is sent to and that is not visible over HTTP.
func newPushInstance(t *testing.T) (*instance, *Server) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	db, err := database.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	srv, err := New(config.Config{DBFile: dbPath, KeyFile: filepath.Join(filepath.Dir(dbPath), "test.key")}, db)
	if err != nil {
		t.Fatal(err)
	}
	clock := &testClock{at: time.Unix(1_700_000_000, 0)}
	srv.Clock = clock.now
	httpSrv := httptest.NewServer(srv)
	t.Cleanup(httpSrv.Close)
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	return &instance{t: t, server: httpSrv, client: &http.Client{Jar: jar}, dbHandle: db, clock: srv.now, tick: clock}, srv
}

// subscription is what a browser sends: a real P-256 point and 16 bytes of secret.
func subscription(t *testing.T, endpoint string) map[string]any {
	t.Helper()
	key, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	secret := make([]byte, 16)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	return map[string]any{
		"endpoint": endpoint,
		"keys":     map[string]string{"p256dh": webpush.Encode(key.PublicKey().Bytes()), "auth": webpush.Encode(secret)},
		"label":    "Test browser",
	}
}

// pushService is a push service of the test's own, which records what reaches it.
type pushService struct {
	*httptest.Server
	mu       sync.Mutex
	paths    []string
	reply    int
	lastAuth string
}

func newPushService(t *testing.T) *pushService {
	t.Helper()
	p := &pushService{reply: http.StatusCreated}
	p.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p.mu.Lock()
		p.paths = append(p.paths, r.URL.Path)
		p.lastAuth = r.Header.Get("Authorization")
		code := p.reply
		p.mu.Unlock()
		w.WriteHeader(code)
	}))
	t.Cleanup(p.Close)
	// A push service is reached over https, and the sender signs for an https
	// address, so the test's is one too. The gate the sender sends through uses
	// the default transport, which is made to trust this service's certificate.
	wasTransport := http.DefaultTransport
	http.DefaultTransport = p.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = wasTransport })
	// The real rule only lets a push service's own domain through; the test's
	// service is on loopback.
	was := pushEndpointOK
	pushEndpointOK = func(string) bool { return true }
	t.Cleanup(func() { pushEndpointOK = was })
	return p
}

func (p *pushService) reached() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.paths...)
}

func (p *pushService) setReply(code int) {
	p.mu.Lock()
	p.reply = code
	p.mu.Unlock()
}

func (i *instance) signInAs(username string) *instance {
	i.t.Helper()
	c := i.anonymous()
	resp := c.do(http.MethodPost, "/api/session", map[string]string{"username": username, "password": "a good long passphrase"})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		i.t.Fatalf("%s could not sign in: %d", username, resp.StatusCode)
	}
	return c
}

func grantOnBoard(t *testing.T, team *instance, username, boardSlug string) {
	t.Helper()
	accounts := account.Store{DB: team.db()}
	user, err := accounts.Create(username, "a good long passphrase", false, team.now())
	if err != nil {
		t.Fatal(err)
	}
	if boardSlug == "" {
		return
	}
	var id int64
	if err := team.db().QueryRow(`SELECT id FROM repository WHERE slug = ?`, boardSlug).Scan(&id); err != nil {
		t.Fatal(err)
	}
	if err := accounts.Grant(user.ID, authz.Repository(id, 0), authz.RoleCollaborator, team.now()); err != nil {
		t.Fatal(err)
	}
}

func TestOnlyAPushServicesOwnAddressIsSentTo(t *testing.T) {
	for endpoint, want := range map[string]bool{
		"https://fcm.googleapis.com/fcm/send/abc":              true,
		"https://updates.push.services.mozilla.com/wpush/v2/a": true,
		"https://web.push.apple.com/abc":                       true,
		"https://wns2-par02p.notify.windows.com/w/?token=x":    true,
		"http://fcm.googleapis.com/fcm/send/abc":               false, // not https
		"https://fcm.googleapis.com:8443/fcm/send/abc":         false, // not the usual port
		"https://user:pw@fcm.googleapis.com/fcm/send/abc":      false, // credentials
		"https://fcm.googleapis.com.evil.example/x":            false, // a lookalike
		"https://evilfcm.googleapis.com/x":                     false, // another host
		"https://push.apple.com/x":                             false, // a suffix rule needs a host in front
		"https://127.0.0.1/x":                                  false,
		"https://example.org/x":                                false,
		"":                                                     false,
	} {
		if got := allowedPushEndpoint(endpoint); got != want {
			t.Errorf("allowedPushEndpoint(%q) = %v, want %v", endpoint, got, want)
		}
	}
}

func TestABrowserIsSubscribedListedAndRemovedByItsOwner(t *testing.T) {
	team, _ := newPushInstance(t)
	team.claim()
	grantOnBoard(t, team, "other", "")
	other := team.signInAs("other")

	// An address that is not a push service's is refused, so a subscription is
	// never a way to make this server request something.
	resp := team.do(http.MethodPost, "/api/push/subscriptions", subscription(t, "https://example.org/hook"))
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("an address outside the push services returned %d", resp.StatusCode)
	}
	// Keys that are not a point on the curve.
	broken := subscription(t, "https://fcm.googleapis.com/fcm/send/x")
	broken["keys"] = map[string]string{"p256dh": "AAAA", "auth": "AAAA"}
	resp = team.do(http.MethodPost, "/api/push/subscriptions", broken)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("a made-up key returned %d", resp.StatusCode)
	}

	endpoint := "https://fcm.googleapis.com/fcm/send/mine"
	resp = team.do(http.MethodPost, "/api/push/subscriptions", subscription(t, endpoint))
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("subscribing returned %d", resp.StatusCode)
	}
	// Subscribing again is the same browser, not a second one.
	resp = team.do(http.MethodPost, "/api/push/subscriptions", subscription(t, endpoint))
	_ = resp.Body.Close()

	body := decode(t, team.do(http.MethodGet, "/api/push/subscriptions", nil))
	devices, _ := body["devices"].([]any)
	if len(devices) != 1 {
		t.Fatalf("the account has %d devices after subscribing the same browser twice, want 1", len(devices))
	}
	device, _ := devices[0].(map[string]any)
	if device["ref"] != pushRef(endpoint) {
		t.Errorf("the device is named %v, want %s", device["ref"], pushRef(endpoint))
	}
	for k := range device {
		if k == "endpoint" || k == "p256dh" || k == "auth" {
			t.Errorf("the list gives back %q, which is what lets a message reach the device", k)
		}
	}

	// Another account sees none of it and cannot remove it.
	if got, _ := decode(t, other.do(http.MethodGet, "/api/push/subscriptions", nil))["devices"].([]any); len(got) != 0 {
		t.Errorf("another account sees %d of somebody else's devices", len(got))
	}
	id := strconv.Itoa(int(device["id"].(float64)))
	if code := other.status(http.MethodDelete, "/api/push/subscriptions/"+id); code != http.StatusNotFound {
		t.Errorf("removing somebody else's device returned %d, want 404", code)
	}
	if code := team.status(http.MethodDelete, "/api/push/subscriptions/"+id); code != http.StatusOK {
		t.Errorf("removing one's own device returned %d", code)
	}

	// A cap, so a loop in a script cannot fill the table.
	for n := 0; n < maxPushDevices; n++ {
		resp = team.do(http.MethodPost, "/api/push/subscriptions", subscription(t, "https://fcm.googleapis.com/fcm/send/n"+strconv.Itoa(n)))
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("device %d returned %d", n, resp.StatusCode)
		}
	}
	resp = team.do(http.MethodPost, "/api/push/subscriptions", subscription(t, "https://fcm.googleapis.com/fcm/send/one-too-many"))
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusConflict {
		t.Errorf("a device past the cap returned %d, want 409", resp.StatusCode)
	}
}

// What a board's channel reaches is its team's browsers, and nobody else's.
func TestAPushIsSentToTheTeamOfTheBoardAndNobodyElse(t *testing.T) {
	team, srv := newPushInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")
	team.createRepo("codigosh", "lastdav")
	grantOnBoard(t, team, "helper", "lastboard")  // works on the board
	grantOnBoard(t, team, "neighbour", "lastdav") // works on the other
	grantOnBoard(t, team, "bystander", "")        // no role anywhere

	service := newPushService(t)
	for _, who := range []string{"admin", "helper", "neighbour", "bystander"} {
		c := team
		if who != "admin" {
			c = team.signInAs(who)
		}
		resp := c.do(http.MethodPost, "/api/push/subscriptions", subscription(t, service.URL+"/"+who))
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s could not subscribe: %d", who, resp.StatusCode)
		}
	}

	var board int64
	if err := team.db().QueryRow(`SELECT id FROM repository WHERE slug = 'lastboard'`).Scan(&board); err != nil {
		t.Fatal(err)
	}
	if err := srv.pushTo(context.Background(), notify.Message{Title: "lastboard: idea", Body: "dark mode", Repo: board}); err != nil {
		t.Fatalf("telling the team failed: %v", err)
	}

	got := map[string]bool{}
	for _, p := range service.reached() {
		got[strings.TrimPrefix(p, "/")] = true
	}
	for _, who := range []string{"admin", "helper"} {
		if !got[who] {
			t.Errorf("%s works on the board and was not told", who)
		}
	}
	for _, who := range []string{"neighbour", "bystander"} {
		if got[who] {
			t.Errorf("%s does not work on the board and was told", who)
		}
	}
	service.mu.Lock()
	signed := strings.HasPrefix(service.lastAuth, "vapid t=")
	service.mu.Unlock()
	if !signed {
		t.Errorf("the push was not signed as VAPID: %q", service.lastAuth)
	}

	// Somebody the administrator disabled is not told either.
	if _, err := team.db().Exec(`UPDATE app_user SET disabled = 1 WHERE username = 'helper'`); err != nil {
		t.Fatal(err)
	}
	before := len(service.reached())
	if err := srv.pushTo(context.Background(), notify.Message{Title: "x", Body: "y", Repo: board}); err != nil {
		t.Fatal(err)
	}
	for _, p := range service.reached()[before:] {
		if strings.HasSuffix(p, "/helper") {
			t.Error("a disabled account was told")
		}
	}
}

func TestADeviceThePushServiceSaysIsGoneIsForgotten(t *testing.T) {
	team, srv := newPushInstance(t)
	team.claim()
	service := newPushService(t)
	service.setReply(http.StatusGone)

	resp := team.do(http.MethodPost, "/api/push/subscriptions", subscription(t, service.URL+"/old-phone"))
	_ = resp.Body.Close()
	var me int64
	if err := team.db().QueryRow(`SELECT id FROM app_user`).Scan(&me); err != nil {
		t.Fatal(err)
	}
	// Gone is not a failure to retry: the device is forgotten, so an uninstalled
	// app stops costing a request every time.
	if err := srv.pushTo(context.Background(), notify.Message{Title: "t", Body: "b", User: me}); err != nil {
		t.Fatalf("a device that is gone is not an error to retry, got %v", err)
	}
	var left int
	if err := team.db().QueryRow(`SELECT COUNT(*) FROM push_subscription`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Errorf("%d subscriptions left after the service said 410", left)
	}

	// A refusal that is not "gone" is kept and reported, so the queue tries again.
	service.setReply(http.StatusInternalServerError)
	resp = team.do(http.MethodPost, "/api/push/subscriptions", subscription(t, service.URL+"/new-phone"))
	_ = resp.Body.Close()
	if err := srv.pushTo(context.Background(), notify.Message{Title: "t", Body: "b", User: me}); err == nil {
		t.Error("a push service that refused every device reported success")
	}
	if err := team.db().QueryRow(`SELECT COUNT(*) FROM push_subscription`).Scan(&left); err != nil || left != 1 {
		t.Errorf("a device the service refused once was forgotten: %d left (%v)", left, err)
	}
}

func TestNobodyBeingSubscribedIsNotAnErrorForAnAnnouncementButIsForATest(t *testing.T) {
	team, srv := newPushInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")
	var board, me int64
	if err := team.db().QueryRow(`SELECT id FROM repository`).Scan(&board); err != nil {
		t.Fatal(err)
	}
	if err := team.db().QueryRow(`SELECT id FROM app_user`).Scan(&me); err != nil {
		t.Fatal(err)
	}
	// An announcement retried twelve times because nobody joined the channel yet
	// would end up in "needs you" for a choice that is nobody's mistake.
	if err := srv.pushTo(context.Background(), notify.Message{Title: "t", Body: "b", Repo: board}); err != nil {
		t.Errorf("an announcement to a team with no devices failed: %v", err)
	}
	if err := srv.pushTo(context.Background(), notify.Message{Title: "t", Body: "b", User: me}); err != errNoPushDevices {
		t.Errorf("a test with no devices returned %v, want errNoPushDevices", err)
	}
	// And over HTTP it says so in words the page has.
	resp := team.do(http.MethodPost, "/api/push/test", nil)
	body := decode(t, resp)
	if resp.StatusCode != http.StatusBadRequest || body["error"] != "error.push_no_devices" {
		t.Errorf("the test button with no devices returned %d %v", resp.StatusCode, body)
	}
}

func TestAPushChannelNeedsNothingTypedIn(t *testing.T) {
	team, _ := newPushInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")

	resp := team.do(http.MethodPost, "/api/orgs/codigosh/repos/lastboard/channel", map[string]any{"kind": "push", "url": "https://junk.example", "token": "x"})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("choosing push returned %d", resp.StatusCode)
	}
	var stored string
	if err := team.db().QueryRow(`SELECT notify_config FROM repository`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if cfg := notify.Parse(stored); cfg.Kind != notify.Push || cfg.URL != "" || cfg.Token != "" {
		t.Errorf("stored %q: a push channel keeps nothing but its kind", stored)
	}
	if !notify.Parse(stored).Active() {
		t.Error("a push channel is not a complete choice")
	}
}

// TestAPushIsWrittenInTheLanguageOfWhoReadsIt.
//
// A channel shared by a team cannot choose a language, but a push goes to one
// person's browser, and the server knows what language that account reads.
func TestAPushIsWrittenInTheLanguageOfWhoReadsIt(t *testing.T) {
	_, srv := newPushInstance(t)

	cases := []struct {
		lang, kind, want string
	}{
		{"en", "idea", "Atlas: Idea"},
		{"es", "idea", "Atlas: Idea"},
		{"es", "bug", "Atlas: Fallo"},
		{"de", "bug", "Atlas: Fehler"},
		{"es", "comment", "Atlas: Comentario pendiente de revisión"},
	}
	for _, c := range cases {
		got := srv.pushPayloadIn(c.lang, notify.Message{Title: "Atlas", Body: "dark mode", Kind: c.kind}).Title
		if got != c.want {
			t.Errorf("%s/%s reads %q, want %q", c.lang, c.kind, got, c.want)
		}
	}
	if es, de := srv.pushPayloadIn("es", notify.Message{Title: "A", Kind: "bug"}).Title, srv.pushPayloadIn("de", notify.Message{Title: "A", Kind: "bug"}).Title; es == de {
		t.Errorf("two languages read the same notification: %q", es)
	}
	// What was written for a test is left as its writer wrote it.
	if got := srv.pushPayloadIn("es", notify.Message{Title: "LastHelp: Prueba", Body: "b"}).Title; got != "LastHelp: Prueba" {
		t.Errorf("a message with no kind was rewritten: %q", got)
	}
	// A name written the other way is kept from dragging the words around it.
	if got := srv.pushPayloadIn("ar", notify.Message{Title: "Atlas", Kind: "idea"}).Title; !strings.Contains(got, "\u2068Atlas\u2069") {
		t.Errorf("an Arabic notification does not isolate the name: %q", got)
	}
}

func TestTheLanguageOfAnAccountFallsBackToEnglish(t *testing.T) {
	team, srv := newPushInstance(t)
	team.claim()
	var me int64
	if err := team.db().QueryRow(`SELECT id FROM app_user`).Scan(&me); err != nil {
		t.Fatal(err)
	}
	set := func(code string) {
		t.Helper()
		if _, err := team.db().Exec(`UPDATE app_user SET language = ? WHERE id = ?`, code, me); err != nil {
			t.Fatal(err)
		}
	}
	for stored, want := range map[string]string{"es": "es", "": "en", "xx": "en", "../../etc/passwd": "en", "ES": "en"} {
		set(stored)
		if got := srv.languageOf(me); got != want {
			t.Errorf("an account that chose %q is told in %q, want %q", stored, got, want)
		}
	}
	// A code is checked before it is made into a path.
	if srv.words("../dist/index") != nil || srv.words("") != nil {
		t.Error("a language code that is not one was read as a file")
	}
	// A sentence a language lacks is English, and one English lacks is its key.
	if got := srv.say("en", "no.such.key", nil); got != "no.such.key" {
		t.Errorf("a missing key reads %q", got)
	}
}
