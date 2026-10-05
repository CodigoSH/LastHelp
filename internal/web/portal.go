package web

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"strconv"
	"strings"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/guard"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/item"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/org"
	"git.codigosh.com/CodigoSH/LastHelp/internal/web/middleware"
)

// reportBodyLimit is what a submission may be. Generous enough for a stack
// trace pasted into the description, small enough that a flood costs the
// sender something.
//
// It is in bytes, because it bounds what is read off the wire, and the limits
// on the words are in characters: a report at every one of them in a language
// that takes four bytes a letter is (200 + 20,000 + two answers of 4,000) times
// four, about 113 KB. 64 KiB refused that as a malformed request.
const reportBodyLimit = 128 << 10

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
		out = append(out, map[string]any{"slug": o.Slug, "name": o.Name, "icon": o.Icon})
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
	organisation := map[string]any{"slug": o.Slug, "name": o.Name, "forge": string(o.Forge), "icon": o.Icon}

	// The webhook address, but only for somebody who may manage this
	// organisation. It is not a secret exactly — a delivery still has to be
	// signed — but it is the one thing that identifies the connection, and
	// handing it to the public would be handing out the surface to hammer.
	if actor, ok := middleware.ActorFrom(r); ok && actor.Can(authz.ManageOrganization, authz.Organization(o.ID)) {
		if hook, err := s.orgs().HookID(o.ID); err == nil && hook != "" {
			organisation["webhook_path"] = "/hooks/" + hook
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"organization": organisation,
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
	// Open or closed, the way every forge splits its issues: what still waits
	// for an answer, and what has one. Anything else is refused rather than
	// read as "both", which would look like a filter that works.
	state := q.Get("state")
	sides := map[string][]item.Status{"open": item.OpenStatuses(), "closed": item.ClosedStatuses()}
	if state != "" {
		statuses, ok := sides[state]
		if !ok {
			writeError(w, http.StatusBadRequest, "error.bad_request")
			return
		}
		filter.Statuses = statuses
	}
	// Moderation is deliberately not taken from the query: the public listing
	// is published items, and the queue is a different route behind a session.
	list, err := s.items().List(filter)
	if err != nil {
		slog.Error("listing items", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	// What the page says about the list: how many there are in all, so it can
	// say it is showing some of them rather than stopping at fifty in silence,
	// and how many are open and closed under the same kind and search, for the
	// two tabs that choose between them.
	all := filter
	all.Limit, all.Offset = 0, 0
	total, err := s.items().Tally(all)
	counts := map[string]int{}
	for side, statuses := range sides {
		if err != nil {
			break
		}
		one := all
		one.Status, one.Statuses = "", statuses
		counts[side], err = s.items().Tally(one)
	}
	if err != nil {
		slog.Error("counting items", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}

	voter := s.voterID(w, r)
	out := make([]map[string]any, 0, len(list))
	for _, it := range list {
		out = append(out, s.describeItem(it, voter))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "total": total, "counts": counts})
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

	// The kind first, because it decides which questions this report was asked,
	// and the answers are part of what the next layer inspects.
	kind := item.Kind(in.Kind)
	switch {
	case kind == item.Bug && !repo.AcceptsBugs, kind == item.Idea && !repo.AcceptsIdeas:
		writeError(w, http.StatusBadRequest, "error.kind_not_accepted")
		return
	case kind != item.Bug && kind != item.Idea:
		writeError(w, http.StatusBadRequest, "error.unknown_kind")
		return
	}

	// Only the questions this kind is actually asked, in the order it asks
	// them, with the labels the domain owns. Anything else in the request is
	// dropped: the label travels with the answer into the forge issue, and a
	// label the sender chose is a sentence they got the team to print.
	fields := make([]item.Field, 0, len(in.Fields))
	answers := make([]string, 0, len(in.Fields))
	for _, spec := range item.FieldsFor(kind) {
		value := strings.TrimSpace(in.Fields[spec.Key])
		if value == "" {
			continue
		}
		spec.Value = value
		fields = append(fields, spec)
		answers = append(answers, value)
	}

	verdict, err := s.guard().Inspect(repo.ID, in.Title, in.Body,
		strings.Join(answers, " "), s.stoplist(), s.now())
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

	// Everything waits for a human unless the repository says otherwise. The
	// queue is the protection: nothing the public writes reaches a public page
	// without somebody having looked at it.
	// Left blank — which is the normal case and what the form encourages —
	// nothing is stored at all. When it is given, it is stored as given: what
	// keeps it from travelling is that nothing reads the column except the code
	// about to send a message, and that it is deleted when the report closes.
	address := strings.TrimSpace(in.Email)
	if address != "" && !plausibleAddress(address) {
		writeError(w, http.StatusBadRequest, "error.invalid_email")
		return
	}

	moderation := item.Pending
	created, token, createErr := s.items().Create(item.New{
		RepoID: repo.ID, Kind: kind, Title: in.Title, Body: in.Body,
		Email: address, Fields: fields,
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

	// Who may see it before it is published: the person who wrote it, holding
	// their follow link, and whoever has to decide about it.
	//
	// The second half was missing, and it made the queue undecidable: a report
	// waiting for review is exactly the one somebody needs to read before
	// publishing it, and opening it from the panel answered "not here". Asked
	// of authz per board, so it is the same rule the triage endpoints use.
	if it.Moderation != item.Published && !mine && !s.mayTriageItem(r, it) {
		// Not "it is waiting for review": a caller who may not see it learns
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
			"id":         c.ID,
			"author":     string(c.Author),
			"name":       c.AuthorRef,
			"body":       c.Body,
			"created_at": c.CreatedAt.Unix(),
		})
	}

	body := s.describeItem(it, s.voterID(w, r))
	body["body"] = it.Body
	body["fields"] = describeFields(it.Fields)
	body["comments"] = thread
	body["images"] = s.visiblePictures(r, it)
	body["mine"] = mine
	// Whether the form is shown to this reader. An answered report keeps taking
	// words from the reporter and the team for a while, and `comment_until` says
	// when that ends so the page can tell them; a visitor's form goes at once.
	reader := item.AuthorVisitor
	switch {
	case s.mayTriageItem(r, it):
		reader = item.AuthorTeam
	case mine:
		reader = item.AuthorReporter
	}
	body["commentable"] = it.TakesCommentsFrom(reader, s.now())
	if it.Status.Closes() && it.TakesCommentsFrom(reader, s.now()) {
		body["comment_until"] = it.ClosedAt.Add(item.GraceAfterClosing).Unix()
	}
	// Where this came from. A report is the one page of this product that
	// travels: it is pasted into a chat, mailed to a colleague, linked from an
	// issue. Whoever arrives that way used to land on a title with no way to
	// tell which project it belongs to and no way onward — the page named
	// nothing and led nowhere.
	if repo, err := s.orgs().RepositoryByID(it.RepoID); err == nil {
		// accepts_images: whether the reporter's follow-up and the team's reply
		// offer pictures on this page.
		board := map[string]any{"slug": repo.Slug, "name": repo.Name, "accepts_images": repo.AcceptsImages}
		if o, err := s.orgs().OrganizationByID(repo.OrgID); err == nil {
			board["organization"] = map[string]any{"slug": o.Slug, "name": o.Name}
		}
		body["repository"] = board
		// For whoever works on it, what they may do to it, in the same answer:
		// the reports screen reads a report per row chosen, and asking this
		// separately doubled every row against the per-address ceiling.
		scope := authz.Repository(repo.ID, repo.OrgID)
		if actor, ok := middleware.ActorFrom(r); ok && actor.Can(authz.Triage, scope) {
			body["team"] = teamWork(it, actor, scope)
		}
	}
	// The reporter is the only one who can be here before publication, and the
	// only one who needs to be told so. Without this the page had no way to
	// tell the two apart, and told them their published report was still
	// waiting to be read — for as long as they kept the link.
	body["moderation"] = string(it.Moderation)
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
	case errors.Is(err, item.ErrVotingClosed):
		writeError(w, http.StatusConflict, "error.voting_closed")
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
		if errors.Is(err, item.ErrVotingClosed) {
			writeError(w, http.StatusConflict, "error.voting_closed")
			return
		}
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

	c, err := s.items().Comment(it.ID, author, "", in.Body, s.now())
	if err != nil {
		writeCommentError(w, err)
		return
	}
	// The id lets the reporter's page hang their pictures on this message.
	writeJSON(w, http.StatusAccepted, map[string]any{"id": c.ID, "moderation": string(item.Pending)})
}

// plausibleAddress is whether what was typed is one bare mailbox. The relay is
// the real judge, but a field that takes anything lets somebody write a third
// party's address with a name and a comma list around it; one address, no
// display name, no control characters, within the length a mailbox has.
func plausibleAddress(address string) bool {
	if len(address) > 254 {
		return false
	}
	parsed, err := mail.ParseAddress(address)
	return err == nil && parsed.Address == address && !strings.ContainsAny(address, "<>,;\r\n\t ")
}

// writeCommentError says why a comment or a reply was refused. Shared by the
// public form and the team's reply, so the two cannot answer differently.
func writeCommentError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, item.ErrConversationClosed):
		writeError(w, http.StatusConflict, "error.conversation_closed")
	case errors.Is(err, item.ErrNotFound):
		writeError(w, http.StatusNotFound, "error.not_found")
	default:
		writeError(w, http.StatusBadRequest, "error.invalid_comment")
	}
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
		// Whether the form offers pictures: the board's switch.
		"accepts_images": r.AcceptsImages,
		"icon":           r.Icon,
		// With no relay there is nothing to notify anybody with, so the form
		// does not ask for an address rather than collecting one it cannot use.
		"can_notify": mailConfigured,
	}
}

