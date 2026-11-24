package web

import (
	"net/http"
	"testing"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/account"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/totp"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/twostep"
)

// turnOnTwoStep sets up the second step for the account i is signed in as and
// returns its secret and recovery codes.
func (i *instance) turnOnTwoStep() ([]byte, []string) {
	i.t.Helper()
	resp := i.do(http.MethodPost, "/api/me/two-step/setup", nil)
	if resp.StatusCode != http.StatusOK {
		i.t.Fatalf("setup: %d", resp.StatusCode)
	}
	setup := decode(i.t, resp)
	secret, err := totp.DecodeSecret(setup["secret"].(string))
	if err != nil {
		i.t.Fatal(err)
	}
	resp = i.do(http.MethodPost, "/api/me/two-step/enable", map[string]string{"code": totp.Code(secret, i.now())})
	if resp.StatusCode != http.StatusOK {
		i.t.Fatalf("enable: %d", resp.StatusCode)
	}
	var codes []string
	for _, c := range decode(i.t, resp)["recovery_codes"].([]any) {
		codes = append(codes, c.(string))
	}
	return secret, codes
}

// passwordStep signs in with the owner's password and returns the challenge.
func passwordStep(t *testing.T, c *instance) string {
	t.Helper()
	resp := c.do(http.MethodPost, "/api/session", map[string]string{"username": "owner", "password": ownerPassword})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("password step: %d", resp.StatusCode)
	}
	got := decode(t, resp)
	challenge, _ := got["challenge"].(string)
	if got["two_step"] != true || challenge == "" {
		t.Fatalf("the password alone did not ask for a code: %v", got)
	}
	return challenge
}

func statusOf(c *instance, method, path string, body any) int {
	resp := c.do(method, path, body)
	_ = resp.Body.Close()
	return resp.StatusCode
}

// With a second step, the password opens no session: a code from the app does,
// once, and the challenge it finished is spent.
func TestThePasswordAloneOpensNoSessionWithASecondStep(t *testing.T) {
	i := newInstance(t)
	i.claim()
	secret, _ := i.turnOnTwoStep()

	browser := i.anonymous()
	challenge := passwordStep(t, browser)
	if got := statusOf(browser, http.MethodGet, "/api/me", nil); got != http.StatusUnauthorized {
		t.Fatalf("signed in before the code: %d", got)
	}
	if got := statusOf(browser, http.MethodPost, "/api/session/second-step", map[string]string{"challenge": challenge, "code": "000000"}); got != http.StatusUnauthorized {
		t.Errorf("a wrong code: %d", got)
	}
	i.tick.advance(totp.Period)
	code := totp.Code(secret, i.now())
	if got := statusOf(browser, http.MethodPost, "/api/session/second-step", map[string]string{"challenge": challenge, "code": code}); got != http.StatusOK {
		t.Fatalf("the right code: %d", got)
	}
	if got := statusOf(browser, http.MethodGet, "/api/me", nil); got != http.StatusOK {
		t.Errorf("not signed in after the code: %d", got)
	}
	// The same challenge, and the same code in a new one, are both spent.
	if got := statusOf(i.anonymous(), http.MethodPost, "/api/session/second-step", map[string]string{"challenge": challenge, "code": code}); got != http.StatusUnauthorized {
		t.Errorf("a spent challenge: %d", got)
	}
	other := i.anonymous()
	again := passwordStep(t, other)
	if got := statusOf(other, http.MethodPost, "/api/session/second-step", map[string]string{"challenge": again, "code": code}); got != http.StatusUnauthorized {
		t.Errorf("a code used twice: %d", got)
	}
}

// A recovery code stands in for the phone, once.
func TestARecoveryCodeSignsInOnce(t *testing.T) {
	i := newInstance(t)
	i.claim()
	_, codes := i.turnOnTwoStep()

	b := i.anonymous()
	if got := statusOf(b, http.MethodPost, "/api/session/second-step", map[string]string{"challenge": passwordStep(t, b), "code": codes[0]}); got != http.StatusOK {
		t.Fatalf("a recovery code: %d", got)
	}
	c := i.anonymous()
	if got := statusOf(c, http.MethodPost, "/api/session/second-step", map[string]string{"challenge": passwordStep(t, c), "code": codes[0]}); got != http.StatusUnauthorized {
		t.Errorf("the same recovery code twice: %d", got)
	}
}

