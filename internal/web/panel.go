package web

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/item"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/org"
	"git.codigosh.com/CodigoSH/LastHelp/internal/web/middleware"
)

const panelBodyLimit = 16 << 10

// may asks authz and writes the refusal. Every private handler starts with one
// of these, and nothing anywhere else compares against a role.
func (s *Server) may(w http.ResponseWriter, r *http.Request, action authz.Action, scope authz.Scope) bool {
	actor, ok := middleware.ActorFrom(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "error.unauthorized")
		return false
	}
	if !actor.Can(action, scope) {
		// 404 rather than 403 for things scoped to somebody else's
		// organisation would be better still, but a flat refusal here is
		// honest: the address was already public.
		writeError(w, http.StatusForbidden, "error.forbidden")
		return false
	}
	return true
}

type newOrganization struct {
	Slug       string `json:"slug"`
	Name       string `json:"name"`
	Forge      string `json:"forge"`
	BaseURL    string `json:"base_url"`
	ForgeOwner string `json:"forge_owner"`
	// Icon is a data URL, or empty for the initials of the name.
	Icon string `json:"icon"`
}

// handleCreateOrganization adds a connection. Server-level: an organisation
// belongs to no organisation, so only the administrator creates one.
func (s *Server) handleCreateOrganization(w http.ResponseWriter, r *http.Request) {
	if !s.may(w, r, authz.ManageServer, authz.Scope{}) {
		return
	}

	// Room for the icon, the one thing in here that can be large.
	r.Body = http.MaxBytesReader(w, r.Body, panelBodyLimit+maxIconBytes)
	var in newOrganization
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		if tooLarge(err) {
			writeError(w, http.StatusBadRequest, "error.image_too_big")
			return
		}
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}
	icon, ok := iconFrom(in.Icon)
	if !ok {
		writeError(w, http.StatusBadRequest, "error.image_too_big")
		return
	}

	created, err := s.orgs().CreateOrganization(org.Organization{
		Slug: in.Slug, Name: in.Name, Forge: org.Forge(in.Forge),
		BaseURL: in.BaseURL, ForgeOwner: in.ForgeOwner,
		Icon: icon,
	}, s.now())
	if err != nil {
		writeError(w, statusForOrgError(err), keyForOrgError(err))
		return
	}

	// Whoever created it owns it. Otherwise the administrator has to grant
	// themselves a membership before they can use what they just made.
	if actor, ok := middleware.ActorFrom(r); ok {
		if err := s.accounts().Grant(actor.UserID, authz.Organization(created.ID), authz.RoleOwner, s.now()); err != nil {
			slog.Error("granting ownership of a new organisation", "error", err)
		}
	}

	writeJSON(w, http.StatusCreated, map[string]any{"slug": created.Slug, "name": created.Name})
}

type newRepository struct {
	Slug         string `json:"slug"`
	Name         string `json:"name"`
	ForgeRepo    string `json:"forge_repo"`
	AcceptsBugs  *bool  `json:"accepts_bugs"`
	AcceptsIdeas *bool  `json:"accepts_ideas"`
	// AcceptsImages is on unless it is sent off.
	AcceptsImages *bool `json:"accepts_images"`
	// Icon is a data URL, or empty for the initials of the name.
	Icon string `json:"icon"`
}

// maxIconBytes keeps a repository's icon small enough to travel inline in
// every list that shows it: a page of twenty repositories carries twenty.
const maxIconBytes = 24 << 10

// iconFrom reads an icon — a repository's or an organisation's — as it
// arrived: empty, or an image as a data URL
// under the bound. Anything else is refused rather than stored and served.
func iconFrom(raw string) (string, bool) {
	icon := strings.TrimSpace(raw)
	if icon == "" {
		return "", true
	}
	return icon, strings.HasPrefix(icon, "data:image/") && len(icon) <= maxIconBytes
}

