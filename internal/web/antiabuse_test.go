package web

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

// send posts a report as a stranger and says what the server answered.
func send(t *testing.T, public *instance, title, body string) (int, map[string]any) {
	t.Helper()
	base := "/api/orgs/acme/repos/atlas"
	form := decode(t, public.do(http.MethodGet, base+"/form", nil))
	public.tick.advance(5 * time.Second)
	resp := public.do(http.MethodPost, base+"/items", map[string]any{
		"kind": "idea", "title": title, "body": body, "token": form["token"],
	})
	return resp.StatusCode, decode(t, resp)
}

func wordsOf(t *testing.T, who *instance) (words []map[string]any, layers []map[string]any) {
	t.Helper()
	resp := who.do(http.MethodGet, "/api/antiabuse", nil)
	body := decode(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reading the anti-abuse screen: %d %v", resp.StatusCode, body)
	}
	for _, w := range body["words"].([]any) {
		words = append(words, w.(map[string]any))
	}
	for _, l := range body["layers"].([]any) {
		layers = append(layers, l.(map[string]any))
	}
	return
}

// TestTheWordListIsTheAdministratorsAndCountsWhatItRefuses.
func TestTheWordListIsTheAdministratorsAndCountsWhatItRefuses(t *testing.T) {
	admin, owner, _ := teamSetup(t)
	public := admin.anonymous()

	if got := status(t, owner.do(http.MethodGet, "/api/antiabuse", nil)); got != http.StatusForbidden {
		t.Errorf("an owner read the server's list: %d", got)
	}
	if got := status(t, owner.do(http.MethodPut, "/api/antiabuse/words", map[string]any{"words": []string{"x"}})); got != http.StatusForbidden {
		t.Errorf("an owner changed the server's list: %d", got)
	}

	// Case and accents are one word; blanks go.
	resp := admin.do(http.MethodPut, "/api/antiabuse/words", map[string]any{"words": []string{" Casino ", "casiño", "", "ass"}})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("setting the list: %d %v", resp.StatusCode, decode(t, resp))
	}
	words, _ := wordsOf(t, admin)
	if len(words) != 2 || words[0]["word"] != "Casino" || words[1]["word"] != "ass" {
		t.Fatalf("the list is %v, want Casino and ass once each", words)
	}

	if code, body := send(t, public, "great CASIÑO deals", "visit now for the best offers"); code != http.StatusBadRequest {
		t.Errorf("a listed word got through: %d %v", code, body)
	}
	if code, body := send(t, public, "a class for beginners", "teach a grass roots course"); code != http.StatusCreated {
		t.Errorf("a word that only contains a listed one was refused: %d %v", code, body)
	}

	words, layers := wordsOf(t, admin)
	if words[0]["count"] != float64(1) || words[1]["count"] != float64(0) {
		t.Errorf("the counts are %v, want 1 for Casino and 0 for ass", words)
	}
	found := false
	for _, l := range layers {
		if l["layer"] == "stoplist" && l["org"] == "acme" && l["repo"] == "atlas" && l["count"] == float64(1) {
			found = true
		}
	}
	if !found {
		t.Errorf("the refusals by layer do not show the stoplist on acme/atlas: %v", layers)
	}

	// Taking the word off takes its count; adding it again starts from nothing.
	admin.do(http.MethodPut, "/api/antiabuse/words", map[string]any{"words": []string{"ass"}}).Body.Close()
	admin.do(http.MethodPut, "/api/antiabuse/words", map[string]any{"words": []string{"ass", "casino"}}).Body.Close()
	words, _ = wordsOf(t, admin)
	if words[1]["count"] != float64(0) {
		t.Errorf("a word put back inherited the count of its earlier time: %v", words)
	}

	for name, list := range map[string][]string{
		"too long":  {strings.Repeat("a", 61)},
		"a comma":   {"a,b"},
		"a control": {"a\x00b"},
		"too many":  manyWords(501),
	} {
		if got := status(t, admin.do(http.MethodPut, "/api/antiabuse/words", map[string]any{"words": list})); got != http.StatusBadRequest {
			t.Errorf("%s was accepted: %d", name, got)
		}
	}
}

func manyWords(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "word" + strings.Repeat("x", i%7) + string(rune('a'+i%26)) + string(rune('A'+(i/26)%26)) + string(rune('a'+(i/676)%26))
	}
	return out
}

// TestARejectedReportCanAddItsGiveawayWordForTheAdministratorOnly.
func TestARejectedReportCanAddItsGiveawayWordForTheAdministratorOnly(t *testing.T) {
	admin, owner, _ := teamSetup(t)
	public := admin.anonymous()

	id1, _ := public.submit("acme", "atlas", "idea", "first one")
	id2, _ := public.submit("acme", "atlas", "idea", "second one")
	id3, _ := public.submit("acme", "atlas", "idea", "third one")
	moderate := func(who *instance, id string, body map[string]any) int {
		return status(t, who.do(http.MethodPost, "/api/items/"+id+"/moderation", body))
	}

	// An owner triages the board but does not write the server's list, and the
	// report is left as it was.
	if got := moderate(owner, id1, map[string]any{"state": "rejected", "add_word": "viagra"}); got != http.StatusForbidden {
		t.Errorf("an owner added to the server's list: %d", got)
	}
	if words, _ := wordsOf(t, admin); len(words) != 0 {
		t.Errorf("a refused add still reached the list: %v", words)
	}
	if got := moderate(owner, id1, map[string]any{"state": "rejected"}); got != http.StatusOK {
		t.Errorf("the owner could not reject without a word: %d", got)
	}

	// A word makes sense only with a decision that turns the report down.
	if got := moderate(admin, id2, map[string]any{"state": "published", "add_word": "viagra"}); got != http.StatusBadRequest {
		t.Errorf("a word was accepted with an approval: %d", got)
	}
	if got := moderate(admin, id2, map[string]any{"state": "spam", "add_word": "bad,word"}); got != http.StatusBadRequest {
		t.Errorf("a word with a comma was accepted: %d", got)
	}
	if got := moderate(admin, id3, map[string]any{"state": "spam", "add_word": " Viagra "}); got != http.StatusOK {
		t.Fatalf("the administrator rejecting with a word: %d", got)
	}
	words, _ := wordsOf(t, admin)
	if len(words) != 1 || words[0]["word"] != "Viagra" {
		t.Errorf("the list is %v, want Viagra", words)
	}
	// Once more adds nothing.
	moderate(admin, id2, map[string]any{"state": "rejected", "add_word": "viagra"})
	if words, _ := wordsOf(t, admin); len(words) != 1 {
		t.Errorf("a word on the list twice: %v", words)
	}
}
