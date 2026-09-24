package web

import (
	"html"
	"log/slog"
	"strings"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/landing"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/settings"
)

// shellTitle is the title index.html is built with, replaced on every request.
const shellTitle = "<title>LastHelp</title>"

// head is what the document says about itself before any script runs: its
// title, and the description and picture a link to it shows when it is shared.
//
// Written by the server because a link preview is made by a program that never
// runs the page's script — a chat client, a search engine — so a title set from
// JavaScript is a title those never see. The operator writes these in the page
// settings; an installation that has not falls back to its own name.
//
// Every value is escaped: the operator's words end up inside attributes, and a
// quote in a description is otherwise the end of the attribute.
func (s *Server) head() string {
	store := s.settings()
	name, err := store.Get(settings.InstanceName)
	if err != nil {
		slog.Error("reading the instance name for the page head", "error", err)
	}
	stored, err := store.Get(settings.LandingLayout)
	if err != nil {
		slog.Error("reading the front page for the page head", "error", err)
	}
	page, err := landing.Parse(stored)
	if err != nil {
		page = landing.Default()
	}
	return headTags(page.Settings, name, s.cfg.PublicURL)
}

func headTags(set landing.Settings, name, publicURL string) string {
	site := strings.TrimSpace(name)
	if site == "" {
		site = "LastHelp"
	}
	title := strings.TrimSpace(set.Title)
	if title == "" {
		title = site
	}
	description := strings.TrimSpace(set.Description)

	esc := html.EscapeString
	var b strings.Builder
	b.WriteString("<title>" + esc(title) + "</title>")
	b.WriteString(`<meta property="og:site_name" content="` + esc(site) + `" />`)
	b.WriteString(`<meta property="og:title" content="` + esc(title) + `" />`)
	b.WriteString(`<meta property="og:type" content="website" />`)
	if description != "" {
		b.WriteString(`<meta name="description" content="` + esc(description) + `" />`)
		b.WriteString(`<meta property="og:description" content="` + esc(description) + `" />`)
	}
	// A preview needs an absolute address, and the only trustworthy one is the
	// one the operator configured: the Host header is whatever the caller
	// wrote. Without it the picture is left out rather than guessed.
	base := strings.TrimSuffix(strings.TrimSpace(publicURL), "/")
	if set.ShareImage != "" && base != "" {
		b.WriteString(`<meta property="og:image" content="` + esc(base+set.ShareImage) + `" />`)
		b.WriteString(`<meta name="twitter:card" content="summary_large_image" />`)
	}
	return b.String()
}
