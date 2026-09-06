// Package web serves the HTTP interface: the public portal and the panel.
package web

import (
	"database/sql"
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/config"
	"git.codigosh.com/CodigoSH/LastHelp/internal/version"
)

// The frontend is built into internal/web/dist by web/build.ts and embedded
// here. go:embed cannot reach outside its own package directory, which is why
// the build mirrors web/dist into this one rather than embedding it in place.
//
//go:embed all:dist
var content embed.FS

// Server holds what every handler needs. It is deliberately small: anything
// that grows here is usually something that belongs in core.
type Server struct {
	cfg    config.Config
	db     *sql.DB
	mux    *http.ServeMux
	assets fs.FS
}

// New wires the routes and returns a server ready to be handed to net/http.
func New(cfg config.Config, db *sql.DB) (*Server, error) {
	assets, err := fs.Sub(content, "dist")
	if err != nil {
		return nil, err
	}

	s := &Server{cfg: cfg, db: db, mux: http.NewServeMux(), assets: assets}
	s.routes()
	return s, nil
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.Handle("GET /", http.FileServerFS(s.assets))
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// handleHealth answers the container healthcheck. It says the version so that
// "is the new binary actually running?" is one request rather than a guess.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write([]byte(`{"status":"ok","version":"` + version.Current + `"}`))
}

// HTTPServer builds the net/http server.
//
// Only ReadHeaderTimeout is set, on purpose. WriteTimeout and IdleTimeout are
// absolute deadlines that cut long-lived connections — a feed being streamed, a
// slow forge call being awaited — and they have cut them before in this house.
// ReadHeaderTimeout is what actually covers slow-loris, and it is not affected.
// The write deadline comes back later as middleware that re-arms it on every
// write, which protects against a client that stops reading without punishing
// a response that legitimately takes a while to produce.
func HTTPServer(cfg config.Config, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              cfg.Addr,
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		ErrorLog:          slog.NewLogLogger(slog.Default().Handler(), slog.LevelWarn),
	}
}