// handleCreateRepository registers a repository for feedback.
func (s *Server) handleCreateRepository(w http.ResponseWriter, r *http.Request) {
	o, ok := s.lookupOrganization(w, r)
	if !ok {
		return
	}
	if !s.may(w, r, authz.ManageOrganization, authz.Organization(o.ID)) {
		return
	}

	// Room for the icon, the one thing in here that can be large.
	r.Body = http.MaxBytesReader(w, r.Body, panelBodyLimit+maxIconBytes)
	var in newRepository
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		if tooLarge(err) {
			writeError(w, http.StatusBadRequest, "error.image_too_big")
			return
		}
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}
	icon, ok := iconFrom(in.Icon)
	if !ok {
		writeError(w, http.StatusBadRequest, "error.image_too_big")
		return
	}

	// Both default to true: a board that accepts nothing is a public page with
	// no way to use it.
	accepts := func(p *bool) bool { return p == nil || *p }

	created, err := s.orgs().CreateRepository(org.Repository{
		OrgID: o.ID, Slug: in.Slug, Name: in.Name, ForgeRepo: in.ForgeRepo,
		AcceptsBugs: accepts(in.AcceptsBugs), AcceptsIdeas: accepts(in.AcceptsIdeas),
		Icon: icon,
	}, s.now())
	if err != nil {
		writeError(w, statusForOrgError(err), keyForOrgError(err))
		return
	}
	if !accepts(in.AcceptsImages) {
		if err := s.orgs().SetAcceptsImages(created.ID, false); err != nil {
			writeError(w, http.StatusInternalServerError, "error.internal")
			return
		}
		created.AcceptsImages = false
	}
	writeJSON(w, http.StatusCreated, describeRepository(created, s.mailConfigured()))
}

// handleQueue is the moderation queue: the panel's front page.
func (s *Server) handleQueue(w http.ResponseWriter, r *http.Request) {
	repo, o, ok := s.lookupRepository(w, r)
	if !ok {
		return
	}
	if !s.may(w, r, authz.Triage, authz.Repository(repo.ID, o.ID)) {
		return
	}

	state := item.Moderation(r.URL.Query().Get("state"))
	if state == "" {
		state = item.Pending
	}
	list, err := s.items().List(item.Filter{
		RepoID: repo.ID, Moderation: state,
		Limit: intParam(r.URL.Query().Get("limit"), 50),
	})
	if err != nil {
		slog.Error("reading the queue", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	// In one query rather than one per row: a listing that loads its answers
	// item by item is fifty queries to draw one screen.
	if list, err = s.items().WithFields(list); err != nil {
		slog.Error("reading the queue", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	out := make([]map[string]any, 0, len(list))
	for _, it := range list {
		entry := s.describeItem(it, "")
		entry["body"] = it.Body
		// Whoever triages decides from what is in front of them. A bug whose
		// version and steps are only visible one click away is a bug triaged
		// without them.
		entry["fields"] = describeFields(it.Fields)
		out = append(out, entry)
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "state": string(state)})
}

// triageTarget resolves an item all the way up to its organisation, which is
// what authz needs to answer anything about it.
func (s *Server) triageTarget(w http.ResponseWriter, r *http.Request) (item.Item, authz.Scope, bool) {
	it, ok := s.lookupItem(w, r)
	if !ok {
		return item.Item{}, authz.Scope{}, false
	}
	repo, err := s.orgs().RepositoryByID(it.RepoID)
	if err != nil {
		writeError(w, http.StatusNotFound, "error.not_found")
		return item.Item{}, authz.Scope{}, false
	}
	scope := authz.Repository(repo.ID, repo.OrgID)
	if !s.may(w, r, authz.Triage, scope) {
		return item.Item{}, authz.Scope{}, false
	}
	return it, scope, true
}

type moderationChange struct {
	State string `json:"state"`
}

type textChange struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}

// handleEditItem rewrites the title and the description of a report.
//
// Somebody triaging fixes a title that says "it's broken", or trims a
// description with half a stack trace pasted twice. Behind the same door as
// every other decision about a report — Triage on the board it belongs to —
// and it changes nothing else: not the kind, not the status, not whether it is
// published.
//
// The domain applies the same limits the public form does. A report the team
// edits into a shape the form would have refused is a report nothing else here
// expects to see, and the check being in one place is what stops the two
// drifting.
func (s *Server) handleEditItem(w http.ResponseWriter, r *http.Request) {
	it, _, ok := s.triageTarget(w, r)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, reportBodyLimit)
	var in textChange
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}

	if err := s.items().Edit(it.ID, in.Title, in.Body, s.now()); err != nil {
		if errors.Is(err, item.ErrNotFound) {
			writeError(w, http.StatusNotFound, "error.not_found")
			return
		}
		// A refusal from the domain is about what was typed, not about the
		// server: too long, or nothing at all.
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"title": strings.TrimSpace(in.Title)})
}

