// Package web serves the HTTP interface: the public portal and the panel.
package web

import (
	"database/sql"
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/account"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/item"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/org"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/session"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/settings"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/config"
	"git.codigosh.com/CodigoSH/LastHelp/internal/version"
	"git.codigosh.com/CodigoSH/LastHelp/internal/web/middleware"
)

// The frontend is built into internal/web/dist by web/build.ts and embedded
// here. go:embed cannot reach outside its own package directory, which is why
// the build mirrors web/dist into this one rather than embedding it in place.
//
//go:embed all:dist
var content embed.FS

// GlobalBodyLimit is the backstop for a handler that forgot to set its own.
const GlobalBodyLimit = 2 << 20 // 2 MiB

// Server holds what every handler needs. It stays small on purpose: what grows
// here usually belongs in core.
type Server struct {
	cfg    config.Config
	db     *sql.DB
	mux    *http.ServeMux
	assets fs.FS

	// handler is the fully wrapped chain, built once. Building it per request
	// would allocate the whole stack on every hit for no reason.
	handler http.Handler

	// voterKey signs the cookie that identifies a voter. Read once at startup,
	// generated on the first ever start.
	voterKey []byte

	// Clock is the one source of time in the HTTP layer, replaceable so a test
	// can place a session in the past without sleeping.
	Clock func() time.Time
}

// New wires the routes and returns a server ready to be handed to net/http.
func New(cfg config.Config, db *sql.DB) (*Server, error) {
	assets, err := fs.Sub(content, "dist")
	if err != nil {
		return nil, err
	}

	s := &Server{
		cfg:    cfg,
		db:     db,
		mux:    http.NewServeMux(),
		assets: assets,
		Clock:  time.Now,
	}

	// Generated on the first ever start and kept. Regenerating it would
	// invalidate every voter identity at once — everybody could vote again on
	// everything — and nothing would look broken.
	if s.voterKey, err = (settings.Store{DB: db}).Secret(settings.VoterSigningKey); err != nil {
		return nil, err
	}

	s.routes()
	s.handler = s.wrap()
	return s, nil
}

// routes declares everything this server answers.
//
// A route is protected by being wrapped in RequireActor, and public by not
// being wrapped. There is deliberately no list of public paths anywhere: this
// file is the list, and it is the file somebody reads when they add a route.
func (s *Server) routes() {
	// --- Public -------------------------------------------------------------
	//
	// The setup screen has to be reachable before anybody exists, signing in
	// cannot require being signed in, and the whole portal is public by
	// definition: that is the product.
	s.mux.HandleFunc("GET /api/status", s.handleStatus)
	s.mux.HandleFunc("POST /api/setup", s.handleSetup)
	s.mux.HandleFunc("POST /api/session", s.handleSignIn)

	s.mux.HandleFunc("GET /api/orgs", s.handleOrganizations)
	s.mux.HandleFunc("GET /api/orgs/{org}/repos", s.handleRepositories)
	s.mux.HandleFunc("GET /api/orgs/{org}/repos/{repo}", s.handleRepository)
	s.mux.HandleFunc("GET /api/orgs/{org}/repos/{repo}/items", s.handleListItems)
	s.mux.HandleFunc("POST /api/orgs/{org}/repos/{repo}/items", s.handleSubmit)
	s.mux.HandleFunc("GET /api/orgs/{org}/repos/{repo}/similar", s.handleSimilar)

	s.mux.HandleFunc("GET /api/items/{id}", s.handleItem)
	s.mux.HandleFunc("POST /api/items/{id}/votes", s.handleVote)
	s.mux.HandleFunc("DELETE /api/items/{id}/votes", s.handleUnvote)
	s.mux.HandleFunc("POST /api/items/{id}/comments", s.handleComment)

	// --- Private ------------------------------------------------------------
	//
	// Wrapped, which is the only thing that makes a route private. Every one of
	// these then asks authz whether this particular actor may act on this
	// particular thing — being signed in is the door, not the permission.
	private := func(pattern string, h http.HandlerFunc) {
		s.mux.Handle(pattern, middleware.RequireActor(h))
	}

	private("DELETE /api/session", s.handleSignOut)
	private("GET /api/me", s.handleMe)

	private("POST /api/organizations", s.handleCreateOrganization)
	private("POST /api/orgs/{org}/repositories", s.handleCreateRepository)
	private("GET /api/orgs/{org}/repos/{repo}/queue", s.handleQueue)

	private("POST /api/items/{id}/moderation", s.handleModerate)
	private("POST /api/items/{id}/status", s.handleSetStatus)
	private("POST /api/items/{id}/merge", s.handleMerge)
	private("POST /api/items/{id}/reply", s.handleReply)

	// Public, and only these: the health check and the static frontend.
	s.mux.HandleFunc("GET /healthz", s.handleHealth)
	s.mux.Handle("GET /", s.static())
}

// static serves the built frontend.
//
// Anything that is not a file falls through to index.html, because the frontend
// routes client-side. That fall-through is exactly how this house once served
// the application shell without a session for every non-existent path under a
// prefix somebody had listed as public — which is why there is no such list
// here, and why nothing behind a session is served by this handler.
func (s *Server) static() http.Handler {
	files := http.FileServerFS(s.assets)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		files.ServeHTTP(w, r)
	})
}

// wrap puts everything that applies to every request around the routes,
// outermost first.
func (s *Server) wrap() http.Handler {
	auth := middleware.Authenticator{
		Sessions: s.sessions(),
		Accounts: s.accounts(),
		Now:      s.now,
	}

	var h http.Handler = s.mux
	h = auth.Attach(h)                           // who is asking
	h = middleware.CSRF(s.cfg.TrustProxy)(h)     // and did the page ask
	h = middleware.CrossSite(h)                  // and was it this site
	h = middleware.BodyLimit(GlobalBodyLimit)(h) // and is it a sane size
	h = middleware.Recover(h)                    // and if it explodes, say so
	return h
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.handler.ServeHTTP(w, r)
}

func (s *Server) accounts() account.Store  { return account.Store{DB: s.db} }
func (s *Server) sessions() session.Store  { return session.Store{DB: s.db} }
func (s *Server) orgs() org.Store          { return org.Store{DB: s.db} }
func (s *Server) items() item.Store        { return item.Store{DB: s.db} }
func (s *Server) settings() settings.Store { return settings.Store{DB: s.db} }

func (s *Server) now() time.Time {
	if s.Clock != nil {
		return s.Clock()
	}
	return time.Now()
}

// isSecure decides whether to mark cookies Secure.
//
// Behind a proxy that terminates TLS the connection to us is plain http and
// r.TLS is nil, so the forwarded header is the only signal — and it is trusted
// only when the operator said a proxy is in front. Believing it unconditionally
// would let a caller decide how our cookies are marked.
func (s *Server) isSecure(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	if s.cfg.TrustProxy && strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		return true
	}
	return false
}

// handleHealth answers the container healthcheck. It says the version so that
// "is the new binary actually running?" is one request rather than a guess.
func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
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
