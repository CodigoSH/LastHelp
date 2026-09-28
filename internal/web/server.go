// Package web serves the HTTP interface: the public portal and the panel.
package web

import (
	"database/sql"
	"embed"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"mime"
	"net/http"
	"path"
	"strings"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/account"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/item"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/org"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/outbox"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/session"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/settings"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/config"
	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/outbound"
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

	// hooks is where a forge delivers, on a shorter chain: no session, no CSRF
	// and no same-site check, because a forge has none of those and its
	// signature is what authenticates it instead.
	hooks       *http.ServeMux
	hookHandler http.Handler

	// voterKey signs the cookie that identifies a voter. Read once at startup,
	// generated on the first ever start.
	voterKey []byte

	// formKey signs the token a report form carries. A separate key from the
	// voter one: two things that are not the same secret should not be the
	// same secret, so that leaking one does not hand over the other.
	formKey []byte

	// Two limiters, because an address alone means nothing in IPv6 — a single
	// customer holds a /64 — and a block alone would punish a whole office for
	// one person. Both are bounded: the keys are chosen by whoever is calling.
	perAddress *middleware.Limiter
	perBlock   *middleware.Limiter

	// box seals the forge credentials. Its key lives in a file beside the
	// database rather than in it, so a leaked backup is not write access to
	// somebody's forge.

	// out is the one way to somebody else's server.
	out *outbound.Client

	// shell is index.html, read once. Every request stamps a language into a
	// copy of it.
	shell string

	// Clock is the one source of time in the HTTP layer, replaceable so a test
	// can place a session in the past without sleeping.
	Clock func() time.Time

	// startedAt is when this process came up, for the one screen that reports
	// how long it has been running. Taken from the clock like everything else,
	// so a test can say what it is.
	startedAt time.Time
}

