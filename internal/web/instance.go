package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/landing"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/settings"
)

// instanceSettings is what an operator can change about the installation
// itself, as opposed to about one organisation.
type instanceSettings struct {
	Name string `json:"name"`
	// Logo is a data URL. Bounded because it is written into every page.
	Logo   string `json:"logo"`
	Accent string `json:"accent"`
}

// maxLogoBytes keeps the logo to something that belongs inline. Anything larger
// is a file, and a file is a thing to serve, cache and back up separately.
const maxLogoBytes = 96 << 10

// accentPattern is a hex colour and nothing else: this value goes into a style
// attribute, so anything that is not exactly six hex digits is refused rather
// than escaped and hoped about.
var accentPattern = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// maxInstanceNameLength is a heading, not a paragraph. Bounded here rather than
// only in the browser, where a bound is a suggestion.
const maxInstanceNameLength = 60

// handleGetInstance answers what this installation calls itself.
//
// Readable by anybody, because the name, the mark and the colour are on every
// public page anyway — a visitor who could not read them would be shown a page
// wearing somebody else's branding. Only changing them is the administrator's.
func (s *Server) handleGetInstance(w http.ResponseWriter, r *http.Request) {
	store := s.settings()
	name, err := store.Get(settings.InstanceName)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	logo, err := store.Get(settings.InstanceLogo)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	accent, err := store.Get(settings.InstanceAccent)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusOK, instanceSettings{Name: name, Logo: logo, Accent: accent})
}

// handleSetInstance changes it.
func (s *Server) handleSetInstance(w http.ResponseWriter, r *http.Request) {
	if !s.may(w, r, authz.ManageServer, authz.Scope{}) {
		return
	}

	// Large enough for the logo it is allowed to carry, plus the rest of the
	// object. It was panelBodyLimit — 16 KiB — while a logo may be 96, so the
	// body was cut off mid-JSON and the decode failed: finishing the wizard
	// with any logo over about 12 KB answered "the request was not understood",
	// which is true and useless. Two bounds on the same value, in two files,
	// disagreeing.
	r.Body = http.MaxBytesReader(w, r.Body, maxLogoBytes+(8<<10))
	var in instanceSettings
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		// A body that ran past the bound is an oversized logo, not a malformed
		// request: the only thing in here that can be large is the image.
		if tooLarge(err) {
			writeError(w, http.StatusBadRequest, "error.image_too_big")
			return
		}
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}

	name := strings.TrimSpace(in.Name)
	if len([]rune(name)) > maxInstanceNameLength {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}
	logo := strings.TrimSpace(in.Logo)
	if logo != "" && (!strings.HasPrefix(logo, "data:image/") || len(logo) > maxLogoBytes) {
		writeError(w, http.StatusBadRequest, "error.image_too_big")
		return
	}
	accent := strings.TrimSpace(in.Accent)
	if accent != "" && !accentPattern.MatchString(accent) {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}

	store := s.settings()
	for key, value := range map[string]string{
		settings.InstanceName:   name,
		settings.InstanceLogo:   logo,
		settings.InstanceAccent: accent,
	} {
		if err := store.Set(key, value); err != nil {
			writeError(w, http.StatusInternalServerError, "error.internal")
			return
		}
	}
	writeJSON(w, http.StatusOK, instanceSettings{Name: name, Logo: logo, Accent: accent})
}

// handleGetLanding answers what the front page is made of.
//
// Open, like the name and the mark: this is the page itself, and a visitor who
// could not read it would be shown nothing. An unset layout comes back as the
// default rather than as empty — an installation that never opened the editor
// has a front page, it just never chose one.
func (s *Server) handleGetLanding(w http.ResponseWriter, r *http.Request) {
	stored, err := s.settings().Get(settings.LandingLayout)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	layout, err := landing.Parse(stored)
	if err != nil {
		// Something unreadable is in the settings row. The front page is not
		// the place to find that out, so it falls back to the page everybody
		// had before and says so where somebody will see it.
		slog.Error("the stored front page layout could not be read", "error", err)
		layout = landing.Default()
	}
	writeJSON(w, http.StatusOK, layout)
}

// handleSetLanding replaces it.
//
// Validated by the domain, which refuses a layout it cannot draw entirely
// rather than saving the parts it recognised. Saving half of what somebody
// composed and showing them the rest missing is how an editor teaches people
// not to trust it.
func (s *Server) handleSetLanding(w http.ResponseWriter, r *http.Request) {
	if !s.may(w, r, authz.ManageServer, authz.Scope{}) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, panelBodyLimit)
	var in landing.Layout
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}

	encoded, err := in.Encode()
	if err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_layout")
		return
	}
	if err := s.settings().Set(settings.LandingLayout, encoded); err != nil {
		slog.Error("storing the front page layout", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusOK, in)
}
