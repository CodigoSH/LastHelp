package web

import (
	"net/http"
	"testing"
	"time"
)

// Five wrong guesses at one name lock it for five minutes, from whichever
// address they come, and the right password is refused while it lasts: a lock
// that opens for the correct guess tells the guesser when they got it.
func TestRepeatedWrongPasswordsLockTheAccount(t *testing.T) {
	team := newInstance(t)
	team.claim()
	signIn := func(password string) *http.Response {
		return team.anonymous().do(http.MethodPost, "/api/session", map[string]string{
			"username": "owner", "password": password,
		})
	}

	for i := 0; i < 5; i++ {
		if got := status(t, signIn("not the passphrase")); got != http.StatusUnauthorized {
			t.Fatalf("wrong guess %d answered %d", i+1, got)
		}
	}

	locked := signIn("a good long passphrase")
	if locked.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("the right password after five failures answered %d, want 429", locked.StatusCode)
	}
	if locked.Header.Get("Retry-After") == "" {
		t.Error("the refusal does not say when to come back")
	}

	team.tick.advance(5*time.Minute + time.Second)
	if got := status(t, signIn("a good long passphrase")); got != http.StatusOK {
		t.Errorf("after the lock ran out the right password answered %d", got)
	}
}

func TestANameThatDoesNotExistIsLockedTheSameWay(t *testing.T) {
	team := newInstance(t)
	team.claim()
	try := func() int {
		return status(t, team.anonymous().do(http.MethodPost, "/api/session", map[string]string{
			"username": "nobody-here", "password": "a good long passphrase",
		}))
	}
	for i := 0; i < 5; i++ {
		_ = try()
	}
	if got := try(); got != http.StatusTooManyRequests {
		t.Errorf("a name with no account answered %d after five tries, so the lock says which names exist", got)
	}
}
