package web

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/settings"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/notify"
	"git.codigosh.com/CodigoSH/LastHelp/internal/version"
)

// releaseForge is the forge's release list, which says what the newest release is and how
// many times it was asked.
type releaseForge struct {
	srv  *httptest.Server
	mu   sync.Mutex
	tag  string
	asks int
}

func newReleaseForge(t *testing.T, tag string) *releaseForge {
	t.Helper()
	f := &releaseForge{tag: tag}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.asks++
		tag := f.tag
		f.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"tag_name":"`+tag+`","draft":false,"prerelease":false}`)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func (f *releaseForge) setTag(tag string) { f.mu.Lock(); f.tag = tag; f.mu.Unlock() }
func (f *releaseForge) asked() int        { f.mu.Lock(); defer f.mu.Unlock(); return f.asks }

// noticeReceiver is a webhook that keeps what was posted to it.
type noticeReceiver struct {
	srv    *httptest.Server
	mu     sync.Mutex
	bodies []string
	code   int
}

func newNoticeReceiver(t *testing.T) *noticeReceiver {
	t.Helper()
	r := &noticeReceiver{code: http.StatusOK}
	r.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		raw, _ := io.ReadAll(req.Body)
		r.mu.Lock()
		r.bodies = append(r.bodies, string(raw))
		code := r.code
		r.mu.Unlock()
		w.WriteHeader(code)
	}))
	t.Cleanup(r.srv.Close)
	return r
}

func (r *noticeReceiver) got() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.bodies...)
}
func (r *noticeReceiver) answer(code int) { r.mu.Lock(); r.code = code; r.mu.Unlock() }

// noticeSetup is an instance running v1.0.0 whose forge has a newer release, with a webhook as
// the server's channel and the notice on or off as asked.
func noticeSetup(t *testing.T, on bool) (*instance, *Server, *releaseForge, *noticeReceiver) {
	t.Helper()
	was := version.Current
	version.Current = "v1.0.0"
	t.Cleanup(func() { version.Current = was })

	team, srv := newPushInstance(t)
	team.claim()
	forge := newReleaseForge(t, "v1.1.0")
	srv.releaseAPI = forge.srv.URL
	hook := newNoticeReceiver(t)
	resp := team.do(http.MethodPut, "/api/notifications", map[string]any{"kind": "webhook", "url": hook.srv.URL, "updates": on})
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("setting the server's channel: %d", resp.StatusCode)
	}
	return team, srv, forge, hook
}

// A version is said once to the channel, and the next one again.
func TestAdministratorsAreToldOnceOfEachNewerVersion(t *testing.T) {
	_, srv, forge, hook := noticeSetup(t, true)

	srv.notifyUpdate(context.Background())
	srv.notifyUpdate(context.Background())
	got := hook.got()
	if len(got) != 1 {
		t.Fatalf("the channel was told %d times of the same version, want 1: %v", len(got), got)
	}
	for _, want := range []string{`"kind":"update"`, `"version":"v1.1.0"`, `"current_version":"v1.0.0"`} {
		if !strings.Contains(got[0], want) {
			t.Errorf("the message is missing %s: %s", want, got[0])
		}
	}

	forge.setTag("v1.2.0")
	srv.notifyUpdate(context.Background())
	if n := len(hook.got()); n != 2 {
		t.Errorf("a newer version after that was told %d times in all, want 2", n)
	}
}

// Off until somebody turns it on, and then the forge is not even asked.
func TestNobodyIsToldWhenNobodyAskedToBe(t *testing.T) {
	_, srv, forge, hook := noticeSetup(t, false)

	srv.notifyUpdate(context.Background())
	if n := len(hook.got()); n != 0 {
		t.Errorf("a channel that was set up for reports was told of a version: %v", hook.got())
	}
	if n := forge.asked(); n != 0 {
		t.Errorf("the forge was asked %d times for somebody who does not listen", n)
	}
}

// A version is marked as told only when the channel took it, so one that was down gets it later.
func TestAVersionIsNotMarkedToldWhenTheChannelRefusedIt(t *testing.T) {
	_, srv, _, hook := noticeSetup(t, true)
	hook.answer(http.StatusInternalServerError)

	srv.notifyUpdate(context.Background())
	if told, _ := srv.settings().Get(settings.UpdateNotified); told != "" {
		t.Fatalf("a version the channel refused was marked as told: %q", told)
	}

	hook.answer(http.StatusOK)
	srv.notifyUpdate(context.Background())
	if told, _ := srv.settings().Get(settings.UpdateNotified); told != "v1.1.0" {
		t.Errorf("after the channel came back the version told is %q, want v1.1.0", told)
	}
	if n := len(hook.got()); n != 2 {
		t.Errorf("the channel saw %d tries, want the refused one and the one that went through", n)
	}
}

// The version that is running, or an older one, is not news.
func TestTheVersionAlreadyRunningIsNotAnnounced(t *testing.T) {
	_, srv, forge, hook := noticeSetup(t, true)
	forge.setTag("v1.0.0")

	srv.notifyUpdate(context.Background())
	if n := len(hook.got()); n != 0 {
		t.Errorf("the running version was announced: %v", hook.got())
	}
}

// Through push it goes to the administrators' devices and nobody else's, in each one's language and
// without the urgency of a report; and with nobody subscribed it is not marked as told.
func TestTheNoticeThroughPushGoesToAdministratorsInTheirLanguage(t *testing.T) {
	team, srv, _, _ := noticeSetup(t, true)
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")
	grantOnBoard(t, team, "helper", "lastboard")
	resp := team.do(http.MethodPut, "/api/notifications", map[string]any{"kind": "push", "updates": true})
	_ = resp.Body.Close()

	// Nobody has a device yet: not told, and not marked.
	srv.notifyUpdate(context.Background())
	if told, _ := srv.settings().Get(settings.UpdateNotified); told != "" {
		t.Fatalf("marked as told with no device to tell: %q", told)
	}

	service := newPushService(t)
	for _, who := range []string{"owner", "helper"} {
		c := team
		if who != "owner" {
			c = team.signInAs(who)
		}
		r := c.do(http.MethodPost, "/api/push/subscriptions", subscription(t, service.URL+"/"+who))
		_ = r.Body.Close()
	}
	if r := team.do(http.MethodPost, "/api/me/language", map[string]string{"language": "de"}); r.StatusCode != http.StatusOK {
		t.Fatalf("language: %d", r.StatusCode)
	}

	srv.notifyUpdate(context.Background())
	reached := service.reached()
	if len(reached) != 1 || reached[0] != "/owner" {
		t.Fatalf("the notice reached %v, want only the administrator's device", reached)
	}
	if urgencies := service.urgenciesSeen(); urgencies[len(urgencies)-1] != "low" {
		t.Errorf("a notice of a version went with urgency %q, want low", urgencies[len(urgencies)-1])
	}
	if told, _ := srv.settings().Get(settings.UpdateNotified); told != "v1.1.0" {
		t.Errorf("not marked as told after it went out: %q", told)
	}

	payload := srv.pushPayloadIn("de", notify.Message{Kind: "update", Version: "v1.1.0", Current: "v1.0.0"})
	if !strings.Contains(payload.Title, "v1.1.0") || !strings.Contains(payload.Title, "verfügbar") {
		t.Errorf("the German title is %q", payload.Title)
	}
	if payload.Tag != "update" || payload.URL != "/panel/about" {
		t.Errorf("tag %q url %q", payload.Tag, payload.URL)
	}
}

// Only an administrator sets where the server tells, and an absent field leaves the switch as it is.
func TestOnlyAnAdministratorTurnsTheNoticeOnAndAnAbsentFieldLeavesIt(t *testing.T) {
	team, srv, _, _ := noticeSetup(t, true)
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")
	grantOnBoard(t, team, "helper", "lastboard")
	helper := team.signInAs("helper")

	if got := status(t, helper.do(http.MethodPut, "/api/notifications", map[string]any{"kind": "", "updates": false})); got != http.StatusForbidden {
		t.Errorf("a collaborator changed the server's channel: %d", got)
	}
	if !srv.updateNoticeOn() {
		t.Fatal("the refused change turned the notice off")
	}

	// A page that does not know the field.
	resp := team.do(http.MethodPut, "/api/notifications", map[string]any{"kind": "ntfy", "url": "https://ntfy.sh/acme"})
	_ = resp.Body.Close()
	if !srv.updateNoticeOn() {
		t.Error("a save without the field turned the notice off")
	}
	got := decode(t, team.do(http.MethodGet, "/api/notifications", nil))
	if got["updates"] != true {
		t.Errorf("the channel reads back updates=%v, want true", got["updates"])
	}

	resp = team.do(http.MethodPut, "/api/notifications", map[string]any{"kind": "ntfy", "url": "https://ntfy.sh/acme", "updates": false})
	_ = resp.Body.Close()
	if srv.updateNoticeOn() {
		t.Error("asking for it off left it on")
	}
}

// The watcher looks on its own, and turning the notice on does not wait for the next round.
func TestTheWatcherTellsWithoutBeingAsked(t *testing.T) {
	_, srv, _, hook := noticeSetup(t, true)
	was := noticeFirstCheck
	noticeFirstCheck = 20 * time.Millisecond
	t.Cleanup(func() { noticeFirstCheck = was })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { srv.WatchUpdates(ctx); close(done) }()
	t.Cleanup(func() { cancel(); <-done })

	deadline := time.Now().Add(5 * time.Second)
	for len(hook.got()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if len(hook.got()) != 1 {
		t.Fatalf("the watcher told %d times, want 1", len(hook.got()))
	}
}

func TestTurningTheNoticeOnTellsAtOnceWhenTheWatcherIsRunning(t *testing.T) {
	team, srv, _, hook := noticeSetup(t, false)
	srv.watching.Store(true)

	resp := team.do(http.MethodPut, "/api/notifications", map[string]any{"kind": "webhook", "url": hook.srv.URL, "updates": true})
	_ = resp.Body.Close()

	deadline := time.Now().Add(5 * time.Second)
	for len(hook.got()) == 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if len(hook.got()) != 1 {
		t.Fatalf("turning it on told %d times, want 1", len(hook.got()))
	}
}

// With the server set to work offline nothing is asked and nothing is told.
func TestAnOfflineServerAsksAndTellsNothingAboutVersions(t *testing.T) {
	_, srv, forge, hook := noticeSetup(t, true)
	srv.cfg.Offline = true

	srv.notifyUpdate(context.Background())
	if forge.asked() != 0 || len(hook.got()) != 0 {
		t.Errorf("offline, the forge was asked %d times and the channel told %d", forge.asked(), len(hook.got()))
	}
}

// A build with no release keys cannot install an update, so there is nothing to tell about —
// except in a container, where the answer to a new version is to pull an image.
func TestABuildThatCannotVerifyIsOnlyToldInAContainer(t *testing.T) {
	_, srv, forge, hook := noticeSetup(t, true)
	keys, container := version.ReleaseKeys, inContainer
	version.ReleaseKeys = nil
	t.Cleanup(func() { version.ReleaseKeys, inContainer = keys, container })

	inContainer = func() bool { return false }
	srv.notifyUpdate(context.Background())
	if forge.asked() != 0 || len(hook.got()) != 0 {
		t.Fatalf("a build that cannot verify, outside a container, asked %d times and told %d", forge.asked(), len(hook.got()))
	}

	inContainer = func() bool { return true }
	srv.notifyUpdate(context.Background())
	if len(hook.got()) != 1 {
		t.Errorf("in a container it told %d times, want 1", len(hook.got()))
	}
}
