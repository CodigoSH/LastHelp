package web

import (
	"context"
	"log/slog"
	"time"
)

// WorkEvery is how often the queue is drained.
//
// Ten seconds, not one: a report becoming an issue ten seconds later is
// invisible to everybody, and polling a database ten times a second for work
// that arrives a few times a day is a busy loop with a nicer name.
const WorkEvery = 10 * time.Second

// Work drains the outbound queue until the context is cancelled.
//
// It also sweeps what has expired. Both are here rather than in separate
// goroutines because they are the same kind of thing — periodic tidying — and
// one timer is easier to reason about than three.
func (s *Server) Work(ctx context.Context) {
	ticker := time.NewTicker(WorkEvery)
	defer ticker.Stop()

	var sweptAt time.Time

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}

		now := s.now()

		if done, err := s.bridge().Run(ctx, now, 10); err != nil {
			slog.Error("draining the outbound queue", "error", err)
		} else if done > 0 {
			slog.Info("forge jobs done", "count", done)
		}

		// Hourly, and only the parts that grow: spent form tokens, expired
		// sessions, old delivery ids and the pictures of closed reports. Each is a table indexed by something a
		// caller supplies, which is the definition of a table that needs a
		// ceiling.
		if now.Sub(sweptAt) < time.Hour {
			continue
		}
		sweptAt = now

		if err := s.guard().PurgeTokens(now); err != nil {
			slog.Error("sweeping form tokens", "error", err)
		}
		if err := s.sessions().Purge(now); err != nil {
			slog.Error("sweeping sessions", "error", err)
		}
		if err := s.bridge().PurgeDeliveries(now, 30*24*time.Hour); err != nil {
			slog.Error("sweeping deliveries", "error", err)
		}
		if n, err := s.items().PurgeImages(now); err != nil {
			slog.Error("sweeping pictures", "error", err)
		} else if n > 0 {
			slog.Info("pictures swept", "count", n)
		}
	}
}
