// Command lasthelp is the server: the public portal, the panel and the bridge
// to whatever forge the operator connected.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/config"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/handoff"
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

	// `lasthelp password reset <username>` sets a forgotten password, from the
	// terminal of the machine the server runs on.
	if len(os.Args) > 1 && os.Args[1] == "password" {
		os.Exit(runPassword(os.Args[2:], console{fd: int(os.Stdin.Fd())}, os.Stdout, os.Stderr))
	}
	// `lasthelp 2fa reset <username>` takes the second step of signing in off an
	// account whose phone and recovery codes are both lost.
	if len(os.Args) > 1 && os.Args[1] == "2fa" {
		os.Exit(runTwoStep(os.Args[2:], os.Stdout, os.Stderr))
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
	// Looks for a newer version now and then, and says so through the server's channel
	// if an administrator asked to be told. Does nothing otherwise.
	go server.WatchUpdates(workCtx)

	// A process that replaced another one is handed its listening socket, so the
	// port never closed while it started; otherwise it listens as it always did.
	ln := handoff.Inherit(cfg.ListenFD, cfg.Addr)
	if ln == nil {
		if ln, err = net.Listen("tcp", cfg.Addr); err != nil {
			slog.Error("could not listen", "addr", cfg.Addr, "error", err)
			os.Exit(1)
		}
	} else {
		slog.Info("took over the listening socket of the process this one replaced", "addr", ln.Addr().String())
	}

	if err := serve(web.HTTPServer(cfg, server), ln); err != nil {
		slog.Error("the server stopped", "error", err)
		os.Exit(1)
	}
	slog.Info("stopped cleanly")
}

// errHandedOver says the server stopped accepting because its listening socket
// was given to the process that is about to replace this one.
var errHandedOver = errors.New("the listening socket was handed to the next process")

// serve runs until a signal arrives, then gives in-flight requests time to
// finish. A container stop that severs a request half way through writing is
// how a database ends up with an item nobody can find and a reporter with a
// follow link that leads nowhere.
//
// When an update replaces the binary, the server is readied to be replaced
// instead: it stops accepting, answers what is in flight and gives its socket to
// the new process (see handoff). Serving then ends, and the process has to stay
// until the exec that follows replaces it. Returning here would end it first, and
// the port with it.
func serve(srv *http.Server, ln net.Listener) error {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	serveOn, ready := handoff.Wrap(srv, ln)
	handoff.Register(ready)

	errs := make(chan error, 1)
	go func() {
		err := srv.Serve(serveOn)
		switch {
		case err == nil, errors.Is(err, http.ErrServerClosed):
			errs <- nil
		case errors.Is(err, net.ErrClosed):
			errs <- errHandedOver
		default:
			errs <- err
		}
	}()

	select {
	case err := <-errs:
		if errors.Is(err, errHandedOver) {
			// If the exec fails, whoever asked for it exits with an error. Until
			// then only a signal ends this process.
			sig := <-stop
			slog.Info("stopping while being replaced", "signal", sig.String())
			return nil
		}
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
