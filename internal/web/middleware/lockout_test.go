package middleware

import (
	"fmt"
	"testing"
	"time"
)

type fakeClock struct{ t time.Time }

func (c *fakeClock) now() time.Time { return c.t }

func newTestLockout(c *fakeClock, maxKeys int) *Lockout {
	return NewLockout(5, time.Minute, 5*time.Minute, maxKeys, c.now)
}

func TestFiveFailuresInAMinuteLockTheName(t *testing.T) {
	c := &fakeClock{t: time.Unix(1_000_000, 0)}
	l := newTestLockout(c, 0)

	for i := 0; i < 4; i++ {
		l.Fail("ana")
		if _, locked := l.Locked("ana"); locked {
			t.Fatalf("locked after %d failures", i+1)
		}
	}
	l.Fail("ana")
	left, locked := l.Locked("ana")
	if !locked || left != 5*time.Minute {
		t.Fatalf("after five failures: locked=%v for %v, want locked for 5m", locked, left)
	}
	if _, other := l.Locked("luis"); other {
		t.Error("somebody else's name was locked")
	}

	c.t = c.t.Add(5*time.Minute + time.Second)
	if _, still := l.Locked("ana"); still {
		t.Error("still locked after the time was up")
	}
}

func TestSlowFailuresDoNotAddUp(t *testing.T) {
	c := &fakeClock{t: time.Unix(1_000_000, 0)}
	l := newTestLockout(c, 0)

	// A typo every 70 seconds, for ever: never five inside one minute.
	for i := 0; i < 20; i++ {
		l.Fail("ana")
		c.t = c.t.Add(70 * time.Second)
	}
	if _, locked := l.Locked("ana"); locked {
		t.Error("a typo now and then locked the account")
	}
}

func TestASuccessForgetsTheFailures(t *testing.T) {
	c := &fakeClock{t: time.Unix(1_000_000, 0)}
	l := newTestLockout(c, 0)

	for i := 0; i < 4; i++ {
		l.Fail("ana")
	}
	l.Succeed("ana")
	l.Fail("ana")
	if _, locked := l.Locked("ana"); locked {
		t.Error("four failures, a sign-in and one more locked the account")
	}
}

func TestTheLockoutHoldsABoundedNumberOfNames(t *testing.T) {
	c := &fakeClock{t: time.Unix(1_000_000, 0)}
	l := newTestLockout(c, 100)

	for i := 0; i < 5_000; i++ {
		l.Fail(fmt.Sprintf("name-%d", i))
		c.t = c.t.Add(time.Millisecond)
	}
	l.mu.Lock()
	n := len(l.entries)
	l.mu.Unlock()
	if n > 100 {
		t.Errorf("it holds %d names, the ceiling is 100", n)
	}
}
