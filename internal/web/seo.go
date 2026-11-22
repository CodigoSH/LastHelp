package web

import (
	"encoding/xml"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strings"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/org"
)

// What a search engine is told about this installation: where its public pages
// are, and that an address which does not exist is not a page.

// maxSitemapURLs is the protocol's own ceiling for one sitemap file.
const maxSitemapURLs = 50_000

// publicBase is the address this installation says it is reachable at, without a
// trailing slash, or "" when the operator did not say. A sitemap has to carry
// absolute addresses, and the only trustworthy one is the one the operator
// configured: the Host header is whatever the caller wrote.
func (s *Server) publicBase() string {
	return strings.TrimSuffix(strings.TrimSpace(s.cfg.PublicURL), "/")
}

type sitemapURL struct {
	Loc string `xml:"loc"`
}

type sitemapSet struct {
	XMLName xml.Name     `xml:"http://www.sitemaps.org/schemas/sitemap/0.9 urlset"`
	URLs    []sitemapURL `xml:"url"`
}

// handleSitemap lists the pages that are public: the front page, each public
// organization, and each public board with its roadmap. What is private is not in
// it, whoever asks — a signed-in administrator gets the same file a crawler does,
// because a sitemap is a statement about the public site and not about a viewer.
//
// With no public address configured there is no sitemap, and not one with guessed
// addresses in it.
func (s *Server) handleSitemap(w http.ResponseWriter, r *http.Request) {
	base := s.publicBase()
	if base == "" {
		http.NotFound(w, r)
		return
	}
	orgs, err := s.orgs().ListOrganizations()
	if err != nil {
		slog.Error("listing organizations for the sitemap", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	boards, err := s.orgs().ListBoards()
	if err != nil {
		slog.Error("listing boards for the sitemap", "error", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	set := sitemapSet{URLs: []sitemapURL{{Loc: base + "/"}}}
	add := func(p string) {
		if len(set.URLs) < maxSitemapURLs {
			set.URLs = append(set.URLs, sitemapURL{Loc: base + p})
		}
	}
	for _, o := range orgs {
		if !o.Private {
			add("/" + o.Slug)
		}
	}
	for _, b := range boards {
		if b.Hidden() {
			continue
		}
		add("/" + b.OrgSlug + "/" + b.Slug)
		add("/" + b.OrgSlug + "/" + b.Slug + "/roadmap")
	}

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	_, _ = io.WriteString(w, xml.Header)
	if err := xml.NewEncoder(w).Encode(set); err != nil {
		slog.Warn("writing the sitemap", "error", err)
	}
}

// handleRobots serves robots.txt, and says where the sitemap is when there is one.
func (s *Server) handleRobots(w http.ResponseWriter, r *http.Request) {
	f, err := s.assets.Open("robots.txt")
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = f.Close() }()
	raw, err := io.ReadAll(f)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	body := strings.TrimRight(string(raw), "\n") + "\n"
	if base := s.publicBase(); base != "" {
		body += "\nSitemap: " + base + "/sitemap.xml\n"
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.WriteString(w, body)
}

// shellStatus is the status the application's page is served with.
//
// The page is the same for every address that has no file, because the frontend
// routes on its own. That made every address that does not exist answer 200 with
// the front page's title: to a search engine, a page, and an endless supply of
// them. The server knows what the frontend will show, so it says so: an address
// that leads to a screen is 200, and one that leads to "not found" is 404, with
// the same page, which is what draws that message.
//
// The routes here are the frontend's, in the same order. What is behind a session
// answers 200 whether it exists or not: it is kept out of search on its own terms,
// and a status that said which panel addresses exist would only tell a stranger.
func (s *Server) shellStatus(r *http.Request) int {
	clean := path.Clean("/" + strings.Trim(r.URL.Path, "/"))
	if clean == "/" {
		return http.StatusOK
	}
	switch clean {
	case "/sign-in", "/setup", "/setup/account", "/setup/instance", "/setup/done", "/review":
		return http.StatusOK
	}
	parts := strings.Split(strings.TrimPrefix(clean, "/"), "/")
	if parts[0] == "panel" || parts[0] == "i" {
		return http.StatusOK
	}

	switch len(parts) {
	case 1:
		if _, ok := s.visibleOrganization(r, parts[0]); ok {
			return http.StatusOK
		}
	case 2:
		if s.visibleBoard(r, parts[0], parts[1]) {
			return http.StatusOK
		}
	case 3:
		switch parts[2] {
		case "new", "roadmap", "queue":
			if s.visibleBoard(r, parts[0], parts[1]) {
				return http.StatusOK
			}
		}
	}
	return http.StatusNotFound
}

func (s *Server) visibleOrganization(r *http.Request, slug string) (org.Organization, bool) {
	o, err := s.orgs().OrganizationBySlug(slug)
	if err != nil || !s.mayViewOrganization(r, o) {
		return org.Organization{}, false
	}
	return o, true
}

func (s *Server) visibleBoard(r *http.Request, orgSlug, repoSlug string) bool {
	o, ok := s.visibleOrganization(r, orgSlug)
	if !ok {
		return false
	}
	repo, err := s.orgs().RepositoryBySlugs(o.Slug, repoSlug)
	return err == nil && s.mayViewRepository(r, o, repo)
}