// handleModerate approves, rejects or marks as spam.
func (s *Server) handleModerate(w http.ResponseWriter, r *http.Request) {
	it, _, ok := s.triageTarget(w, r)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, panelBodyLimit)
	var in moderationChange
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}

	var err error
	switch item.Moderation(in.State) {
	case item.Published:
		err = s.items().Approve(it.ID, s.now())
	case item.Rejected:
		err = s.items().Reject(it.ID, s.now())
	case item.Spam:
		err = s.items().MarkSpam(it.ID, s.now())
	default:
		writeError(w, http.StatusBadRequest, "error.unknown_state")
		return
	}
	if err != nil {
		slog.Error("moderating", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	// Only when it is published. Somebody whose report was rejected is not
	// told, on purpose: a rejection notice is an invitation to argue with a
	// filter, and the person who most wants to argue is the one it caught
	// correctly.
	if item.Moderation(in.State) == item.Published {
		if err := s.bridge().RequestTellReporter(it.ID, "is now public", s.now()); err != nil {
			slog.Error("queueing a reporter notification", "error", err)
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{"moderation": in.State})
}

type statusChange struct {
	Status string `json:"status"`
}

// handleSetStatus moves an item along its own vocabulary.
func (s *Server) handleSetStatus(w http.ResponseWriter, r *http.Request) {
	it, _, ok := s.triageTarget(w, r)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, panelBodyLimit)
	var in statusChange
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}

	if err := s.items().SetStatus(it.ID, item.Status(in.Status), s.now()); err != nil {
		// A status outside this kind's vocabulary is the caller's mistake, not
		// the server's.
		writeError(w, http.StatusBadRequest, "error.unknown_status")
		return
	}

	// The reporter hears about it, and if this closed the item their address is
	// forgotten on the way — collected for one purpose, and that purpose is now
	// over.
	if err := s.bridge().RequestTellReporter(it.ID, "changed status", s.now()); err != nil {
		slog.Error("queueing a reporter notification", "error", err)
	}

	writeJSON(w, http.StatusOK, map[string]any{"status": in.Status})
}