// New wires the routes and returns a server ready to be handed to net/http.
func New(cfg config.Config, db *sql.DB) (*Server, error) {
	// Go's table does not know the web manifest's extension, so the file server
	// sniffed it as plain text. Browsers forgive that today; the standard type
	// is what they are entitled to, and it costs one line.
	if err := mime.AddExtensionType(".webmanifest", "application/manifest+json"); err != nil {
		return nil, err
	}

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
	s.startedAt = s.now()

	// Generated on the first ever start and kept. Regenerating it would
	// invalidate every voter identity at once — everybody could vote again on
	// everything — and nothing would look broken.
	if s.voterKey, err = (settings.Store{DB: db}).Secret(settings.VoterSigningKey); err != nil {
		return nil, err
	}
	if s.formKey, err = (settings.Store{DB: db}).Secret(settings.FormSigningKey); err != nil {
		return nil, err
	}

	shell, err := fs.ReadFile(assets, "index.html")
	if err != nil {
		// The binary embeds whatever the frontend build produced. Without an
		// index there is no application, and failing at startup says so once
		// rather than 404ing for ever.
		return nil, fmt.Errorf("the embedded frontend has no index.html: %w", err)
	}
	s.shell = string(shell)
	// The head is written into the title's place. A build whose index lost
	// that line would serve every page untitled and unpreviewable without a
	// word, so it is refused here instead.
	if !strings.Contains(s.shell, shellTitle) {
		return nil, fmt.Errorf("the embedded index.html has no %s to write the page head into", shellTitle)
	}

	// Secrets are stored as they are given, and the one thing left that knows
	// how to open a sealed value is the migration that empties the old columns.
	if err := unsealOnce(db, cfg.KeyFile); err != nil {
		return nil, err
	}
	s.out = outbound.New(cfg.Offline, 20*time.Second)

	s.perAddress = middleware.NewLimiter(240, time.Minute, 20_000, s.now)
	s.perBlock = middleware.NewLimiter(600, time.Minute, 20_000, s.now)

	s.routes()
	// The webhook endpoint is not on the application's mux at all.
	//
	// A forge cannot sign in, sends no CSRF token, and is cross-site by
	// definition — it is somebody else's server. Every one of those checks
	// would refuse it, and correctly. So it lives on its own handler with its
	// own, shorter chain, and its authentication is the signature: an opaque
	// address identifies the connection, and the connection says which verifier
	// to use.
	//
	// It is separated here, where the routes are declared, rather than by a
	// path exemption inside a middleware. An exemption list is invisible from
	// the place people add routes, which is how something ends up outside a
	// check nobody remembered was there.
	s.hooks = http.NewServeMux()
	s.hooks.HandleFunc("POST /hooks/{hook}", s.handleWebhook)

	s.handler = s.wrap()
	s.hookHandler = middleware.Recover(
		middleware.SecurityHeaders(s.isSecure)(
			middleware.BodyLimit(webhookBodyLimit)(
				middleware.RateLimit(s.perAddress, s.perBlock, s.addressOf)(s.hooks))))
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
	// The name, the mark and the colour are on every public page, so reading
	// them is public. Changing them is not, and that is the other route.
	s.mux.HandleFunc("GET /api/instance", s.handleGetInstance)
	s.mux.HandleFunc("POST /api/setup", s.handleSetup)
	s.mux.HandleFunc("POST /api/session", s.handleSignIn)

	s.mux.HandleFunc("GET /api/orgs", s.handleOrganizations)

	// The composition of the front page. Readable by anybody, for the same
	// reason the name and the mark are: it IS the page a visitor is looking at.
	// Changing it is the administrator's, and that half is wrapped below.
	s.mux.HandleFunc("GET /api/landing", s.handleGetLanding)
	// The search on the front page, public for the same reason: it is how a
	// visitor finds a report before writing it again.
	s.mux.HandleFunc("GET /api/search", s.handleSearch)
	// Public for the same reason the page is: a picture on the front page that
	// asked for a session would be a broken box for everybody who is not the
	// operator.
	s.mux.HandleFunc("GET /api/landing/images/{id}", s.handleGetLandingImage)
	s.mux.HandleFunc("GET /api/orgs/{org}/repos", s.handleRepositories)
	s.mux.HandleFunc("GET /api/orgs/{org}/repos/{repo}", s.handleRepository)
	s.mux.HandleFunc("GET /api/orgs/{org}/repos/{repo}/items", s.handleListItems)
	s.mux.HandleFunc("POST /api/orgs/{org}/repos/{repo}/items", s.handleSubmit)
	s.mux.HandleFunc("GET /api/orgs/{org}/repos/{repo}/similar", s.handleSimilar)
	s.mux.HandleFunc("GET /api/orgs/{org}/repos/{repo}/form", s.handleForm)
	s.mux.HandleFunc("GET /api/orgs/{org}/repos/{repo}/roadmap", s.handleRoadmap)

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
	private("GET /api/server", s.handleServerInfo)
	private("GET /api/orgs/{org}/repos/{repo}/comments", s.handleCommentQueue)

	// The same queue without having to name a board first. The per-board one
	// stays: it is what somebody working on one board wants.
	private("GET /api/panel/comments", s.handleAllWaitingComments)
	private("GET /api/orgs/{org}/repos/{repo}/export", s.handleExport)

	private("POST /api/items/{id}/moderation", s.handleModerate)

	// Separate from moderation on purpose: one endpoint that could change the
	// text and the state is one where an authorisation mistake costs both.
	private("PATCH /api/items/{id}", s.handleEditItem)

	// Composing the front page. Only the administrator: it is the first thing
	// every visitor sees.
	private("PUT /api/landing", s.handleSaveLanding)
	private("POST /api/landing/restore", s.handleRestoreLandingDefault)
	private("POST /api/landing/images", s.handleAddLandingImage)
	private("POST /api/comments/{id}/moderation", s.handleModerateComment)
	private("GET /api/items/{id}/statuses", s.handleStatuses)
	private("POST /api/items/{id}/status", s.handleSetStatus)
	private("POST /api/items/{id}/merge", s.handleMerge)
	private("POST /api/items/{id}/reply", s.handleReply)
	private("POST /api/items/{id}/issue", s.handleCreateIssue)
	private("POST /api/items/{id}/attach", s.handleAttachIssue)
	private("POST /api/orgs/{org}/credentials", s.handleSetCredentials)
	private("POST /api/orgs/{org}/repos/{repo}/channel", s.handleSetNotifyChannel)

	private("POST /api/me/password", s.handleChangePassword)
	private("POST /api/me/profile", s.handleChangeProfile)
	private("POST /api/me/language", s.handleChangeLanguage)
	private("POST /api/me/avatar", s.handleChangeAvatar)

	private("GET /api/overview", s.handleOverview)
	private("GET /api/reports", s.handleReports)

	private("PATCH /api/orgs/{org}", s.handleRenameOrganization)
	private("DELETE /api/orgs/{org}", s.handleDeleteOrganization)
	private("PATCH /api/orgs/{org}/repos/{repo}", s.handleUpdateRepository)
	// A repository's page in the panel, and the team of an organisation: who
	// works where, changed by whoever runs it.
	private("GET /api/orgs/{org}/repos/{repo}/panel", s.handleRepositoryPanel)
	private("GET /api/panel/organizations", s.handleOrganizationsPanel)
	private("GET /api/orgs/{org}/panel", s.handleOrganizationPanel)
	private("GET /api/orgs/{org}/team", s.handleTeam)
	private("POST /api/orgs/{org}/team", s.handleTeamChange)
	private("DELETE /api/orgs/{org}/repos/{repo}", s.handleDeleteRepository)

	private("PATCH /api/people/{id}", s.handleUpdatePerson)
	private("DELETE /api/people/{id}", s.handleDeletePerson)

	private("POST /api/restore", s.handleRestore)

	private("GET /api/people", s.handleListPeople)
	private("POST /api/people", s.handleCreatePerson)
	private("POST /api/people/{id}/grants", s.handleGrant)

	private("GET /api/backup", s.handleBackup)

	private("POST /api/instance", s.handleSetInstance)

	private("GET /api/update", s.handleUpdateStatus)
	private("POST /api/update", s.handleUpdatePerform)
	private("POST /api/update/channel", s.handleSetChannel)

	// The feed, under its own prefix.
	//
	// The obvious address — /{org}/{repo}/feed.atom — is ambiguous against
	// /api/items/{id}, and Go's mux says so at registration rather than picking
	// one at random. Which is the right behaviour and the reason this is not a
	// bug discovered by somebody whose repository happens to be called "items".
	s.mux.HandleFunc("GET /feed/{org}/{repo}/atom.xml", s.handleFeed)

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

		// A path with no extension is a client-side route, not a file. It gets
		// the shell, stamped with a language, rather than a 404.
		clean := path.Clean(r.URL.Path)
		if clean == "/" || path.Ext(clean) == "" {
			s.serveShell(w, r)
			return
		}
		files.ServeHTTP(w, r)
	})
}

