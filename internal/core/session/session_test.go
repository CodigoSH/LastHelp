package session

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

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
	if _, err := db.Exec(`INSERT INTO app_user (id, email, password_hash, created_at)
		VALUES (7, 'a@example.com', 'x', 0)`); err != nil {
		t.Fatal(err)
	}
	return Store{DB: db}
}

func TestCreateAndResolve(t *testing.T) {
	s := store(t)

	token, err := s.Create(7, epoch, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Resolve(token, epoch.Add(time.Minute), time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if got != 7 {
		t.Errorf("resolved to %d, want 7", got)
	}
}

// TestOnlyTheHashIsStored.
//
// A database backup that leaked would otherwise be a drawer full of live
// sessions somebody can paste into a browser.
func TestOnlyTheHashIsStored(t *testing.T) {
	s := store(t)
	token, err := s.Create(7, epoch, time.Hour)
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
	token, err := s.Create(7, epoch, time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := s.Resolve(token, epoch.Add(2*time.Hour), time.Hour); !errors.Is(err, ErrInvalid) {
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
		if _, err := s.Resolve(token, epoch, time.Hour); !errors.Is(err, ErrInvalid) {
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
	token, err := s.Create(7, epoch, 24*time.Hour)
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
	if _, err := s.Resolve(token, epoch.Add(time.Minute), 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if expiry() != first {
		t.Error("a request a minute later rewrote the row; that is a write on every request")
	}

	// A day later it must.
	if _, err := s.Resolve(token, epoch.Add(20*time.Hour), 24*time.Hour); err != nil {
		t.Fatal(err)
	}
	if expiry() == first {
		t.Error("using the session did not slide its expiry, so an active person is signed out on schedule")
	}
}

func TestSigningOutEndsTheSessionEverywhere(t *testing.T) {
	s := store(t)
	token, err := s.Create(7, epoch, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Delete(token); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve(token, epoch, time.Hour); !errors.Is(err, ErrInvalid) {
		t.Error("the token still worked after signing out; dropping the cookie is not ending the session")
	}
}

func TestDeleteAllForEndsEverySessionOfAnAccount(t *testing.T) {
	s := store(t)
	var tokens []string
	for i := 0; i < 3; i++ {
		token, err := s.Create(7, epoch, time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		tokens = append(tokens, token)
	}
	if err := s.DeleteAllFor(7); err != nil {
		t.Fatal(err)
	}
	for _, token := range tokens {
		if _, err := s.Resolve(token, epoch, time.Hour); !errors.Is(err, ErrInvalid) {
			t.Error("a session survived; disabling somebody would not actually lock them out")
		}
	}
}

func TestPurgeRemovesOnlyWhatExpired(t *testing.T) {
	s := store(t)
	short, err := s.Create(7, epoch, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	long, err := s.Create(7, epoch, 100*time.Hour)
	if err != nil {
		t.Fatal(err)
	}

	if err := s.Purge(epoch.Add(2 * time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Resolve(short, epoch.Add(2*time.Hour), time.Hour); !errors.Is(err, ErrInvalid) {
		t.Error("the expired session survived the purge")
	}
	if _, err := s.Resolve(long, epoch.Add(2*time.Hour), 100*time.Hour); err != nil {
		t.Errorf("the purge took a live session with it: %v", err)
	}
}
