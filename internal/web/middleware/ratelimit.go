package middleware

import (
	"container/list"
	"net/http"
	"sync"
	"time"
)

// Limiter is a token bucket per key, with a hard ceiling on how many keys it
// will ever hold.
//
// The ceiling is the point. A map indexed by client address is a map whose keys
// are chosen by whoever is calling, and with IPv6 that is effectively unlimited:
// a single customer holds a /64, so somebody minting a fresh address per request
// fills memory rather than hitting a limit. When the ceiling is reached the
// least recently used key is dropped — which costs an attacker nothing, and
// costs the process nothing either, which is the trade that matters.
type Limiter struct {
	mu      sync.Mutex
	entries map[string]*list.Element
	order   *list.List // most recent at the front
	max     int

	// burst is how many requests may arrive at once; refill is how fast the
	// allowance comes back.
	burst  float64
	refill float64 // tokens per second

	now func() time.Time
}

type bucket struct {
	key    string
	tokens float64
	seen   time.Time
}

// NewLimiter builds one. maxKeys is the ceiling on distinct addresses tracked.
func NewLimiter(burst int, per time.Duration, maxKeys int, now func() time.Time) *Limiter {
	if now == nil {
		now = time.Now
	}
	if maxKeys <= 0 {
		maxKeys = 10_000
	}
	rate := float64(burst) / per.Seconds()
	return &Limiter{
		entries: make(map[string]*list.Element, maxKeys),
		order:   list.New(),
		max:     maxKeys,
		burst:   float64(burst),
		refill:  rate,
		now:     now,
	}
}

// Allow spends one token for a key, or refuses.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := l.now()

	if element, ok := l.entries[key]; ok {
		l.order.MoveToFront(element)
		b := element.Value.(*bucket)
		b.tokens = min(l.burst, b.tokens+now.Sub(b.seen).Seconds()*l.refill)
		b.seen = now
		if b.tokens < 1 {
			return false
		}
		b.tokens--
		return true
	}

	// New key. Evict the least recently used one if the ceiling is reached,
	// rather than growing without bound.
	if l.order.Len() >= l.max {
		oldest := l.order.Back()
		if oldest != nil {
			l.order.Remove(oldest)
			delete(l.entries, oldest.Value.(*bucket).key)
		}
	}

	l.entries[key] = l.order.PushFront(&bucket{key: key, tokens: l.burst - 1, seen: now})
	return true
}

// Len is how many keys are being tracked, for the test that proves the ceiling.
func (l *Limiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.order.Len()
}

// RateLimit refuses a request whose address has been too busy.
//
// Two limiters, not one: the exact address is allowed a generous burst because
// a whole office or a mobile carrier shares one, and the surrounding block is
// limited on top because an address by itself means nothing in IPv6.
func RateLimit(exact, block *Limiter, addressOf func(*http.Request) (string, string)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			address, group := addressOf(r)
			if !exact.Allow(address) || !block.Allow(group) {
				w.Header().Set("Retry-After", "60")
				http.Error(w, "error.too_many", http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}
