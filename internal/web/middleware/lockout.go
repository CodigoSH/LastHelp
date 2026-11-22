package middleware

import (
	"sync"
	"time"
)

// Lockout stops guessing at one account.
//
// The per-address limiters count requests, and a patient caller spreads guesses
// over many addresses. This counts failures against the name being tried, from
// wherever they come: after too many inside a window, that name is refused for a
// while — even for the right password, or the lock would only slow the guesser
// down while telling them when they got it.
//
// A name that does not exist is counted the same as one that does, so the
// refusal does not say which names have an account.
//
// Like the limiter it is bounded: the keys are chosen by whoever is calling.
type Lockout struct {
	mu      sync.Mutex
	entries map[string]*lockEntry
	max     int

	failures int
	window   time.Duration
	lock     time.Duration

	now func() time.Time
}

type lockEntry struct {
	count       int
	windowStart time.Time
	lockedUntil time.Time
}

// NewLockout locks a key for lock after failures failed attempts inside window.
func NewLockout(failures int, window, lock time.Duration, maxKeys int, now func() time.Time) *Lockout {
	if now == nil {
		now = time.Now
	}
	if maxKeys <= 0 {
		maxKeys = 10_000
	}
	return &Lockout{
		entries:  make(map[string]*lockEntry),
		max:      maxKeys,
		failures: failures,
		window:   window,
		lock:     lock,
		now:      now,
	}
}

// Locked says whether the key is refused right now, and for how much longer.
func (l *Lockout) Locked(key string) (time.Duration, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()

	e, ok := l.entries[key]
	if !ok {
		return 0, false
	}
	if left := e.lockedUntil.Sub(l.now()); left > 0 {
		return left, true
	}
	return 0, false
}

// Fail records a failed attempt.
func (l *Lockout) Fail(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()
	e, ok := l.entries[key]
	if !ok {
		if len(l.entries) >= l.max {
			l.makeRoom(now)
		}
		e = &lockEntry{windowStart: now}
		l.entries[key] = e
	}
	if now.Sub(e.windowStart) > l.window {
		e.count, e.windowStart = 0, now
	}
	e.count++
	if e.count >= l.failures {
		e.lockedUntil = now.Add(l.lock)
		e.count, e.windowStart = 0, now
	}
}

// Succeed forgets the failures against a key.
func (l *Lockout) Succeed(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}

// makeRoom drops what no longer matters, and if everything does, the entry
// whose window started longest ago.
func (l *Lockout) makeRoom(now time.Time) {
	var oldestKey string
	var oldest time.Time
	for key, e := range l.entries {
		if now.After(e.lockedUntil) && now.Sub(e.windowStart) > l.window {
			delete(l.entries, key)
			continue
		}
		if oldestKey == "" || e.windowStart.Before(oldest) {
			oldestKey, oldest = key, e.windowStart
		}
	}
	if len(l.entries) >= l.max && oldestKey != "" {
		delete(l.entries, oldestKey)
	}
}
