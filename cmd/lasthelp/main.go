// Command lasthelp is the server: the public portal, the panel and the bridge
// to whatever forge the operator connected.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/config"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
	"git.codigosh.com/CodigoSH/LastHelp/internal/version"
	"git.codigosh.com/CodigoSH/LastHelp/internal/web"
)

func main() {
	// `lasthelp version` prints what this binary is, as JSON.
	//
	// It exists for the updater: before a downloaded binary replaces the
	// running one, it is executed with this and has to answer. That proves it
	// runs at all on this machine — right architecture, right libraries, not
	// truncated — and it reports the highest schema it understands, so a binary
	// older than the database can be refused before the swap rather than after.
	if len(os.Args) > 1 && os.Args[1] == "version" {
		fmt.Printf(`{"version":%q,"commit":%q,"schema":%d}`+"\n",
			version.Current, version.Commit, database.LatestSchema())
		return
	}

	cfg := config.Load()
	setupLogger(cfg)

	slog.Info("starting LastHelp",
		"version", version.Current, "commit", version.Commit, "addr", cfg.Addr, "database", cfg.DBFile)

	// Opening also migrates. A schema problem should stop the server here,
	// where the message is read, rather than on somebody's first request.
	db, err := database.Open(cfg.DBFile)
	if err != nil {
		slog.Error("could not open the database", "error", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	server, err := web.New(cfg, db)
	if err != nil {
		slog.Error("could not build the server", "error", err)
		os.Exit(1)
	}

	// The queue drains in the background. Nothing calls a forge from a request:
	// forges rate-limit and forges go down, and a report lost because somebody
	// else's server was having a bad afternoon is a report nobody writes twice.
	workCtx, stopWork := context.WithCancel(context.Background())
	defer stopWork()
	go server.Work(workCtx)

	if err := serve(web.HTTPServer(cfg, server)); err != nil {
		slog.Error("the server stopped", "error", err)
		os.Exit(1)
	}
	slog.Info("stopped cleanly")
}

// serve runs until a signal arrives, then gives in-flight requests time to
// finish. A container stop that severs a request half way through writing is
// how a database ends up with an item nobody can find and a reporter with a
// follow link that leads nowhere.
func serve(srv *http.Server) error {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	errs := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errs <- err
			return
		}
		errs <- nil
	}()

	select {
	case err := <-errs:
		return err
	case sig := <-stop:
		slog.Info("shutting down", "signal", sig.String())
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return srv.Shutdown(ctx)
	}
}

func setupLogger(cfg config.Config) {
	var handler slog.Handler
	if cfg.LogJSON {
		handler = slog.NewJSONHandler(os.Stdout, nil)
	} else {
		handler = slog.NewTextHandler(os.Stdout, nil)
	}
	slog.SetDefault(slog.New(handler))
}