// serveShell writes index.html with the language and reading direction filled
// in, so the first paint is already right.
//
// Leaving lang="en" on the document is not cosmetic: it is what a screen reader
// announces, and what the browser hyphenates and spellchecks by. And dir has to
// be there before the first pixel, or an Arabic page renders left-to-right and
// then jumps.
func (s *Server) serveShell(w http.ResponseWriter, r *http.Request) {
	lang := negotiateLanguage(r.Header.Get("Accept-Language"))

	shell := strings.NewReplacer(
		`lang="en"`, `lang="`+lang+`"`,
		`dir="ltr"`, `dir="`+direction(lang)+`"`,
		shellTitle, s.head(),
	).Replace(s.shell)

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// The shell carries the language of whoever asked, so a shared cache must
	// not hand an Arabic document to the next person.
	w.Header().Set("Vary", "Accept-Language")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = io.WriteString(w, shell)
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
	h = auth.Attach(h)                       // who is asking
	h = middleware.CSRF(s.cfg.TrustProxy)(h) // and did the page ask
	h = middleware.CrossSite(h)              // and was it this site
	// How busy this caller has been. Not the anti-spam layer — the form token
	// and the queue are — this is the ceiling that stops one caller spending
	// the whole server on itself.
	h = middleware.RateLimit(s.perAddress, s.perBlock, s.addressOf)(h)
	h = middleware.BodyLimit(GlobalBodyLimit)(h) // and is it a sane size
	// Outside the rate limiter, so a refused request is answered with the same
	// policy as a served one: a 429 that arrives without a CSP is a page the
	// browser will happily run anything in.
	h = middleware.SecurityHeaders(s.isSecure)(h)
	h = middleware.Recover(h) // and if it explodes, say so
	return h
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// One branch, in the open, rather than an exemption buried in a middleware.
	// Everything not under /hooks/ goes through the full chain.
	if strings.HasPrefix(r.URL.Path, "/hooks/") {
		s.hookHandler.ServeHTTP(w, r)
		return
	}
	s.handler.ServeHTTP(w, r)
}

func (s *Server) accounts() account.Store  { return account.Store{DB: s.db} }
func (s *Server) sessions() session.Store  { return session.Store{DB: s.db} }
func (s *Server) orgs() org.Store          { return org.Store{DB: s.db} }
func (s *Server) items() item.Store        { return item.Store{DB: s.db} }
func (s *Server) settings() settings.Store { return settings.Store{DB: s.db} }
func (s *Server) outbox() outbox.Store     { return outbox.Store{DB: s.db} }

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
