package web

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/notify"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/webpush"
	"git.codigosh.com/CodigoSH/LastHelp/internal/web/middleware"
)

// Web Push: telling a browser something while no page of LastHelp is open.
//
// It is the one place that talks to a third party the operator did not type in.
// A browser, when it subscribes, is given an address by its maker's push
// service, and a message goes there: to Google for Chrome and Android, to Apple
// for Safari and iPhones, to Mozilla for Firefox, to Microsoft for Edge. The
// message is encrypted for the one browser it is for, so the service carries it
// without reading it, but it does learn that a device was told something, and
// when. That is why it is a channel a board chooses and a person turns on in
// their own browser, and why the documentation says so.
//
// The address comes from the browser, which is to say from whoever is signed
// in, so it is not trusted: only an https address on a push service's own
// domain is ever sent to.

// errNoPushDevices says nobody has a browser subscribed for what was asked.
var errNoPushDevices = errors.New("push: no devices subscribed")

// errPushNotDelivered says there were devices and none could be told.
var errPushNotDelivered = errors.New("push: no device could be told")

// pushHosts are the push services' own domains. An entry starting with a dot
// matches any host under it; one without matches exactly that host.
var pushHosts = []string{
	"fcm.googleapis.com",                // Chrome, Chromium browsers, Samsung Internet, Android
	"updates.push.services.mozilla.com", // Firefox
	".push.services.mozilla.com",        // Firefox, the servers a subscription may live on
	".push.apple.com",                   // Safari, and apps added to an iPhone's or iPad's home screen
	".notify.windows.com",               // Edge
}

func pushHostAllowed(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	for _, h := range pushHosts {
		if strings.HasPrefix(h, ".") {
			if strings.HasSuffix(host, h) && len(host) > len(h) {
				return true
			}
		} else if host == h {
			return true
		}
	}
	return false
}

