package middleware

import (
	"fmt"
	"testing"
	"time"
)

func TestABurstIsAllowedAndThenRefused(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	l := NewLimiter(3, time.Minute, 100, func() time.Time { return now })

	for i := 0; i < 3; i++ {
		if !l.Allow("a") {
			t.Fatalf("request %d of the burst was refused", i+1)
		}
	}
	if l.Allow("a") {
		t.Error("the fourth request of a burst of three was allowed")
	}
}

func TestTheAllowanceComesBack(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	l := NewLimiter(3, time.Minute, 100, func() time.Time { return now })

	for i := 0; i < 3; i++ {
		l.Allow("a")
	}
	now = now.Add(30 * time.Second) // half the window
	if !l.Allow("a") {
		t.Error("nothing came back after half the window; a limiter that never refills is a ban")
	}
}

func TestOneAddressDoesNotSpendAnothersAllowance(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	l := NewLimiter(2, time.Minute, 100, func() time.Time { return now })

	l.Allow("a")
	l.Allow("a")
	if !l.Allow("b") {
		t.Error("a busy address used up somebody else's allowance")
	}
}

// TestTheMapIsBounded is the test this file exists for.
//
// The keys are chosen by whoever is calling. With IPv6 a single customer holds
// eighteen quintillion addresses, so an unbounded map indexed by address is a
// way to fill this process's memory from outside — and it would look like a
// slow leak rather than an attack.
func TestTheMapIsBounded(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	const ceiling = 50
	l := NewLimiter(2, time.Minute, ceiling, func() time.Time { return now })

	for i := 0; i < 5_000; i++ {
		l.Allow(fmt.Sprintf("2001:db8::%x", i))
	}

	if got := l.Len(); got > ceiling {
		t.Errorf("tracking %d keys with a ceiling of %d: minting addresses fills memory", got, ceiling)
	}
}

// TestTheBusiestKeySurvivesEviction.
//
// Eviction has to drop the ones nobody is using, not the one doing the
// flooding — otherwise the flood evicts its own record and gets a fresh
// allowance every time round.
func TestTheBusiestKeySurvivesEviction(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	l := NewLimiter(2, time.Minute, 10, func() time.Time { return now })

	// Spend the flooder's allowance.
	l.Allow("flooder")
	l.Allow("flooder")

	// Push through more distinct keys than the ceiling, touching the flooder
	// along the way as a real flood would.
	for i := 0; i < 50; i++ {
		l.Allow(fmt.Sprintf("passer-%d", i))
		if l.Allow("flooder") {
			t.Fatalf("the flooder got a fresh allowance after %d evictions", i+1)
		}
	}
}
