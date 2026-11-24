package session

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
)

var epoch = time.Unix(1_700_000_000, 0)

func store(t *testing.T) Store {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, err := db.Exec(`INSERT INTO app_user (id, username, password_hash, created_at)
		VALUES (7, 'a@example.com', 'x', 0)`); err != nil {
		t.Fatal(err)
	}
	return Store{DB: db}
}

func TestCreateAndResolve(t *testing.T) {
	s := store(t)

	token, err := s.Create(7, epoch, time.Hour, Device{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Resolve(token, epoch.Add(time.Minute), time.Hour, Device{})
	if err != nil {
		t.Fatal(err)
	}
	if got.UserID != 7 {
		t.Errorf("resolved to %d, want 7", got.UserID)
	}
}

// TestOnlyTheHashIsStored.
//
// A database backup that leaked would otherwise be a drawer full of live
// sessions somebody can paste into a browser.
func TestOnlyTheHashIsStored(t *testing.T) {
	s := store(t)
	token, err := s.Create(7, epoch, time.Hour, Device{})
	if err != nil {
		t.Fatal(err)
	}

	var stored string
	if err := s.DB.QueryRow(`SELECT token_hash FROM session`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored == token {
		t.Fatal("the token itself is in the database")
	}
	if len(stored) != 64 {
		t.Errorf("stored value is %d characters, want a 64-character sha256 hex digest", len(stored))
	}
}

func TestAnExpiredSessionIsRefusedAndRemoved(t *testing.T) {
	s := store(t)
	token, err := s.Create(7, epoch, time.Hour, Device{})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.Resolve(token, epoch.Add(2*time.Hour), time.Hour, Device{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("an expired session resolved: %v", err)
	}

	var left int
	if err := s.DB.QueryRow(`SELECT count(*) FROM session`).Scan(&left); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Error("the expired row was left behind")
	}
}

func TestAnUnknownTokenIsRefused(t *testing.T) {
	s := store(t)
	for _, token := range []string{"", "nonsense", "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"} {
		if _, err := s.Resolve(token, epoch, time.Hour, Device{}); !errors.Is(err, ErrInvalid) {
			t.Errorf("token %q gave %v", token, err)
		}
	}
}

// TestUsingASessionSlidesItsExpiry, and only occasionally writes.
//
// Somebody in the panel every day should not be signed out on the thirtieth.
// But a session lookup happens on every request, so the write has to be rare or
// it is a write on every request.
func TestUsingASessionSlidesItsExpiry(t *testing.T) {
	s := store(t)
	token, err := s.Create(7, epoch, 24*time.Hour, Device{})
	if err != nil {
		t.Fatal(err)
	}

	expiry := func() int64 {
		t.Helper()
		var v int64
		if err := s.DB.QueryRow(`SELECT expires_at FROM session`).Scan(&v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	first := expiry()

	// A second request moments later must not write.
	if _, err := s.Resolve(token, epoch.Add(time.Minute), 24*time.Hour, Device{}); err != nil {
		t.Fatal(err)
	}
	if expiry() != first {
		t.Error("a request a minute later rewrote the row; that is a write on every request")
	}

	// A day later it must.
	if _, err := s.Resolve(token, epoch.Add(20*time.Hour), 24*time.Hour, Device{}); err != nil {
		t.Fatal(err)
	}
	if expiry() == first {
		t.Error("using the session did not slide its expiry, so an active person is signed out on schedule")
	}
}

func TestSigningOutEndsTheSessionEverywhere(t *testing.T) {
	s := store(t)
	token, err := s.Create(7, epoch, time.Hour, Device{})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(token); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve(token, epoch, time.Hour, Device{}); !errors.Is(err, ErrInvalid) {
		t.Error("the token still worked after signing out; dropping the cookie is not ending the session")
	}
}

func TestDeleteAllForEndsEverySessionOfAnAccount(t *testing.T) {
	s := store(t)
	var tokens []string
	for i := 0; i < 3; i++ {
		token, err := s.Create(7, epoch, time.Hour, Device{})
		if err != nil {
			t.Fatal(err)
		}
		tokens = append(tokens, token)
	}
	if err := s.DeleteAllFor(7); err != nil {
		t.Fatal(err)
	}
	for _, token := range tokens {
		if _, err := s.Resolve(token, epoch, time.Hour, Device{}); !errors.Is(err, ErrInvalid) {
			t.Error("a session survived; disabling somebody would not actually lock them out")
		}
	}
}

func TestPurgeRemovesOnlyWhatExpired(t *testing.T) {
	s := store(t)
	short, err := s.Create(7, epoch, time.Hour, Device{})
	if err != nil {
		t.Fatal(err)
	}
	long, err := s.Create(7, epoch, 100*time.Hour, Device{})
	if err != nil {
		t.Fatal(err)
	}

	if err := s.Purge(epoch.Add(2 * time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve(short, epoch.Add(2*time.Hour), time.Hour, Device{}); !errors.Is(err, ErrInvalid) {
		t.Error("the expired session survived the purge")
	}
	if _, err := s.Resolve(long, epoch.Add(2*time.Hour), 100*time.Hour, Device{}); err != nil {
		t.Errorf("the purge took a live session with it: %v", err)
	}
}

var firefox = Device{UserAgent: "Mozilla/5.0 (X11; Linux x86_64; rv:140.0) Gecko/20100101 Firefox/140.0", IP: "192.0.2.7"}

// The list says where each session was opened from, and what it says about one
// moves with it at most once a minute, which is when the expiry slides too.
func TestTheListShowsTheDeviceAndFollowsItHourly(t *testing.T) {
	s := store(t)
	token, err := s.Create(7, epoch, 24*time.Hour, firefox)
	if err != nil {
		t.Fatal(err)
	}
	list, err := s.List(7, epoch)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].UserAgent != firefox.UserAgent || list[0].IP != firefox.IP {
		t.Fatalf("the list is %+v, want the one session from Firefox at 192.0.2.7", list)
	}

	phone := Device{UserAgent: "phone", IP: "198.51.100.1"}
	found, err := s.Resolve(token, epoch.Add(time.Minute), 24*time.Hour, phone)
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != list[0].ID {
		t.Errorf("the token resolved to session %q, the list names it %q", found.ID, list[0].ID)
	}
	if list, _ = s.List(7, epoch); list[0].IP != firefox.IP {
		t.Errorf("within the minute the address is %q, want it unchanged", list[0].IP)
	}
	if _, err := s.Resolve(token, epoch.Add(2*time.Hour), 24*time.Hour, phone); err != nil {
		t.Fatal(err)
	}
	if list, _ = s.List(7, epoch); list[0].IP != phone.IP || list[0].UserAgent != phone.UserAgent {
		t.Errorf("two hours later the session is %+v, want it to say the phone", list[0])
	}
}

// A browser can say anything in its description; what is kept of it is capped,
// in characters and not bytes, so a long one in any script is cut and not broken.
func TestABrowserDescriptionIsCapped(t *testing.T) {
	s := store(t)
	long := strings.Repeat("ж", maxUserAgent+50)
	if _, err := s.Create(7, epoch, time.Hour, Device{UserAgent: long}); err != nil {
		t.Fatal(err)
	}
	list, _ := s.List(7, epoch)
	if got := utf8.RuneCountInString(list[0].UserAgent); got != maxUserAgent || !utf8.ValidString(list[0].UserAgent) {
		t.Errorf("kept %d characters (valid %v), want %d", got, utf8.ValidString(list[0].UserAgent), maxUserAgent)
	}
}

// The list is one account's own, without what has expired.
func TestTheListIsTheAccountsOwnAndAlive(t *testing.T) {
	s := store(t)
	if _, err := s.DB.Exec(`INSERT INTO app_user (id, username, password_hash, created_at) VALUES (8, 'b', 'x', 0)`); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(7, epoch, time.Hour, firefox); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(7, epoch, time.Minute, firefox); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Create(8, epoch, time.Hour, firefox); err != nil {
		t.Fatal(err)
	}
	list, err := s.List(7, epoch.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Errorf("listed %d sessions, want the one of this account that has not expired", len(list))
	}
}

// Closing is scoped to the account: another account's session is not found,
// and so is anything that is not a session's name.
func TestClosingIsTheAccountsOwn(t *testing.T) {
	s := store(t)
	if _, err := s.DB.Exec(`INSERT INTO app_user (id, username, password_hash, created_at) VALUES (8, 'b', 'x', 0)`); err != nil {
		t.Fatal(err)
	}
	mine, _ := s.Create(7, epoch, time.Hour, firefox)
	theirs, _ := s.Create(8, epoch, time.Hour, firefox)
	theirID := idOf(hashToken(theirs))

	if err := s.Close(7, theirID); !errors.Is(err, ErrNotFound) {
		t.Errorf("closing another account's session: %v, want not found", err)
	}
	if _, err := s.Resolve(theirs, epoch, time.Hour, Device{}); err != nil {
		t.Errorf("their session after the attempt: %v", err)
	}
	for _, bad := range []string{"", "%", "' OR 1=1 --", strings.ToUpper(idOf(hashToken(mine)))} {
		if err := s.Close(7, bad); !errors.Is(err, ErrNotFound) {
			t.Errorf("closing %q: %v, want not found", bad, err)
		}
	}
	if err := s.Close(7, idOf(hashToken(mine))); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve(mine, epoch, time.Hour, Device{}); !errors.Is(err, ErrInvalid) {
		t.Errorf("a closed session still resolves: %v", err)
	}
}

// "Close the others" keeps the one asking and the other accounts' sessions,
// and with nothing to keep it closes nothing.
func TestClosingTheOthersKeepsThisOne(t *testing.T) {
	s := store(t)
	if _, err := s.DB.Exec(`INSERT INTO app_user (id, username, password_hash, created_at) VALUES (8, 'b', 'x', 0)`); err != nil {
		t.Fatal(err)
	}
	here, _ := s.Create(7, epoch, time.Hour, firefox)
	a, _ := s.Create(7, epoch, time.Hour, firefox)
	b, _ := s.Create(7, epoch, time.Hour, firefox)
	theirs, _ := s.Create(8, epoch, time.Hour, firefox)

	if n, err := s.CloseOthers(7, ""); !errors.Is(err, ErrNotFound) || n != 0 {
		t.Errorf("with nothing to keep: closed %d, %v; want none and not found", n, err)
	}
	n, err := s.CloseOthers(7, idOf(hashToken(here)))
	if err != nil || n != 2 {
		t.Fatalf("closed %d (%v), want 2", n, err)
	}
	for _, gone := range []string{a, b} {
		if _, err := s.Resolve(gone, epoch, time.Hour, Device{}); !errors.Is(err, ErrInvalid) {
			t.Errorf("another session still resolves: %v", err)
		}
	}
	for _, kept := range []string{here, theirs} {
		if _, err := s.Resolve(kept, epoch, time.Hour, Device{}); err != nil {
			t.Errorf("a session that should be kept: %v", err)
		}
	}
}

// A session from before the list was kept knows no browser; it learns it the
// first time it is used, without waiting for the hour, and only once.
func TestASessionWithoutABrowserLearnsItAtOnce(t *testing.T) {
	s := store(t)
	token, err := s.Create(7, epoch, time.Hour, Device{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve(token, epoch.Add(time.Minute), time.Hour, Device{}); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.List(7, epoch); list[0].UserAgent != "" {
		t.Errorf("a request without a browser wrote %q", list[0].UserAgent)
	}
	if _, err := s.Resolve(token, epoch.Add(2*time.Minute), time.Hour, firefox); err != nil {
		t.Fatal(err)
	}
	if list, _ := s.List(7, epoch); list[0].UserAgent != firefox.UserAgent || list[0].IP != firefox.IP {
		t.Errorf("after one use the session says %+v, want Firefox at 192.0.2.7", list[0])
	}
}
