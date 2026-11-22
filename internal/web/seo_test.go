package web

import (
	"encoding/xml"
	"net/http"
	"strings"
	"testing"
)

func seoInstance(t *testing.T) (*instance, *Server) {
	t.Helper()
	team, srv := newPushInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "lastboard")
	return team, srv
}

func getStatus(t *testing.T, c *instance, path string) (int, string) {
	t.Helper()
	resp := c.do(http.MethodGet, path, nil)
	code := resp.StatusCode
	return code, body(t, resp)
}

// An address that leads to "not found" is a 404, with the same page, which is what draws that
// message; an address that leads to a screen is a 200.
func TestAnAddressThatDoesNotExistIsA404ButStillTheApplicationPage(t *testing.T) {
	team, _ := seoInstance(t)
	anon := team.anonymous()

	for path, want := range map[string]int{
		"/":                           http.StatusOK,
		"/codigosh":                   http.StatusOK,
		"/codigosh/":                  http.StatusOK,
		"/codigosh/lastboard":         http.StatusOK,
		"/codigosh/lastboard/new":     http.StatusOK,
		"/codigosh/lastboard/roadmap": http.StatusOK,
		"/codigosh/lastboard/queue":   http.StatusOK,
		"/sign-in":                    http.StatusOK,
		"/setup/done":                 http.StatusOK,
		"/review":                     http.StatusOK,
		"/panel/anything/at/all":      http.StatusOK,
		"/i/some-token":               http.StatusOK,

		"/nonexistent":            http.StatusNotFound,
		"/codigosh/nope":          http.StatusNotFound,
		"/nope/lastboard":         http.StatusNotFound,
		"/codigosh/lastboard/zzz": http.StatusNotFound,
		"/nope/lastboard/roadmap": http.StatusNotFound,
		"/a/b/c/d":                http.StatusNotFound,
		"/sign-in/x":              http.StatusNotFound,
	} {
		code, page := getStatus(t, anon, path)
		if code != want {
			t.Errorf("%s answered %d, want %d", path, code, want)
		}
		if !strings.Contains(page, "<title>") {
			t.Errorf("%s did not carry the application page: %.80q", path, page)
		}
	}
}

// What is private is a 404 to the public, as everywhere else, and a page to the team.
func TestAPrivateBoardIsA404ToThePublicAndAPageToTheTeam(t *testing.T) {
	team, _ := seoInstance(t)
	if _, err := team.db().Exec(`UPDATE repository SET is_private = 1 WHERE slug = 'lastboard'`); err != nil {
		t.Fatal(err)
	}
	if code, _ := getStatus(t, team.anonymous(), "/codigosh/lastboard"); code != http.StatusNotFound {
		t.Errorf("the public saw a private board: %d", code)
	}
	if code, _ := getStatus(t, team, "/codigosh/lastboard"); code != http.StatusOK {
		t.Errorf("the administrator was told a private board does not exist: %d", code)
	}

	if _, err := team.db().Exec(`UPDATE organization SET is_private = 1`); err != nil {
		t.Fatal(err)
	}
	if code, _ := getStatus(t, team.anonymous(), "/codigosh"); code != http.StatusNotFound {
		t.Errorf("the public saw a private organization: %d", code)
	}
}

func sitemapLocs(t *testing.T, raw string) []string {
	t.Helper()
	var set struct {
		URLs []struct {
			Loc string `xml:"loc"`
		} `xml:"url"`
	}
	if err := xml.Unmarshal([]byte(raw), &set); err != nil {
		t.Fatalf("the sitemap is not XML: %v\n%s", err, raw)
	}
	var out []string
	for _, u := range set.URLs {
		out = append(out, u.Loc)
	}
	return out
}

// The sitemap names the public pages, with the address the operator configured, and what is private
// is not in it, whoever asks.
func TestTheSitemapListsThePublicPagesAndNothingPrivate(t *testing.T) {
	team, srv := seoInstance(t)
	team.createRepo("codigosh", "secret")
	if _, err := team.db().Exec(`UPDATE repository SET is_private = 1 WHERE slug = 'secret'`); err != nil {
		t.Fatal(err)
	}
	team.createOrg("hidden")
	team.createRepo("hidden", "inside")
	if _, err := team.db().Exec(`UPDATE organization SET is_private = 1 WHERE slug = 'hidden'`); err != nil {
		t.Fatal(err)
	}

	// No public address, no sitemap: never one with guessed addresses.
	if code, _ := getStatus(t, team.anonymous(), "/sitemap.xml"); code != http.StatusNotFound {
		t.Errorf("a sitemap with no public address answered %d", code)
	}

	srv.cfg.PublicURL = "https://help.example.com/"
	for name, c := range map[string]*instance{"a crawler": team.anonymous(), "the administrator": team} {
		code, raw := getStatus(t, c, "/sitemap.xml")
		if code != http.StatusOK {
			t.Fatalf("%s got %d", name, code)
		}
		got := map[string]bool{}
		for _, loc := range sitemapLocs(t, raw) {
			got[loc] = true
		}
		for _, want := range []string{
			"https://help.example.com/", "https://help.example.com/codigosh",
			"https://help.example.com/codigosh/lastboard", "https://help.example.com/codigosh/lastboard/roadmap",
		} {
			if !got[want] {
				t.Errorf("%s: the sitemap is missing %s", name, want)
			}
		}
		for loc := range got {
			if strings.Contains(loc, "secret") || strings.Contains(loc, "hidden") || strings.Contains(loc, "inside") {
				t.Errorf("%s: the sitemap names something private: %s", name, loc)
			}
			if strings.Contains(loc, "//codigosh") || strings.HasSuffix(loc, "//") {
				t.Errorf("%s: a doubled slash in %s", name, loc)
			}
		}
	}
}

// robots.txt says where the sitemap is, when there is one to point at.
func TestRobotsTxtPointsAtTheSitemapOnlyWhenThereIsOne(t *testing.T) {
	team, srv := seoInstance(t)
	_, plain := getStatus(t, team.anonymous(), "/robots.txt")
	if strings.Contains(plain, "Sitemap:") {
		t.Errorf("robots.txt points at a sitemap that does not exist:\n%s", plain)
	}

	srv.cfg.PublicURL = "https://help.example.com"
	_, with := getStatus(t, team.anonymous(), "/robots.txt")
	if !strings.Contains(with, "Sitemap: https://help.example.com/sitemap.xml") {
		t.Errorf("robots.txt does not say where the sitemap is:\n%s", with)
	}
	if !strings.Contains(with, "User-agent: *") {
		t.Errorf("the rules went missing:\n%s", with)
	}
}
