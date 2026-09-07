package web

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/guard"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/item"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/org"
)

// reportBodyLimit is what a submission may be. Generous enough for a stack
// trace pasted into the description, small enough that a flood costs the
// sender something.
const reportBodyLimit = 64 << 10

// handleOrganizations lists what this instance serves. Public: it is the front
// page of the portal.
func (s *Server) handleOrganizations(w http.ResponseWriter, _ *http.Request) {
	list, err := s.orgs().ListOrganizations()
	if err != nil {
		slog.Error("listing organisations", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, o := range list {
		out = append(out, map[string]any{"slug": o.Slug, "name": o.Name})
	}
	writeJSON(w, http.StatusOK, map[string]any{"organizations": out})
}

// handleRepositories lists one organisation's repositories.
func (s *Server) handleRepositories(w http.ResponseWriter, r *http.Request) {
	o, ok := s.lookupOrganization(w, r)
	if !ok {
		return
	}
	list, err := s.orgs().ListRepositories(o.ID)
	if err != nil {
		slog.Error("listing repositories", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	out := make([]map[string]any, 0, len(list))
	for _, repo := range list {
		out = append(out, describeRepository(repo, s.mailConfigured()))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"organization": map[string]any{"slug": o.Slug, "name": o.Name},
		"repositories": out,
	})
}

// handleRepository describes one board: what it accepts, and therefore what the
// form should offer.
func (s *Server) handleRepository(w http.ResponseWriter, r *http.Request) {
	repo, organisation, ok := s.lookupRepository(w, r)
	if !ok {
		return
	}
	// The organisation travels with it, because every page that shows a board
	// also shows which organisation it belongs to — and asking for that
	// separately is a second request every page has to remember to make.
	body := describeRepository(repo, s.mailConfigured())
	body["organization"] = map[string]any{"slug": organisation.Slug, "name": organisation.Name}
	writeJSON(w, http.StatusOK, body)
}

// handleListItems is the public board.
func (s *Server) handleListItems(w http.ResponseWriter, r *http.Request) {
	repo, _, ok := s.lookupRepository(w, r)
	if !ok {
		return
	}

	q := r.URL.Query()
	filter := item.Filter{
		RepoID:     repo.ID,
		Kind:       item.Kind(q.Get("kind")),
		Status:     item.Status(q.Get("status")),
		Search:     q.Get("q"),
		SortByVote: q.Get("sort") == "votes",
		Limit:      intParam(q.Get("limit"), 50),
		Offset:     intParam(q.Get("offset"), 0),
	}
	// Moderation is deliberately not taken from the query: the public listing
	// is published items, and the queue is a different route behind a session.
	list, err := s.items().List(filter)
	if err != nil {
		slog.Error("listing items", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	voter := s.voterID(w, r)
	out := make([]map[string]any, 0, len(list))
	for _, it := range list {
		out = append(out, s.describeItem(it, voter))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// handleRoadmap is the board that nobody maintains by hand.
//
// It is derived from the state of the items, which in turn follows what happens
// in the forge once that is connected. A roadmap somebody updates manually is a
// roadmap that is wrong within a month; this one cannot be, because there is
// nothing to forget to update.
func (s *Server) handleRoadmap(w http.ResponseWriter, r *http.Request) {
	repo, _, ok := s.lookupRepository(w, r)
	if !ok {
		return
	}

	board, err := s.items().Roadmap(repo.ID, 25)
	if err != nil {
		slog.Error("building the roadmap", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	voter := s.voterID(w, r)
	columns := make([]map[string]any, 0, len(item.Columns))
	for _, column := range item.Columns {
		entries := make([]map[string]any, 0, len(board[column]))
		for _, it := range board[column] {
			entries = append(entries, s.describeItem(it, voter))
		}
		// Every column is sent even when empty: a page that draws two columns
		// leaves the reader wondering which two they are looking at.
		columns = append(columns, map[string]any{"column": string(column), "items": entries})
	}
	writeJSON(w, http.StatusOK, map[string]any{"columns": columns})
}

// handleSimilar is the "this looks like these" hint, asked while somebody is
// still typing the title.
func (s *Server) handleSimilar(w http.ResponseWriter, r *http.Request) {
	repo, _, ok := s.lookupRepository(w, r)
	if !ok {
		return
	}
	found, err := s.items().Similar(repo.ID, r.URL.Query().Get("title"), 3)
	if err != nil {
		slog.Error("looking for similar items", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	out := make([]map[string]any, 0, len(found))
	for _, it := range found {
		out = append(out, map[string]any{
			"public_id": it.PublicID, "title": it.Title,
			"kind": string(it.Kind), "votes": it.VoteCount,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"similar": out})
}

// handleForm hands out the token a submission has to carry.
//
// Fetching it is what separates a person who opened the page from a script that
// found the endpoint, and it is bound to this board, this moment and this part
// of the internet.
func (s *Server) handleForm(w http.ResponseWriter, r *http.Request) {
	repo, _, ok := s.lookupRepository(w, r)
	if !ok {
		return
	}

	accepting, err := s.guard().Accepting(repo.ID, s.now())
	if err != nil {
		slog.Error("checking the breaker", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"token":     s.guard().IssueToken(repo.ID, s.bucketOf(r), s.now()),
		"accepting": accepting,
	})
}

type submission struct {
	Kind   string            `json:"kind"`
	Title  string            `json:"title"`
	Body   string            `json:"body"`
	Email  string            `json:"email"`
	Fields map[string]string `json:"fields"`

	// Token is what handleForm issued.
	Token string `json:"token"`

	// Website is the honeypot: no person sees the field, so anything in it was
	// put there by something filling in every input it found.
	Website string `json:"website"`
}

// handleSubmit takes a report. No account, no email required.
//
// The follow link is returned once, here, and never again: only its hash is
// stored, so there is nowhere to look it up from later.
func (s *Server) handleSubmit(w http.ResponseWriter, r *http.Request) {
	repo, _, ok := s.lookupRepository(w, r)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, reportBodyLimit)
	var in submission
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}

	// The layers, cheapest first. Each refusal is counted so the operator can
	// see which one is doing the work — a layer that never refuses anything is
	// a layer to delete, and one that refuses nearly everything is biting
	// people who did nothing wrong.
	if err := guard.Honeypot(in.Website); err != nil {
		s.refused(repo.ID, "honeypot")
		// Answered as if it worked. Telling a script which check caught it is
		// telling it what to change.
		writeJSON(w, http.StatusCreated, map[string]any{
			"public_id": "", "follow_token": "", "moderation": string(item.Pending),
		})
		return
	}

	if err := s.guard().CheckToken(in.Token, repo.ID, s.bucketOf(r), s.now()); err != nil {
		s.refused(repo.ID, "form_token")
		writeError(w, http.StatusBadRequest, keyForGuardError(err))
		return
	}

	accepting, err := s.guard().Accepting(repo.ID, s.now())
	if err != nil {
		slog.Error("checking the breaker", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	if !accepting {
		s.refused(repo.ID, "breaker")
		writeError(w, http.StatusServiceUnavailable, "error.board_closed")
		return
	}

	verdict, err := s.guard().Inspect(repo.ID, in.Title, in.Body, s.stoplist(), s.now())
	if err != nil {
		slog.Error("inspecting a submission", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	if !verdict.OK {
		s.refused(repo.ID, verdict.Layer)
		writeError(w, http.StatusBadRequest, verdict.Reason)
		return
	}

	kind := item.Kind(in.Kind)
	switch {
	case kind == item.Bug && !repo.AcceptsBugs, kind == item.Idea && !repo.AcceptsIdeas:
		writeError(w, http.StatusBadRequest, "error.kind_not_accepted")
		return
	case kind != item.Bug && kind != item.Idea:
		writeError(w, http.StatusBadRequest, "error.unknown_kind")
		return
	}

	fields := make([]item.Field, 0, len(in.Fields))
	for key, value := range in.Fields {
		fields = append(fields, item.Field{Key: key, Label: key, Value: value})
	}

	// Everything waits for a human unless the repository says otherwise. The
	// queue is the protection: nothing the public writes reaches a public page
	// without somebody having looked at it.
	// The address is sealed here, at the edge, so the domain never holds it in
	// the clear. Left blank — which is the normal case and what the form
	// encourages — nothing is stored at all.
	var sealedEmail []byte
	if address := strings.TrimSpace(in.Email); address != "" {
		var sealErr error
		if sealedEmail, sealErr = s.box.Seal(address); sealErr != nil {
			slog.Error("sealing a reporter address", "error", sealErr)
			writeError(w, http.StatusInternalServerError, "error.internal")
			return
		}
	}

	moderation := item.Pending
	created, token, createErr := s.items().Create(item.New{
		RepoID: repo.ID, Kind: kind, Title: in.Title, Body: in.Body,
		EmailSealed: sealedEmail, Fields: fields,
	}, moderation, s.now())
	if createErr != nil {
		if errors.Is(createErr, item.ErrNotFound) {
			writeError(w, http.StatusNotFound, "error.not_found")
			return
		}
		writeError(w, http.StatusBadRequest, "error.invalid_report")
		return
	}

	// Tell the team, later. Not from here: a slow relay or a chat server having
	// a bad afternoon must not be something the person reporting a bug waits
	// for, or worse, is refused by.
	if err := s.bridge().RequestAnnouncement(created.ID, s.now()); err != nil {
		slog.Error("queueing a notification", "error", err)
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"public_id": created.PublicID,
		// Shown once. Only the hash is stored, so this is the only moment it
		// exists anywhere outside the reporter's browser.
		"follow_token": token,
		"moderation":   string(created.Moderation),
	})
}

// handleItem shows one item and its published thread.
//
// A follow token in the query is what lets a reporter see their own submission
// while it is still waiting, and nothing else: it does not reveal anybody
// else's, and it is compared against the stored hash in constant time.
func (s *Server) handleItem(w http.ResponseWriter, r *http.Request) {
	publicID := r.PathValue("id")
	store := s.items()

	it, err := store.ByPublicID(publicID)
	if err != nil {
		writeError(w, http.StatusNotFound, "error.not_found")
		return
	}

	mine := false
	if token := r.URL.Query().Get("k"); token != "" {
		if _, err := store.CheckFollowToken(publicID, token); err == nil {
			mine = true
		}
	}

	if it.Moderation != item.Published && !mine {
		// Not "it is waiting for review": an unauthenticated caller learns
		// nothing about what exists and is not public yet.
		writeError(w, http.StatusNotFound, "error.not_found")
		return
	}

	comments, err := store.Comments(it.ID, item.Published)
	if err != nil {
		slog.Error("reading the thread", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	thread := make([]map[string]any, 0, len(comments))
	for _, c := range comments {
		thread = append(thread, map[string]any{
			"author":     string(c.Author),
			"name":       c.AuthorRef,
			"body":       c.Body,
			"created_at": c.CreatedAt.Unix(),
		})
	}

	body := s.describeItem(it, s.voterID(w, r))
	body["body"] = it.Body
	body["comments"] = thread
	body["mine"] = mine
	if linked, err := s.bridge().Link(it.ID); err == nil {
		body["issue"] = map[string]any{"number": linked.Number, "url": linked.URL}
	}
	if it.MergedInto != 0 {
		if survivor, err := store.ByID(it.MergedInto); err == nil {
			body["merged_into"] = survivor.PublicID
		}
	}
	writeJSON(w, http.StatusOK, body)
}

// handleVote records a vote for an idea.
func (s *Server) handleVote(w http.ResponseWriter, r *http.Request) {
	it, ok := s.lookupItem(w, r)
	if !ok {
		return
	}
	voter := s.voterID(w, r)
	if voter == "" {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	err := s.items().Vote(it.ID, voter, s.now())
	switch {
	case errors.Is(err, item.ErrAlreadyVoted):
		// Not a failure: the vote is there, which is what they wanted.
	case errors.Is(err, item.ErrNotVotable):
		writeError(w, http.StatusBadRequest, "error.not_votable")
		return
	case err != nil:
		slog.Error("voting", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	s.writeVoteState(w, it.ID, voter)
}

// handleUnvote takes a vote back.
func (s *Server) handleUnvote(w http.ResponseWriter, r *http.Request) {
	it, ok := s.lookupItem(w, r)
	if !ok {
		return
	}
	voter := s.voterID(w, r)
	if err := s.items().Unvote(it.ID, voter); err != nil {
		slog.Error("removing a vote", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	s.writeVoteState(w, it.ID, voter)
}

func (s *Server) writeVoteState(w http.ResponseWriter, itemID int64, voter string) {
	fresh, err := s.items().ByID(itemID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	voted, err := s.items().HasVoted(itemID, voter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"votes": fresh.VoteCount, "voted": voted})
}

type newComment struct {
	Body  string `json:"body"`
	Token string `json:"token"`
}

// handleComment adds a message to a thread.
//
// A reporter with their follow token is recognised as the reporter; anybody
// else is a visitor. Neither is published without review — the token proves who
// somebody is, not that what they wrote belongs on a public page.
func (s *Server) handleComment(w http.ResponseWriter, r *http.Request) {
	it, ok := s.lookupItem(w, r)
	if !ok {
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, reportBodyLimit)
	var in newComment
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}

	author := item.AuthorVisitor
	if in.Token != "" {
		if _, err := s.items().CheckFollowToken(it.PublicID, in.Token); err == nil {
			author = item.AuthorReporter
		}
	}

	if _, err := s.items().Comment(it.ID, author, "", in.Body, s.now()); err != nil {
		writeError(w, http.StatusBadRequest, "error.invalid_comment")
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"moderation": string(item.Pending)})
}

// --- shared lookups --------------------------------------------------------

func (s *Server) lookupOrganization(w http.ResponseWriter, r *http.Request) (org.Organization, bool) {
	o, err := s.orgs().OrganizationBySlug(r.PathValue("org"))
	if err != nil {
		writeError(w, http.StatusNotFound, "error.not_found")
		return org.Organization{}, false
	}
	return o, true
}

func (s *Server) lookupRepository(w http.ResponseWriter, r *http.Request) (org.Repository, org.Organization, bool) {
	o, ok := s.lookupOrganization(w, r)
	if !ok {
		return org.Repository{}, org.Organization{}, false
	}
	repo, err := s.orgs().RepositoryBySlugs(o.Slug, r.PathValue("repo"))
	if err != nil {
		writeError(w, http.StatusNotFound, "error.not_found")
		return org.Repository{}, org.Organization{}, false
	}
	return repo, o, true
}

func (s *Server) lookupItem(w http.ResponseWriter, r *http.Request) (item.Item, bool) {
	it, err := s.items().ByPublicID(r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusNotFound, "error.not_found")
		return item.Item{}, false
	}
	return it, true
}

func describeRepository(r org.Repository, mailConfigured bool) map[string]any {
	return map[string]any{
		"slug":          r.Slug,
		"name":          r.Name,
		"accepts_bugs":  r.AcceptsBugs,
		"accepts_ideas": r.AcceptsIdeas,
		// With no relay there is nothing to notify anybody with, so the form
		// does not ask for an address rather than collecting one it cannot use.
		"can_notify": mailConfigured,
	}
}

func (s *Server) describeItem(it item.Item, voter string) map[string]any {
	voted := false
	if voter != "" && it.Kind == item.Idea {
		voted, _ = s.items().HasVoted(it.ID, voter)
	}
	return map[string]any{
		"public_id":  it.PublicID,
		"kind":       string(it.Kind),
		"title":      it.Title,
		"status":     string(it.Status),
		"votes":      it.VoteCount,
		"voted":      voted,
		"created_at": it.CreatedAt.Unix(),
	}
}

func intParam(raw string, fallback int) int {
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return fallback
	}
	return n
}

// keyForGuardError turns a refusal into something the page can say in the
// reader's language. The token failures deliberately collapse into one message:
// the difference between "too fast" and "already used" is only useful to
// somebody tuning a script.
func keyForGuardError(err error) string {
	switch {
	case errors.Is(err, guard.ErrTooFast), errors.Is(err, guard.ErrStale),
		errors.Is(err, guard.ErrUsed), errors.Is(err, guard.ErrNoToken):
		return "error.form_stale"
	default:
		return "error.bad_request"
	}
}
