package twostep

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/totp"
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
	if _, err := db.Exec(`INSERT INTO app_user (id, username, password_hash, created_at) VALUES (7, 'ana', 'x', 0)`); err != nil {
		t.Fatal(err)
	}
	return Store{DB: db}
}

// turnedOn drafts and confirms a secret, and returns it with the recovery codes.
func turnedOn(t *testing.T, s Store) ([]byte, []string) {
	t.Helper()
	encoded, err := s.Draft(7, epoch)
	if err != nil {
		t.Fatal(err)
	}
	secret, _ := totp.DecodeSecret(encoded)
	codes, err := s.Enable(7, totp.Code(secret, epoch), epoch)
	if err != nil {
		t.Fatal(err)
	}
	return secret, codes
}

func TestADraftIsNotInForceUntilAConfirmedCode(t *testing.T) {
	s := store(t)
	encoded, err := s.Draft(7, epoch)
	if err != nil {
		t.Fatal(err)
	}
	if on, _ := s.Enabled(7); on {
		t.Fatal("a drafted secret is already in force")
	}
	if _, err := s.Enable(7, "000000", epoch); !errors.Is(err, ErrRejected) {
		t.Errorf("a wrong code turned it on: %v", err)
	}
	secret, _ := totp.DecodeSecret(encoded)
	codes, err := s.Enable(7, totp.Code(secret, epoch), epoch)
	if err != nil || len(codes) != totp.RecoveryCodes {
		t.Fatalf("enable: %v, %d codes", err, len(codes))
	}
	if st, _ := s.Status(7); !st.Enabled || st.RecoveryLeft != totp.RecoveryCodes {
		t.Errorf("status %+v", st)
	}
	if _, err := s.Draft(7, epoch); !errors.Is(err, ErrAlreadyOn) {
		t.Errorf("drafting over a second step in force: %v", err)
	}
}

func TestACodeFromTheAppSignsInOnce(t *testing.T) {
	s := store(t)
	secret, _ := turnedOn(t, s)
	later := epoch.Add(totp.Period)
	code := totp.Code(secret, later)
	if err := s.Accept(7, code, later); err != nil {
		t.Fatalf("a current code: %v", err)
	}
	if err := s.Accept(7, code, later); !errors.Is(err, ErrRejected) {
		t.Errorf("the same code twice: %v", err)
	}
}

func TestARecoveryCodeSignsInOnce(t *testing.T) {
	s := store(t)
	_, codes := turnedOn(t, s)
	if err := s.Accept(7, codes[0], epoch); err != nil {
		t.Fatalf("a recovery code: %v", err)
	}
	if err := s.Accept(7, codes[0], epoch); !errors.Is(err, ErrRejected) {
		t.Errorf("the same recovery code twice: %v", err)
	}
	if st, _ := s.Status(7); st.RecoveryLeft != totp.RecoveryCodes-1 {
		t.Errorf("left %d", st.RecoveryLeft)
	}
}

func TestAChallengeEndsAfterItsAttemptsTimeOrUse(t *testing.T) {
	s := store(t)
	token, err := s.NewChallenge(7, epoch)
	if err != nil {
		t.Fatal(err)
	}
	if id, err := s.Challenge(token, epoch); err != nil || id != 7 {
		t.Fatalf("challenge: %d, %v", id, err)
	}
	if _, err := s.Challenge(token, epoch.Add(ChallengeLifetime+time.Second)); !errors.Is(err, ErrChallengeGone) {
		t.Errorf("out of time: %v", err)
	}
	for i := 0; i < ChallengeAttempts; i++ {
		_ = s.FailChallenge(token)
	}
	if _, err := s.Challenge(token, epoch); !errors.Is(err, ErrChallengeGone) {
		t.Errorf("after %d wrong codes: %v", ChallengeAttempts, err)
	}
	token, _ = s.NewChallenge(7, epoch)
	if err := s.SpendChallenge(token); err != nil {
		t.Fatal(err)
	}
	if err := s.SpendChallenge(token); !errors.Is(err, ErrChallengeGone) {
		t.Errorf("spent twice: %v", err)
	}
}

func TestRemoveTakesEverythingOff(t *testing.T) {
	s := store(t)
	_, codes := turnedOn(t, s)
	token, _ := s.NewChallenge(7, epoch)
	if err := s.Remove(7); err != nil {
		t.Fatal(err)
	}
	if on, _ := s.Enabled(7); on {
		t.Error("still on")
	}
	if err := s.Accept(7, codes[1], epoch); !errors.Is(err, ErrRejected) {
		t.Errorf("a recovery code after removal: %v", err)
	}
	if _, err := s.Challenge(token, epoch); !errors.Is(err, ErrChallengeGone) {
		t.Errorf("a waiting sign-in after removal: %v", err)
	}
}