// A sign-in waits five minutes for its code, and takes five wrong ones.
func TestAChallengeRunsOutOfTimeAndOfTries(t *testing.T) {
	i := newInstance(t)
	i.claim()
	secret, _ := i.turnOnTwoStep()

	late := i.anonymous()
	challenge := passwordStep(t, late)
	i.tick.advance(twostep.ChallengeLifetime + time.Second)
	if got := statusOf(late, http.MethodPost, "/api/session/second-step", map[string]string{"challenge": challenge, "code": totp.Code(secret, i.now())}); got != http.StatusUnauthorized {
		t.Errorf("a code after the challenge ran out: %d", got)
	}

	guesser := i.anonymous()
	challenge = passwordStep(t, guesser)
	for n := 0; n < twostep.ChallengeAttempts; n++ {
		_ = statusOf(guesser, http.MethodPost, "/api/session/second-step", map[string]string{"challenge": challenge, "code": "000000"})
	}
	i.tick.advance(totp.Period)
	if got := statusOf(guesser, http.MethodPost, "/api/session/second-step", map[string]string{"challenge": challenge, "code": totp.Code(secret, i.now())}); got == http.StatusOK {
		t.Error("the right code after five wrong ones still signed in")
	}
}

// Turning it off asks for the password; afterwards the password alone signs in.
func TestTurningItOffAsksForThePassword(t *testing.T) {
	i := newInstance(t)
	i.claim()
	i.turnOnTwoStep()

	if got := statusOf(i, http.MethodPost, "/api/me/two-step/disable", map[string]string{"current": "not the passphrase"}); got != http.StatusForbidden {
		t.Errorf("a wrong password turned it off: %d", got)
	}
	if got := statusOf(i, http.MethodPost, "/api/me/two-step/recovery-codes", map[string]string{"current": "not the passphrase"}); got != http.StatusForbidden {
		t.Errorf("a wrong password drew new codes: %d", got)
	}
	if got := statusOf(i, http.MethodPost, "/api/me/two-step/disable", map[string]string{"current": ownerPassword}); got != http.StatusNoContent {
		t.Fatalf("turning it off: %d", got)
	}
	b := i.anonymous()
	resp := b.do(http.MethodPost, "/api/session", map[string]string{"username": "owner", "password": ownerPassword})
	if got := decode(t, resp); got["two_step"] == true {
		t.Error("still asked for a code after turning it off")
	}
}

// Only the administrator takes it off somebody else.
func TestOnlyTheAdministratorTakesItOffSomebodyElse(t *testing.T) {
	i := newInstance(t)
	i.claim()
	i.turnOnTwoStep()
	var ownerID int64
	if err := i.db().QueryRow(`SELECT id FROM app_user WHERE username = 'owner'`).Scan(&ownerID); err != nil {
		t.Fatal(err)
	}
	if _, err := (account.Store{DB: i.db()}).Create("member", ownerPassword, false, i.now()); err != nil {
		t.Fatal(err)
	}
	member := i.anonymous()
	if got := statusOf(member, http.MethodPost, "/api/session", map[string]string{"username": "member", "password": ownerPassword}); got != http.StatusOK {
		t.Fatalf("member sign-in: %d", got)
	}
	path := "/api/people/" + itoa(ownerID) + "/two-step"
	if got := statusOf(member, http.MethodDelete, path, nil); got != http.StatusForbidden {
		t.Errorf("a member took it off the administrator: %d", got)
	}
	if got := statusOf(i, http.MethodDelete, path, nil); got != http.StatusNoContent {
		t.Fatalf("the administrator: %d", got)
	}
	if on, _ := (twostep.Store{DB: i.db()}).Enabled(ownerID); on {
		t.Error("still on after the administrator took it off")
	}
}
