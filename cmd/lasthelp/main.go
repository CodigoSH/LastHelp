// Command lasthelp is the server: the public portal, the panel and the bridge
// to whatever forge the operator connected.
package main

import (
	"log/slog"
	"net/http"
	"os"

	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/config"
	"git.codigosh.com/CodigoSH/LastHelp/internal/version"
	"git.codigosh.com/CodigoSH/LastHelp/internal/web"
)

func main() {
	cfg := config.Load()
	setupLogger(cfg)

	slog.Info("starting LastHelp", "version", version.Current, "commit", version.Commit, "addr", cfg.Addr)

	server, err := web.New(cfg)
	if err != nil {
		slog.Error("could not build the server", "error", err)
		os.Exit(1)
	}

	if err := web.HTTPServer(cfg, server).ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("the server stopped", "error", err)
		os.Exit(1)
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