// handleCommentQueue lists the comments on one repository that are still
// waiting, alongside the reports they were written on.
//
// The public page tells whoever writes one that a person will read it before it
// appears. Until this existed that was not true of anything: the comment went
// in as Pending, nothing listed it and nothing could publish it, so every
// conversation on this product died at the first reply from outside.
func (s *Server) handleCommentQueue(w http.ResponseWriter, r *http.Request) {
	repo, o, ok := s.lookupRepository(w, r)
	if !ok {
		return
	}
	if !s.may(w, r, authz.Triage, authz.Repository(repo.ID, o.ID)) {
		return
	}

	waiting, err := s.items().WaitingComments(repo.ID, intParam(r.URL.Query().Get("limit"), 50))
	if err != nil {
		slog.Error("reading the comment queue", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	out := make([]map[string]any, 0, len(waiting))
	for _, c := range waiting {
		out = append(out, map[string]any{
			"id":         c.ID,
			"author":     string(c.Author),
			"body":       c.Body,
			"created_at": c.CreatedAt.Unix(),
			"item":       map[string]any{"public_id": c.ItemPublicID, "title": c.ItemTitle},
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"comments": out})
}

// handleAllWaitingComments is the comment queue without naming a board.
//
// The per-board queue answers "what is waiting here"; a person with five boards
// had to ask it five times to find the one with something in it, and nothing
// told them which. This answers "what is waiting for me", which is the question
// they actually had.
//
// Scoped exactly like the count beside Reports, and for the same reason: asked
// board by board through Can, so an administrator gets everything because the
// rules say so and a collaborator gets theirs for the same reason. The board is
// carried on every row — without it a decision would be about a message from a
// place the reader cannot identify.
func (s *Server) handleAllWaitingComments(w http.ResponseWriter, r *http.Request) {
	actor, ok := middleware.ActorFrom(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "error.unauthorized")
		return
	}

	boards, err := s.items().PendingByBoard()
	if err != nil {
		slog.Error("reading the comment queue", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	ids := make([]int64, 0, len(boards))
	where := map[int64]map[string]any{}
	for _, b := range boards {
		if b.Messages == 0 || !actor.Can(authz.Triage, authz.Repository(b.RepoID, b.OrgID)) {
			continue
		}
		ids = append(ids, b.RepoID)
		entry := map[string]any{}
		if repo, err := s.orgs().RepositoryByID(b.RepoID); err == nil {
			entry["repository"] = repo.Slug
			entry["name"] = repo.Name
			if o, err := s.orgs().OrganizationByID(b.OrgID); err == nil {
				entry["organization"] = o.Slug
			}
		}
		where[b.RepoID] = entry
	}

	waiting, err := s.items().WaitingCommentsOn(ids, intParam(r.URL.Query().Get("limit"), 50))
	if err != nil {
		slog.Error("reading the comment queue", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	out := make([]map[string]any, 0, len(waiting))
	for _, c := range waiting {
		row := map[string]any{
			"id":         c.ID,
			"author":     string(c.Author),
			"body":       c.Body,
			"created_at": c.CreatedAt.Unix(),
			"item":       map[string]any{"public_id": c.ItemPublicID, "title": c.ItemTitle},
		}
		if board, ok := where[c.RepoID]; ok {
			row["board"] = board
		}
		out = append(out, row)
	}
	writeJSON(w, http.StatusOK, map[string]any{"comments": out})
}

// handleModerateComment publishes one, or hides it.
//
// Authorised through the report it hangs on: the rules are written about
// repositories, and a comment id on its own names nothing they can answer.
func (s *Server) handleModerateComment(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "error.not_found")
		return
	}

	it, err := s.items().CommentItem(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "error.not_found")
		return
	}
	repo, err := s.orgs().RepositoryByID(it.RepoID)
	if err != nil {
		writeError(w, http.StatusNotFound, "error.not_found")
		return
	}
	if !s.may(w, r, authz.Triage, authz.Repository(repo.ID, repo.OrgID)) {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, panelBodyLimit)
	var in moderationChange
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}

	switch item.Moderation(in.State) {
	case item.Published:
		err = s.items().ApproveComment(id)
	case item.Rejected:
		err = s.items().RejectComment(id)
	default:
		writeError(w, http.StatusBadRequest, "error.unknown_state")
		return
	}
	if err != nil {
		if errors.Is(err, item.ErrNotFound) {
			writeError(w, http.StatusNotFound, "error.not_found")
			return
		}
		slog.Error("moderating a comment", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"state": in.State})
}

type mergeRequest struct {
	Into string `json:"into"`
}

// handleMerge folds a duplicate into the item that survives.
func (s *Server) handleMerge(w http.ResponseWriter, r *http.Request) {
	duplicate, scope, ok := s.triageTarget(w, r)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, panelBodyLimit)
	var in mergeRequest
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}

	survivor, err := s.items().ByPublicID(in.Into)
	if err != nil {
		writeError(w, http.StatusNotFound, "error.not_found")
		return
	}
	// Both ends must be in the same repository, or merging becomes a way to
	// move somebody's votes into a board they have nothing to do with.
	if survivor.RepoID != duplicate.RepoID {
		writeError(w, http.StatusBadRequest, "error.different_repository")
		return
	}
	_ = scope

	if err := s.items().Merge(duplicate.ID, survivor.ID, s.now()); err != nil {
		writeError(w, http.StatusBadRequest, "error.cannot_merge")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"merged_into": survivor.PublicID})
}

type teamReply struct {
	Body string `json:"body"`
}

// handleReply posts the team's answer, which is published without waiting.
func (s *Server) handleReply(w http.ResponseWriter, r *http.Request) {
	it, _, ok := s.triageTarget(w, r)
	if !ok {
		return
	}
	actor, _ := middleware.ActorFrom(r)

	r.Body = http.MaxBytesReader(w, r.Body, panelBodyLimit)
	var in teamReply
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}

	name, err := s.username(actor.UserID)
	if err != nil {
		slog.Error("reading the username a reply is signed with", "error", err)
	}
	c, err := s.items().Comment(it.ID, item.AuthorTeam, name, in.Body, s.now())
	if err != nil {
		writeCommentError(w, err)
		return
	}
	// The id lets the pane hang the reply's pictures on it.
	writeJSON(w, http.StatusCreated, map[string]any{"id": c.ID})
}

// username is what a team reply is signed with: an account has one name, and
// it is this one.
func (s *Server) username(userID int64) (string, error) {
	var name string
	err := s.db.QueryRow(`SELECT username FROM app_user WHERE id = ?`, userID).Scan(&name)
	return name, err
}