// allowedPushEndpoint says whether an address a browser gave is one to send to:
// https, on a push service's domain, on the usual port, with no credentials.
func allowedPushEndpoint(raw string) bool {
	if len(raw) == 0 || len(raw) > 2048 {
		return false
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host == "" || u.User != nil {
		return false
	}
	if p := u.Port(); p != "" && p != "443" {
		return false
	}
	return pushHostAllowed(u.Hostname())
}

// pushEndpointOK is the rule for which addresses are sent to. A variable only so
// a test can point the sender at a push service of its own.
var pushEndpointOK = allowedPushEndpoint

// maxPushDevices is how many browsers one account may subscribe. A person has a
// handful; the cap is what stops a loop in a script from filling the table.
const maxPushDevices = 20

const (
	pushPrivateKey = "push_vapid_private"
	pushPublicKey  = "push_vapid_public"
)

// defaultPushSubject is who a push service may write to about the sender. It
// names the software, not the person running it.
const defaultPushSubject = "https://git.codigosh.com/CodigoSH/LastHelp"

// pushPayload is what a notification says. A tap goes to a path inside this
// instance, never anywhere else.
type pushPayload struct {
	Title string `json:"title"`
	Body  string `json:"body"`
	URL   string `json:"url,omitempty"`
}

type pushSub struct {
	ID       int64
	UserID   int64
	Endpoint string
	P256dh   string
	Auth     string
}

// vapidKeys returns the pair this instance signs with, making it the first time
// anybody asks. It is the instance's own: browsers subscribe to its public half,
// so changing it would strand every device. Kept in the settings with the other
// secrets, so a backup carries it and a restored server keeps its devices.
func (s *Server) vapidKeys() (private, public string, err error) {
	st := s.settings()
	read := func() (string, string, bool) {
		priv, _ := st.Get(pushPrivateKey)
		pub, _ := st.Get(pushPublicKey)
		return priv, pub, priv != "" && pub != ""
	}
	if priv, pub, ok := read(); ok {
		return priv, pub, nil
	}
	priv, pub, err := webpush.GenerateVAPIDKeys()
	if err != nil {
		return "", "", err
	}
	// Both or neither, and the first writer wins: two requests asking at once end
	// up with the same pair, which is read back below.
	tx, err := s.db.Begin()
	if err != nil {
		return "", "", err
	}
	defer func() { _ = tx.Rollback() }()
	for k, v := range map[string]string{pushPrivateKey: priv, pushPublicKey: pub} {
		if _, err := tx.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT (key) DO NOTHING`, k, v); err != nil {
			return "", "", err
		}
	}
	if err := tx.Commit(); err != nil {
		return "", "", err
	}
	if priv, pub, ok := read(); ok {
		return priv, pub, nil
	}
	return "", "", errors.New("push: the signing keys could not be stored")
}

func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

// pushTo is the Push channel of the notifier: to one account's browsers when the
// message is for one (a test), else to the browsers of whoever works on its
// board.
//
// Nobody having turned it on is not a failure of an announcement — the board
// chose a channel and nobody has joined it yet — but it is for a test, which
// exists to say so.
func (s *Server) pushTo(ctx context.Context, msg notify.Message) error {
	var subs []pushSub
	var err error
	switch {
	case msg.User != 0:
		subs, err = s.pushSubsOfUser(ctx, msg.User)
	case msg.Repo != 0:
		subs, err = s.pushSubsOfTeam(ctx, msg.Repo)
	default:
		return errors.New("push: a message with nobody to tell")
	}
	if err != nil {
		return err
	}
	if len(subs) == 0 {
		if msg.User != 0 {
			return errNoPushDevices
		}
		return nil
	}

	priv, _, err := s.vapidKeys()
	if err != nil {
		return err
	}
	// Each person is told in their own language: the browsers are grouped by the
	// language of the account they belong to, and each group gets one message.
	payloads := map[string][]byte{}
	sent, gone := 0, 0
	for _, sub := range subs {
		lang := s.languageOf(sub.UserID)
		payload, ok := payloads[lang]
		if !ok {
			if payload, err = json.Marshal(s.pushPayloadIn(lang, msg)); err != nil {
				return err
			}
			payloads[lang] = payload
		}
		switch s.sendPushOne(ctx, sub, payload, priv) {
		case pushSent:
			sent++
		case pushGone:
			gone++
		}
	}
	if sent == 0 && gone < len(subs) {
		return errPushNotDelivered
	}
	return nil
}

// pushPayloadIn is the notification as one language reads it. What arrived is
// named in that language ("Atlas: Idea"); a message that is not about a report,
// a test, carries the title its writer already wrote.
func (s *Server) pushPayloadIn(lang string, msg notify.Message) pushPayload {
	title := msg.Title
	if word := s.kindWord(lang, msg.Kind); word != "" {
		title = isolate(lang, msg.Title) + ": " + word
	}
	return pushPayload{Title: clip(title, 120), Body: clip(msg.Body, 300), URL: "/review"}
}

func scanPushSubs(rows *sql.Rows) ([]pushSub, error) {
	defer func() { _ = rows.Close() }()
	var out []pushSub
	for rows.Next() {
		var sub pushSub
		if err := rows.Scan(&sub.ID, &sub.UserID, &sub.Endpoint, &sub.P256dh, &sub.Auth); err != nil {
			return nil, err
		}
		out = append(out, sub)
	}
	return out, rows.Err()
}

func (s *Server) pushSubsOfUser(ctx context.Context, userID int64) ([]pushSub, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, user_id, endpoint, p256dh, auth FROM push_subscription WHERE user_id = ? ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	return scanPushSubs(rows)
}

// pushSubsOfTeam is the browsers of the accounts that work on a board: the
// administrator, the owners of its organisation and the collaborators on it. An
// account that was disabled is not told anything.
func (s *Server) pushSubsOfTeam(ctx context.Context, repoID int64) ([]pushSub, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT p.id, p.user_id, p.endpoint, p.p256dh, p.auth
		FROM push_subscription p
		JOIN app_user u ON u.id = p.user_id AND u.disabled = 0
		WHERE u.is_administrator = 1
		   OR EXISTS (SELECT 1 FROM membership m
		              WHERE m.user_id = u.id
		                AND ((m.scope_kind = ? AND m.scope_id = ?)
		                  OR (m.scope_kind = ? AND m.role = ?
		                      AND m.scope_id = (SELECT org_id FROM repository WHERE id = ?))))
		ORDER BY p.id`,
		string(authz.ScopeRepository), repoID, string(authz.ScopeOrganization), string(authz.RoleOwner), repoID)
	if err != nil {
		return nil, err
	}
	return scanPushSubs(rows)
}

type pushOutcome int

const (
	pushSent pushOutcome = iota
	pushGone
	pushFailed
)

func (s *Server) pushSubject() string {
	if u := strings.TrimSpace(s.cfg.PublicURL); strings.HasPrefix(u, "https://") {
		return u
	}
	return defaultPushSubject
}

func (s *Server) sendPushOne(ctx context.Context, sub pushSub, payload []byte, private string) pushOutcome {
	if !pushEndpointOK(sub.Endpoint) {
		// Stored before this check existed, or written by hand: never sent to.
		slog.Warn("push: a subscription is not on a push service's domain; skipped", "subscription", sub.ID)
		return pushFailed
	}
	body, err := webpush.Encrypt(payload, sub.P256dh, sub.Auth)
	if err != nil {
		slog.Warn("push: could not encrypt for a subscription", "subscription", sub.ID, "error", err)
		return pushFailed
	}
	auth, err := webpush.Authorization(sub.Endpoint, s.pushSubject(), private, s.now())
	if err != nil {
		slog.Error("push: could not sign", "error", err)
		return pushFailed
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, sub.Endpoint, bytes.NewReader(body))
	if err != nil {
		return pushFailed
	}
	req.Header.Set("Authorization", auth)
	req.Header.Set("Content-Encoding", "aes128gcm")
	req.Header.Set("Content-Type", "application/octet-stream")
	// Twelve hours: a report that arrived while a phone was switched off is still
	// news when it is switched on, and a day-old one is not.
	req.Header.Set("TTL", "43200")
	req.Header.Set("Urgency", "normal")

	resp, err := s.out.Do(req)
	if err != nil {
		slog.Warn("push: a push service could not be reached", "subscription", sub.ID, "error", err)
		return pushFailed
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))

	switch {
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		_, _ = s.db.ExecContext(ctx, `UPDATE push_subscription SET last_ok_at = ? WHERE id = ?`, s.now().Unix(), sub.ID)
		return pushSent
	case resp.StatusCode == http.StatusNotFound || resp.StatusCode == http.StatusGone:
		// The push service says the device is gone: forgotten, so an uninstalled
		// app stops costing a request every time.
		if _, err := s.db.ExecContext(ctx, `DELETE FROM push_subscription WHERE id = ?`, sub.ID); err != nil {
			slog.Warn("push: could not forget a subscription that is gone", "subscription", sub.ID, "error", err)
		}
		return pushGone
	default:
		slog.Warn("push: a push service refused a message", "subscription", sub.ID, "status", fmt.Sprint(resp.StatusCode))
		return pushFailed
	}
}

