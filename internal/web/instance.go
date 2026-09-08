package web

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
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
	store := settings.Store{DB: s.db}
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

	r.Body = http.MaxBytesReader(w, r.Body, panelBodyLimit)
	var in instanceSettings
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
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

	store := settings.Store{DB: s.db}
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