// describeFields is the answers to the form, ready to draw.
//
// The stored label goes out with them so a report sent under a question that
// has since been reworded still reads the way it was sent.
func describeFields(fields []item.Field) []map[string]any {
	out := make([]map[string]any, 0, len(fields))
	for _, f := range fields {
		out = append(out, map[string]any{"key": f.Key, "label": f.Label, "value": f.Value})
	}
	return out
}

func (s *Server) describeItem(it item.Item, voter string) map[string]any {
	voted := false
	if voter != "" && it.Kind == item.Idea {
		voted, _ = s.items().HasVoted(it.ID, voter)
	}
	return map[string]any{
		"public_id": it.PublicID,
		"kind":      string(it.Kind),
		"title":     it.Title,
		"status":    string(it.Status),
		"votes":     it.VoteCount,
		"voted":     voted,
		"votable":   it.Votable(),
		// False once the team has answered: the thread stays, the form goes.
		"commentable": it.TakesComments(),
		"created_at":  it.CreatedAt.Unix(),
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

// mayTriageItem answers whether whoever is asking may act on this report.
//
// It writes nothing: the caller decides what a "no" looks like, which on the
// public page is a plain "not here" rather than a refusal that admits the
// report exists.
func (s *Server) mayTriageItem(r *http.Request, it item.Item) bool {
	actor, ok := middleware.ActorFrom(r)
	if !ok {
		return false
	}
	repo, err := s.orgs().RepositoryByID(it.RepoID)
	if err != nil {
		return false
	}
	return actor.Can(authz.Triage, authz.Repository(repo.ID, repo.OrgID))
}