// --- the account's own devices ---------------------------------------------
//
// Nothing here reads another account's devices.

func (s *Server) handlePushKey(w http.ResponseWriter, _ *http.Request) {
	_, public, err := s.vapidKeys()
	if err != nil {
		slog.Error("push: reading the signing keys", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"public_key": public})
}

// pushRef names a subscription without being it: the first eight bytes of the
// SHA-256 of its address, in hex. A page computes the same for its own
// subscription, so it can tell which row in its list is this device while the
// address itself never leaves the server.
func pushRef(endpoint string) string {
	sum := sha256.Sum256([]byte(endpoint))
	return hex.EncodeToString(sum[:8])
}

type pushDevice struct {
	ID        int64  `json:"id"`
	Ref       string `json:"ref"`
	Label     string `json:"label"`
	CreatedAt int64  `json:"created_at"`
	LastOKAt  *int64 `json:"last_ok_at"`
}

func (s *Server) handlePushList(w http.ResponseWriter, r *http.Request) {
	actor, _ := middleware.ActorFrom(r)
	rows, err := s.db.QueryContext(r.Context(),
		`SELECT id, endpoint, label, created_at, last_ok_at FROM push_subscription WHERE user_id = ? ORDER BY id`, actor.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	defer func() { _ = rows.Close() }()
	out := []pushDevice{}
	for rows.Next() {
		var d pushDevice
		var endpoint string
		var ok sql.NullInt64
		if err := rows.Scan(&d.ID, &endpoint, &d.Label, &d.CreatedAt, &ok); err != nil {
			writeError(w, http.StatusInternalServerError, "error.internal")
			return
		}
		d.Ref = pushRef(endpoint)
		if ok.Valid {
			d.LastOKAt = &ok.Int64
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"devices": out})
}

// pushLabel is the name a device is listed under: what the browser called
// itself, with anything that is not text taken out and a limit on its length.
func pushLabel(raw string) string {
	clean := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, raw)
	clean = strings.Join(strings.Fields(clean), " ")
	if utf8.RuneCountInString(clean) > 60 {
		clean = string([]rune(clean)[:60])
	}
	if clean == "" {
		return "Browser"
	}
	return clean
}

