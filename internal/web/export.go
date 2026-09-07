package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/item"
)

// Export is the whole of a board, in the shape it goes out.
//
// What is deliberately absent: reporters' email addresses and their follow
// tokens. The addresses are personal data collected for one purpose, and the
// tokens are credentials — an export that carried either would turn "take your
// data with you" into "take everybody's".
type Export struct {
	Format       string       `json:"format"`
	Organization string       `json:"organization"`
	Repository   string       `json:"repository"`
	ExportedAt   int64        `json:"exported_at"`
	Items        []exportItem `json:"items"`
}

type exportItem struct {
	ID         string          `json:"id"`
	Kind       string          `json:"kind"`
	Title      string          `json:"title"`
	Body       string          `json:"body"`
	Status     string          `json:"status"`
	Moderation string          `json:"moderation"`
	Votes      int             `json:"votes"`
	CreatedAt  int64           `json:"created_at"`
	Fields     []exportField   `json:"fields,omitempty"`
	Comments   []exportComment `json:"comments,omitempty"`
	Issue      *exportIssue    `json:"issue,omitempty"`
	MergedInto string          `json:"merged_into,omitempty"`
}

type exportField struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Value string `json:"value"`
}

type exportComment struct {
	Author    string `json:"author"`
	Name      string `json:"name,omitempty"`
	Body      string `json:"body"`
	CreatedAt int64  `json:"created_at"`
}

type exportIssue struct {
	Number int64  `json:"number"`
	URL    string `json:"url"`
}

// handleExport writes a board out as JSON.
//
// This exists because of a promise: what goes in comes out. A product that can
// only be left by hand is one people stay in for the wrong reason, and a
// self-hosted tool that holds data hostage is a contradiction.
//
// Everything is included, not only the published items: the queue, the
// rejections and the merged duplicates are the operator's own record of what
// they decided, and an export that quietly dropped them would be a partial
// answer to a question nobody asked partially.
func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	repo, organisation, ok := s.lookupRepository(w, r)
	if !ok {
		return
	}
	if !s.may(w, r, authz.Triage, authz.Repository(repo.ID, organisation.ID)) {
		return
	}

	out := Export{
		// Named and versioned, because an importer one day has to know what it
		// is reading, and "the shape it happened to have in 2026" is not
		// something anybody can write against.
		Format:       "lasthelp.export.v1",
		Organization: organisation.Slug,
		Repository:   repo.Slug,
		ExportedAt:   s.now().Unix(),
		Items:        []exportItem{},
	}

	store := s.items()
	for _, state := range []item.Moderation{item.Published, item.Pending, item.Rejected, item.Spam} {
		items, err := store.List(item.Filter{RepoID: repo.ID, Moderation: state, Limit: 200})
		if err != nil {
			slog.Error("exporting", "error", err)
			writeError(w, http.StatusInternalServerError, "error.internal")
			return
		}
		for _, it := range items {
			entry, err := s.exportOne(it)
			if err != nil {
				slog.Error("exporting an item", "error", err, "item", it.PublicID)
				writeError(w, http.StatusInternalServerError, "error.internal")
				return
			}
			out.Items = append(out.Items, entry)
		}
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	// Named after the board, so a folder of exports is readable a year later.
	w.Header().Set("Content-Disposition",
		`attachment; filename="`+safeFilename(organisation.Slug+"-"+repo.Slug)+`.json"`)

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(out); err != nil {
		slog.Error("writing the export", "error", err)
	}
}

func (s *Server) exportOne(it item.Item) (exportItem, error) {
	full, err := s.items().ByID(it.ID)
	if err != nil {
		return exportItem{}, err
	}

	entry := exportItem{
		ID:         full.PublicID,
		Kind:       string(full.Kind),
		Title:      full.Title,
		Body:       full.Body,
		Status:     string(full.Status),
		Moderation: string(full.Moderation),
		Votes:      full.VoteCount,
		CreatedAt:  full.CreatedAt.Unix(),
	}

	for _, f := range full.Fields {
		entry.Fields = append(entry.Fields, exportField{Key: f.Key, Label: f.Label, Value: f.Value})
	}

	// Every comment, including the ones still waiting: the queue is part of
	// what the operator decided, and an export that showed only the approved
	// half would misrepresent it.
	for _, state := range []item.Moderation{item.Published, item.Pending, item.Rejected} {
		comments, err := s.items().Comments(full.ID, state)
		if err != nil {
			return exportItem{}, err
		}
		for _, c := range comments {
			entry.Comments = append(entry.Comments, exportComment{
				Author: string(c.Author), Name: c.AuthorRef,
				Body: c.Body, CreatedAt: c.CreatedAt.Unix(),
			})
		}
	}

	if linked, err := s.bridge().Link(full.ID); err == nil {
		entry.Issue = &exportIssue{Number: linked.Number, URL: linked.URL}
	}
	if full.MergedInto != 0 {
		if survivor, err := s.items().ByID(full.MergedInto); err == nil {
			entry.MergedInto = survivor.PublicID
		}
	}
	return entry, nil
}

// safeFilename keeps a header from carrying anything but the slug it was built
// from. Slugs are already narrow, and this is what keeps that true if they ever
// stop being.
func safeFilename(name string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-':
			return r
		default:
			return '-'
		}
	}, strings.ToLower(name))
}
