package web

import (
	"encoding/json"
	"errors"
	"net/http"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/form"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/org"
)

// describeQuestions is a project's own questions, ready for the form that asks
// them and for the settings page that edits them. The same shape for both: what
// a reporter is shown is what the team set, and nothing the team set is secret.
func describeQuestions(fields []form.Field) []map[string]any {
	out := make([]map[string]any, 0, len(fields))
	for _, f := range fields {
		options := f.Options
		if options == nil {
			options = []string{}
		}
		out = append(out, map[string]any{
			"key":      f.Key,
			"label":    f.Label,
			"type":     string(f.Type),
			"required": f.Required,
			"options":  options,
			"for":      string(f.For),
		})
	}
	return out
}

// question is what the settings page sends for one: everything a team chooses.
// Type is read only when the question is created; an edit keeps the kind of
// answer the question already takes.
type question struct {
	Label    string   `json:"label"`
	Type     string   `json:"type"`
	Required bool     `json:"required"`
	Options  []string `json:"options"`
	For      string   `json:"for"`
}

func (q question) field() form.Field {
	return form.Field{Label: q.Label, Type: form.Type(q.Type), Required: q.Required, Options: q.Options, For: form.For(q.For)}
}

// questionTarget resolves the project of the address and checks that the
// reader may change its form: whoever runs its organisation.
func (s *Server) questionTarget(w http.ResponseWriter, r *http.Request) (org.Repository, bool) {
	repo, _, ok := s.lookupRepository(w, r)
	if !ok {
		return org.Repository{}, false
	}
	if !s.may(w, r, authz.ManageOrganization, authz.Organization(repo.OrgID)) {
		return org.Repository{}, false
	}
	return repo, true
}

func writeQuestionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, form.ErrInvalid):
		writeError(w, http.StatusBadRequest, "error.question_invalid")
	case errors.Is(err, form.ErrTooMany):
		writeError(w, http.StatusConflict, "error.question_limit")
	case errors.Is(err, form.ErrNotFound):
		writeError(w, http.StatusNotFound, "error.not_found")
	default:
		writeError(w, http.StatusInternalServerError, "error.internal")
	}
}

func readQuestion(w http.ResponseWriter, r *http.Request) (question, bool) {
	r.Body = http.MaxBytesReader(w, r.Body, panelBodyLimit)
	var in question
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return question{}, false
	}
	return in, true
}

// handleCreateQuestion adds a question at the end of a project's form.
func (s *Server) handleCreateQuestion(w http.ResponseWriter, r *http.Request) {
	repo, ok := s.questionTarget(w, r)
	if !ok {
		return
	}
	in, ok := readQuestion(w, r)
	if !ok {
		return
	}
	f := in.field()
	f.RepoID = repo.ID
	created, err := s.forms().Create(f)
	if err != nil {
		writeQuestionError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, describeQuestions([]form.Field{created})[0])
}

// handleUpdateQuestion rewords a question, or changes what it offers.
func (s *Server) handleUpdateQuestion(w http.ResponseWriter, r *http.Request) {
	repo, ok := s.questionTarget(w, r)
	if !ok {
		return
	}
	in, ok := readQuestion(w, r)
	if !ok {
		return
	}
	changed, err := s.forms().Update(repo.ID, r.PathValue("key"), in.field())
	if err != nil {
		writeQuestionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, describeQuestions([]form.Field{changed})[0])
}

// handleDeleteQuestion takes a question off the form. Answers already given
// stay on their reports.
func (s *Server) handleDeleteQuestion(w http.ResponseWriter, r *http.Request) {
	repo, ok := s.questionTarget(w, r)
	if !ok {
		return
	}
	if err := s.forms().Delete(repo.ID, r.PathValue("key")); err != nil {
		writeQuestionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleMoveQuestion moves a question one place up or down the form.
func (s *Server) handleMoveQuestion(w http.ResponseWriter, r *http.Request) {
	repo, ok := s.questionTarget(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, panelBodyLimit)
	var in struct {
		Direction string `json:"direction"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil || (in.Direction != "up" && in.Direction != "down") {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}
	if err := s.forms().Move(repo.ID, r.PathValue("key"), in.Direction == "up"); err != nil {
		writeQuestionError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