type pushSubscribeRequest struct {
	Endpoint string `json:"endpoint"`
	Keys     struct {
		P256dh string `json:"p256dh"`
		Auth   string `json:"auth"`
	} `json:"keys"`
	Label string `json:"label"`
}

// handlePushSubscribe stores the subscription a browser made, for the caller.
// Subscribing the same browser again — it does so after its subscription is
// renewed — updates it instead of adding a second.
func (s *Server) handlePushSubscribe(w http.ResponseWriter, r *http.Request) {
	actor, _ := middleware.ActorFrom(r)
	r.Body = http.MaxBytesReader(w, r.Body, 8*1024)
	var in pushSubscribeRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}
	if !pushEndpointOK(in.Endpoint) {
		writeError(w, http.StatusBadRequest, "error.push_endpoint_refused")
		return
	}
	if err := webpush.ValidSubscriptionKeys(in.Keys.P256dh, in.Keys.Auth); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}

	tx, err := s.db.BeginTx(r.Context(), nil)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	defer func() { _ = tx.Rollback() }()
	// A device of this account subscribing again is not a new one: only a new
	// one counts toward the cap.
	var existing int
	_ = tx.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM push_subscription WHERE endpoint = ? AND user_id = ?`,
		in.Endpoint, actor.UserID).Scan(&existing)
	if existing == 0 {
		var n int
		if err := tx.QueryRowContext(r.Context(), `SELECT COUNT(*) FROM push_subscription WHERE user_id = ?`, actor.UserID).Scan(&n); err != nil {
			writeError(w, http.StatusInternalServerError, "error.internal")
			return
		}
		if n >= maxPushDevices {
			writeError(w, http.StatusConflict, "error.push_too_many_devices")
			return
		}
	}
	// ON CONFLICT and not INSERT OR REPLACE: the endpoint is unique, and a browser
	// shared between two accounts goes to the one that subscribes it now.
	if _, err := tx.Exec(`INSERT INTO push_subscription (user_id, endpoint, p256dh, auth, label, created_at)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (endpoint) DO UPDATE SET user_id = excluded.user_id, p256dh = excluded.p256dh,
			auth = excluded.auth, label = excluded.label`,
		actor.UserID, in.Endpoint, in.Keys.P256dh, in.Keys.Auth, pushLabel(in.Label), s.now().Unix()); err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	if err := tx.Commit(); err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handlePushDelete removes one of the caller's devices. Another account's id is
// the same as no id.
func (s *Server) handlePushDelete(w http.ResponseWriter, r *http.Request) {
	actor, _ := middleware.ActorFrom(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}
	res, err := s.db.ExecContext(r.Context(), `DELETE FROM push_subscription WHERE id = ? AND user_id = ?`, id, actor.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		writeError(w, http.StatusNotFound, "error.not_found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handlePushTest sends a notification to every device of the caller, so a person
// can see it arrive before relying on it.
func (s *Server) handlePushTest(w http.ResponseWriter, r *http.Request) {
	actor, _ := middleware.ActorFrom(r)
	ctx, cancel := context.WithTimeout(r.Context(), 12*time.Second)
	defer cancel()
	lang := s.languageOf(actor.UserID)
	err := s.pushTo(ctx, notify.Message{
		Title: "LastHelp: " + s.say(lang, "notify.test", nil),
		Body:  s.say(lang, "notify.testBody", map[string]string{"name": "LastHelp"}),
		User:  actor.UserID,
	})
	switch {
	case errors.Is(err, errNoPushDevices):
		writeError(w, http.StatusBadRequest, "error.push_no_devices")
	case err != nil:
		writeError(w, http.StatusBadGateway, "error.push_not_delivered")
	default:
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}