func statusForOrgError(err error) int {
	switch {
	case errors.Is(err, org.ErrSlugTaken):
		return http.StatusConflict
	case errors.Is(err, org.ErrNotFound):
		return http.StatusNotFound
	default:
		return http.StatusBadRequest
	}
}

func keyForOrgError(err error) string {
	switch {
	case errors.Is(err, org.ErrSlugTaken):
		return "error.address_taken"
	case errors.Is(err, org.ErrInvalidSlug):
		return "error.invalid_address"
	case errors.Is(err, org.ErrUnknownForge):
		return "error.unknown_forge"
	case errors.Is(err, org.ErrNotFound):
		return "error.not_found"
	default:
		return "error.invalid_request"
	}
}

// teamWork is what the team may do to a report: the statuses it can be moved
// to and whether it can be deleted. It rides inside the report itself for
// whoever may triage it — it used to be an endpoint of its own, and the
// reports screen doubled every row it read asking it separately.
func teamWork(it item.Item, actor authz.Actor, scope authz.Scope) map[string]any {
	statuses := item.StatusesFor(it.Kind)
	out := make([]string, 0, len(statuses))
	for _, status := range statuses {
		out = append(out, string(status))
	}
	return map[string]any{
		"kind":     string(it.Kind),
		"current":  string(it.Status),
		"statuses": out,
		// Deleting is the organisation's owners' and the administrator's: a
		// collaborator can reject or mark as spam, which hides a report and
		// loses nothing. Said here so the page does not work it out itself.
		"can_delete": actor.Can(authz.ManageOrganization, authz.Organization(scope.OrgID)),
	}
}

// commentTarget reads the {id} of a comment and checks the one permission
// that changing it takes: Triage on the board it was written on, so a
// collaborator looks after the conversations of the boards they work on.
// Editing and deleting share it so the pencil and the bin cannot end up in
// different hands. The report itself, the boards and the organisations stay
// with whoever runs the organisation.
func (s *Server) commentTarget(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeError(w, http.StatusNotFound, "error.not_found")
		return 0, false
	}
	it, err := s.items().CommentItem(id)
	if err != nil {
		writeError(w, http.StatusNotFound, "error.not_found")
		return 0, false
	}
	repo, err := s.orgs().RepositoryByID(it.RepoID)
	if err != nil {
		writeError(w, http.StatusNotFound, "error.not_found")
		return 0, false
	}
	if !s.may(w, r, authz.Triage, authz.Repository(repo.ID, repo.OrgID)) {
		return 0, false
	}
	return id, true
}

// handleEditComment rewrites a comment or a team reply: a typo in an answer,
// or a visitor's message with an address in it that should not be public.
func (s *Server) handleEditComment(w http.ResponseWriter, r *http.Request) {
	id, ok := s.commentTarget(w, r)
	if !ok {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, reportBodyLimit)
	var in struct {
		Body string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}
	if err := s.items().EditComment(id, in.Body); err != nil {
		if errors.Is(err, item.ErrNotFound) {
			writeError(w, http.StatusNotFound, "error.not_found")
			return
		}
		// Empty or too long: about what was typed, not about the server.
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleDeleteComment removes a comment or a team reply for good.
func (s *Server) handleDeleteComment(w http.ResponseWriter, r *http.Request) {
	id, ok := s.commentTarget(w, r)
	if !ok {
		return
	}
	if err := s.items().DeleteComment(id); err != nil {
		slog.Error("deleting a comment", "error", err, "comment", id)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleDeleteItem removes a report for good.
//
// Behind ManageOrganization rather than Triage, on purpose: it cannot be
// undone, and everything a collaborator needs — rejecting, marking as spam,
// merging a duplicate — takes a report out of sight without losing it.
func (s *Server) handleDeleteItem(w http.ResponseWriter, r *http.Request) {
	it, ok := s.lookupItem(w, r)
	if !ok {
		return
	}
	repo, err := s.orgs().RepositoryByID(it.RepoID)
	if err != nil {
		writeError(w, http.StatusNotFound, "error.not_found")
		return
	}
	if !s.may(w, r, authz.ManageOrganization, authz.Organization(repo.OrgID)) {
		return
	}
	if err := s.items().Delete(it.ID); err != nil {
		slog.Error("deleting a report", "error", err, "item", it.PublicID)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	slog.Info("report deleted", "item", it.PublicID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
