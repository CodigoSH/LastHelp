package landing

import (
	"fmt"
	"net/url"
	"strings"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// The markup a module of words may carry, and nothing else.
//
// This is the first place in the product where something other than plain text
// reaches a public page. Everything else a stranger writes — a report, a title,
// a comment — is escaped on the way out and there is no policy to get wrong.
// Here the operator writes markup on purpose, so there has to be one, and it is
// stated in one place rather than spread across the code that happens to touch
// it.
//
// Exported because a whitelist nobody asserts loses entries in silence: the
// test compares these maps exactly rather than sampling them, so removing an
// element or quietly admitting one fails.
var (
	// AllowedTags is what survives. Deliberately short: headings, emphasis,
	// lists, links and quotes are what somebody writing a paragraph about their
	// project needs, and every addition past that is a decision to defend.
	//
	// Not here, and each for its own reason: script and style are the attack;
	// iframe, object and embed bring someone else's page onto ours; img is
	// absent because the policy this page is served under only allows images
	// from this origin, so a pasted one would show as a broken box; form and
	// input would put controls on a page whose controls all mean something.
	AllowedTags = map[string]bool{
		"p": true, "br": true,
		"h1": true, "h2": true, "h3": true,
		"strong": true, "em": true, "u": true, "s": true,
		"ul": true, "ol": true, "li": true,
		"a": true, "blockquote": true, "code": true, "span": true,
	}

	// AllowedClasses is how a colour and an alignment are carried.
	//
	// The editor cannot write them as inline styles: this page is served with
	// style-src 'self' and no unsafe-inline, so the browser drops a style
	// attribute without a word and the operator would watch a button do
	// nothing. A closed set of classes painted from the stylesheet says the
	// same thing, survives the policy, and cannot express anything the
	// stylesheet has not already agreed to.
	AllowedClasses = map[string]bool{
		"align-start": true, "align-center": true, "align-end": true, "align-justify": true,
		"ink-plain": true, "ink-dim": true, "ink-accent": true, "ink-danger": true,
		"ink-success": true, "ink-warning": true, "ink-info": true,
	}

	// AllowedSchemes is where a link may point. A javascript: href is the
	// oldest way there is to put script on a page that allows no script tags.
	AllowedSchemes = map[string]bool{"http": true, "https": true, "mailto": true}
)

// renamed is what a contenteditable produces versus what is stored.
//
// A browser's editing commands still emit the presentational tags of 1997 —
// b, i, strike — and wrap blocks in div. Storing whichever one the visitor's
// browser happened to pick would mean the same sentence saved two ways, so
// they are folded here, once, at the boundary.
var renamed = map[string]string{
	"b": "strong", "i": "em", "strike": "s", "del": "s", "ins": "u", "div": "p",
}

// Clean returns the markup with everything outside the policy removed.
//
// Unknown elements are unwrapped rather than deleted: the words inside them are
// the operator's and dropping a paragraph because it arrived in a tag we do not
// keep would lose work. The exception is the handful whose content is not text
// — script, style and the rest — where the content IS the thing being refused.
//
// It parses and re-serialises rather than filtering a token stream, so the
// result is well-formed whatever arrives. A filter that emits tags as it sees
// them can be handed an unbalanced document and pass its own tags through into
// a page that then closes them somewhere else entirely.
func Clean(in string) (string, error) {
	if strings.TrimSpace(in) == "" {
		return "", nil
	}

	context := &html.Node{Type: html.ElementNode, Data: "body", DataAtom: atom.Body}
	nodes, err := html.ParseFragment(strings.NewReader(in), context)
	if err != nil {
		return "", fmt.Errorf("reading the markup: %w", err)
	}

	var out strings.Builder
	for _, node := range nodes {
		writeClean(&out, node)
	}
	return strings.TrimSpace(out.String()), nil
}

// dropped is the elements whose text goes with them. Everywhere else the words
// survive their wrapper; here the words are the payload.
var dropped = map[string]bool{
	"script": true, "style": true, "iframe": true, "object": true, "embed": true,
	"noscript": true, "template": true, "svg": true, "math": true, "title": true,
}

func writeClean(out *strings.Builder, node *html.Node) {
	switch node.Type {
	case html.TextNode:
		out.WriteString(html.EscapeString(node.Data))
		return
	case html.ElementNode:
	default:
		// Comments, doctypes and anything else carry nothing a page needs.
		return
	}

	name := strings.ToLower(node.Data)
	if dropped[name] {
		return
	}
	if to, ok := renamed[name]; ok {
		name = to
	}

	if !AllowedTags[name] {
		// Unwrapped: the tag goes, the words stay.
		writeChildren(out, node)
		return
	}

	attributes := keepAttributes(name, node.Attr)
	out.WriteString("<" + name + attributes + ">")
	if name != "br" {
		writeChildren(out, node)
		out.WriteString("</" + name + ">")
	}
}

func writeChildren(out *strings.Builder, node *html.Node) {
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		writeClean(out, child)
	}
}

// keepAttributes returns the attributes that survive, already serialised.
//
// A whitelist of names is not enough on its own: href has to be read as a URL,
// because the dangerous part of a link is where it points, not that it is a
// link.
func keepAttributes(name string, attrs []html.Attribute) string {
	var out strings.Builder
	for _, attr := range attrs {
		// Namespaced attributes are how the same name arrives twice.
		if attr.Namespace != "" {
			continue
		}
		switch strings.ToLower(attr.Key) {
		case "class":
			if kept := keepClasses(attr.Val); kept != "" {
				out.WriteString(` class="` + html.EscapeString(kept) + `"`)
			}
		case "href":
			if name != "a" {
				continue
			}
			if target, ok := safeLink(attr.Val); ok {
				out.WriteString(` href="` + html.EscapeString(target) + `"`)
				// A link out of the page opens in its own tab and is told not
				// to hand ours to whatever it opens.
				out.WriteString(` rel="noopener noreferrer"`)
			}
		}
	}
	return out.String()
}

func keepClasses(value string) string {
	var kept []string
	for _, class := range strings.Fields(value) {
		if AllowedClasses[class] {
			kept = append(kept, class)
		}
	}
	return strings.Join(kept, " ")
}

// safeLink says whether a href may be kept, and in what form.
func safeLink(raw string) (string, bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false
	}
	// A relative link stays inside this installation, which is the one place a
	// link cannot take somebody anywhere surprising.
	if strings.HasPrefix(trimmed, "/") && !strings.HasPrefix(trimmed, "//") {
		return trimmed, true
	}
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", false
	}
	if !AllowedSchemes[strings.ToLower(parsed.Scheme)] {
		return "", false
	}
	return parsed.String(), true
}
