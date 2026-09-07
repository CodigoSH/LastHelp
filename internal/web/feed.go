package web

import (
	"encoding/xml"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/item"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/org"
)

// The feed is the notification channel that needs no configuration, no address
// and no account — which is why it is the one the 1.0 leans on. Somebody who
// wants to follow a board subscribes; nothing about them is stored, and nothing
// has to be delivered anywhere.

type atomFeed struct {
	XMLName xml.Name    `xml:"http://www.w3.org/2005/Atom feed"`
	Title   string      `xml:"title"`
	ID      string      `xml:"id"`
	Updated string      `xml:"updated"`
	Links   []atomLink  `xml:"link"`
	Entries []atomEntry `xml:"entry"`
}

type atomLink struct {
	Href string `xml:"href,attr"`
	Rel  string `xml:"rel,attr,omitempty"`
	Type string `xml:"type,attr,omitempty"`
}

type atomEntry struct {
	Title      string         `xml:"title"`
	ID         string         `xml:"id"`
	Updated    string         `xml:"updated"`
	Link       atomLink       `xml:"link"`
	Summary    atomText       `xml:"summary"`
	Categories []atomCategory `xml:"category"`
}

type atomText struct {
	Type string `xml:"type,attr"`
	Text string `xml:",chardata"`
}

type atomCategory struct {
	Term string `xml:"term,attr"`
}

// handleFeed serves one board as Atom.
//
// Published items only, newest first — the same listing the public page shows,
// through the same code, so a feed cannot drift into publishing something the
// page would not.
func (s *Server) handleFeed(w http.ResponseWriter, r *http.Request) {
	repo, organisation, ok := s.lookupRepositoryForFeed(w, r)
	if !ok {
		return
	}

	items, err := s.items().List(item.Filter{RepoID: repo.ID, Limit: 50})
	if err != nil {
		slog.Error("building the feed", "error", err)
		http.Error(w, "error.internal", http.StatusInternalServerError)
		return
	}

	base := strings.TrimSuffix(s.cfg.PublicURL, "/")
	board := base + "/" + organisation.Slug + "/" + repo.Slug

	feed := atomFeed{
		Title: organisation.Name + " · " + repo.Name,
		// A tag: URI rather than the address, because the identity of a feed
		// has to survive the instance moving to a different domain — a reader
		// that sees a new id treats every entry as new.
		ID:      "tag:lasthelp," + organisation.Slug + ":" + repo.Slug,
		Updated: stamp(latest(items)),
		Links: []atomLink{
			{Href: board, Rel: "alternate", Type: "text/html"},
			{Href: base + "/feed/" + organisation.Slug + "/" + repo.Slug + "/atom.xml",
				Rel: "self", Type: "application/atom+xml"},
		},
	}

	for _, it := range items {
		feed.Entries = append(feed.Entries, atomEntry{
			Title:   it.Title,
			ID:      "tag:lasthelp," + organisation.Slug + ":" + repo.Slug + ":" + it.PublicID,
			Updated: stamp(it.CreatedAt),
			Link:    atomLink{Href: base + "/i/" + it.PublicID, Rel: "alternate", Type: "text/html"},
			// The body, as text. Never as HTML: it was written by a stranger,
			// and a feed reader renders what it is given.
			Summary:    atomText{Type: "text", Text: it.Body},
			Categories: []atomCategory{{Term: string(it.Kind)}, {Term: string(it.Status)}},
		})
	}

	w.Header().Set("Content-Type", "application/atom+xml; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=300")
	if _, err := w.Write([]byte(xml.Header)); err != nil {
		return
	}
	encoder := xml.NewEncoder(w)
	encoder.Indent("", "  ")
	if err := encoder.Encode(feed); err != nil {
		slog.Error("writing the feed", "error", err)
	}
}

func (s *Server) lookupRepositoryForFeed(w http.ResponseWriter, r *http.Request) (org.Repository, org.Organization, bool) {
	organisation, err := s.orgs().OrganizationBySlug(r.PathValue("org"))
	if err != nil {
		http.NotFound(w, r)
		return org.Repository{}, org.Organization{}, false
	}
	repo, err := s.orgs().RepositoryBySlugs(organisation.Slug, r.PathValue("repo"))
	if err != nil {
		http.NotFound(w, r)
		return org.Repository{}, org.Organization{}, false
	}
	return repo, organisation, true
}

func latest(items []item.Item) time.Time {
	newest := time.Time{}
	for _, it := range items {
		if it.CreatedAt.After(newest) {
			newest = it.CreatedAt
		}
	}
	if newest.IsZero() {
		// An empty board still needs a valid timestamp: a feed with a malformed
		// `updated` is one some readers refuse outright.
		return time.Unix(0, 0).UTC()
	}
	return newest
}

func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339) }
