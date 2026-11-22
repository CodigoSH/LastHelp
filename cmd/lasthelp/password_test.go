package main

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/account"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/session"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
)

// typed plays the person at the keyboard: each Ask takes the next answer, and a
// prompt is remembered so a test can see what was asked.
type typed struct {
	answers  []string
	prompts  []string
	terminal bool
}

func (t *typed) Terminal() bool { return t.terminal }
func (t *typed) Ask(prompt string) ([]byte, error) {
	t.prompts = append(t.prompts, prompt)
	if len(t.answers) == 0 {
		return nil, errors.New("asked for more than the test typed")
	}
	next := t.answers[0]
	t.answers = t.answers[1:]
	return []byte(next), nil
}

type fixture struct {
	t         *testing.T
	accounts  account.Store
	sessions  session.Store
	ana, luis account.User
}

func newFixture(t *testing.T) (*fixture, database.Execer) {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	accounts := account.Store{DB: db}
	now := time.Unix(1_700_000_000, 0)
	ana, err := accounts.Create("Ana", "the old passphrase", true, now)
	if err != nil {
		t.Fatal(err)
	}
	luis, err := accounts.Create("luis", "luis has a passphrase", false, now)
	if err != nil {
		t.Fatal(err)
	}
	return &fixture{t: t, accounts: accounts, sessions: session.Store{DB: db}, ana: ana, luis: luis}, db
}

func (f *fixture) signedIn(user account.User) string {
	f.t.Helper()
	token, err := f.sessions.Create(user.ID, time.Unix(1_700_000_000, 0), time.Hour)
	if err != nil {
		f.t.Fatal(err)
	}
	return token
}

func (f *fixture) resolves(token string) bool {
	_, err := f.sessions.Resolve(token, time.Unix(1_700_000_100, 0), time.Hour)
	return err == nil
}

func TestResettingAPasswordSetsItAndClosesItsSessions(t *testing.T) {
	f, db := newFixture(t)
	anaSession, luisSession := f.signedIn(f.ana), f.signedIn(f.luis)
	var out, errOut bytes.Buffer

	in := &typed{terminal: true, answers: []string{"a brand new passphrase", "a brand new passphrase"}}
	if rc := resetPassword(db, " ANA ", in, &out, &errOut); rc != 0 {
		t.Fatalf("exit %d: %s", rc, errOut.String())
	}

	if _, err := f.accounts.Authenticate("ana", "a brand new passphrase"); err != nil {
		t.Errorf("the new password does not sign in: %v", err)
	}
	if _, err := f.accounts.Authenticate("ana", "the old passphrase"); err == nil {
		t.Error("the old password still signs in")
	}
	if f.resolves(anaSession) {
		t.Error("a session of the account survived the reset")
	}
	if !f.resolves(luisSession) {
		t.Error("another account's session was closed")
	}
	if _, err := f.accounts.Authenticate("luis", "luis has a passphrase"); err != nil {
		t.Errorf("another account was touched: %v", err)
	}
	if strings.Contains(out.String()+errOut.String(), "a brand new passphrase") {
		t.Error("the password was printed")
	}
}

func TestNothingChangesWhenTheTwoPasswordsDiffer(t *testing.T) {
	f, db := newFixture(t)
	token := f.signedIn(f.ana)
	var out, errOut bytes.Buffer

	in := &typed{terminal: true, answers: []string{"a brand new passphrase", "another brand new one"}}
	if rc := resetPassword(db, "ana", in, &out, &errOut); rc == 0 {
		t.Fatal("two different passwords were accepted")
	}
	if _, err := f.accounts.Authenticate("ana", "the old passphrase"); err != nil {
		t.Errorf("the password changed anyway: %v", err)
	}
	if !f.resolves(token) {
		t.Error("the sessions were closed though nothing was changed")
	}
}

func TestAPasswordTheSignInFormWouldRefuseIsRefused(t *testing.T) {
	f, db := newFixture(t)
	var out, errOut bytes.Buffer

	in := &typed{terminal: true, answers: []string{"short", "short"}}
	if rc := resetPassword(db, "ana", in, &out, &errOut); rc == 0 {
		t.Fatal("a nine-character-or-less password was accepted")
	}
	if _, err := f.accounts.Authenticate("ana", "the old passphrase"); err != nil {
		t.Errorf("the password changed anyway: %v", err)
	}
}

func TestAnAccountThatDoesNotExistIsRefusedBeforeAskingForAPassword(t *testing.T) {
	_, db := newFixture(t)
	var out, errOut bytes.Buffer

	in := &typed{terminal: true, answers: []string{"x", "x"}}
	if rc := resetPassword(db, "nobody", in, &out, &errOut); rc == 0 {
		t.Fatal("it reset an account that does not exist")
	}
	if len(in.prompts) != 0 {
		t.Errorf("it asked for a password nobody would use: %v", in.prompts)
	}
}

func TestWithoutATerminalItDoesNotRun(t *testing.T) {
	var out, errOut bytes.Buffer
	in := &typed{terminal: false}
	if rc := runPassword([]string{"reset", "ana"}, in, &out, &errOut); rc == 0 {
		t.Fatal("it ran with no terminal to ask on")
	}
	if !strings.Contains(errOut.String(), "terminal") {
		t.Errorf("it did not say why: %q", errOut.String())
	}
}

func TestTheUsageIsSaidWhenTheArgumentsAreWrong(t *testing.T) {
	for _, args := range [][]string{nil, {"reset"}, {"change", "ana"}, {"reset", "a", "b"}} {
		var out, errOut bytes.Buffer
		if rc := runPassword(args, &typed{terminal: true}, &out, &errOut); rc != 2 {
			t.Errorf("%v: exit %d, want 2", args, rc)
		}
		if !strings.Contains(errOut.String(), "usage:") {
			t.Errorf("%v: no usage line: %q", args, errOut.String())
		}
	}
}
