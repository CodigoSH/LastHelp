package web

import (
	"net/http"
	"testing"
	"time"
)

// addQuestion creates one of a project's own questions through the panel's
// route and returns its key.
func (i *instance) addQuestion(q map[string]any) string {
	i.t.Helper()
	resp := i.do(http.MethodPost, "/api/orgs/codigosh/repos/atlas/fields", q)
	body := decode(i.t, resp)
	if resp.StatusCode != http.StatusCreated {
		i.t.Fatalf("adding %v answered %d: %v", q, resp.StatusCode, body)
	}
	key, _ := body["key"].(string)
	return key
}

// trySubmit sends a report the way the page does and returns what the server
// answered, refusal included. Each report its own title: the same text twice is
// refused as already sent, which is another layer's business.
func (i *instance) trySubmit(kind, title string, fields map[string]string) (int, map[string]any) {
	i.t.Helper()
	base := "/api/orgs/codigosh/repos/atlas"
	token, _ := decode(i.t, i.do(http.MethodGet, base+"/form", nil))["token"].(string)
	i.tick.advance(5 * time.Second)
	resp := i.do(http.MethodPost, base+"/items", map[string]any{
		"kind": kind, "title": title, "body": "since this morning, " + title,
		"fields": fields, "token": token,
	})
	return resp.StatusCode, decode(i.t, resp)
}

// answersOf publishes a report and returns its answers as key → label and
// key → value.
func answersOf(t *testing.T, team *instance, id string) (labels, values map[string]string) {
	t.Helper()
	_ = status(t, team.do(http.MethodPost, "/api/items/"+id+"/moderation", map[string]string{"state": "published"}))
	fields, _ := decode(t, team.anonymous().do(http.MethodGet, "/api/items/"+id, nil))["fields"].([]any)
	labels, values = map[string]string{}, map[string]string{}
	for _, raw := range fields {
		f, _ := raw.(map[string]any)
		key, _ := f["key"].(string)
		labels[key], _ = f["label"].(string)
		values[key], _ = f["value"].(string)
	}
	return labels, values
}

func questionsSetup(t *testing.T) *instance {
	team := newInstance(t)
	team.claim()
	team.createOrg("codigosh")
	team.createRepo("codigosh", "atlas")
	return team
}

// The team writes the questions and the form is handed them; a visitor can read
// them and cannot add one.
func TestAProjectsOwnQuestionsReachItsForm(t *testing.T) {
	team := questionsSetup(t)
	room := team.addQuestion(map[string]any{"label": "Room", "type": "choice", "options": []string{"Kitchen", "Hall"}, "required": true, "for": "bug"})

	board := decode(t, team.anonymous().do(http.MethodGet, "/api/orgs/codigosh/repos/atlas", nil))
	fields, _ := board["fields"].([]any)
	if len(fields) != 1 {
		t.Fatalf("the form was handed %d questions, want 1: %v", len(fields), board["fields"])
	}
	q, _ := fields[0].(map[string]any)
	if q["key"] != room || q["label"] != "Room" || q["type"] != "choice" || q["required"] != true || q["for"] != "bug" {
		t.Errorf("the form reads the question as %v", q)
	}

	visitor := team.anonymous()
	resp := visitor.do(http.MethodPost, "/api/orgs/codigosh/repos/atlas/fields", map[string]any{"label": "Spam", "type": "short", "for": "both"})
	if code := status(t, resp); code == http.StatusCreated {
		t.Error("a visitor added a question to the form")
	}

	// Signed in and on the project's team, but not running its organisation: a
	// collaborator triages reports and does not decide what the form asks.
	grantOnBoard(t, team, "ana", "atlas")
	ana := team.signInAs("ana")
	if code := status(t, ana.do(http.MethodPost, "/api/orgs/codigosh/repos/atlas/fields", map[string]any{"label": "Mine", "type": "short", "for": "both"})); code != http.StatusForbidden {
		t.Errorf("a collaborator adding a question answered %d, want 403", code)
	}
	if code := status(t, ana.do(http.MethodDelete, "/api/orgs/codigosh/repos/atlas/fields/"+room, nil)); code != http.StatusForbidden {
		t.Errorf("a collaborator removing a question answered %d, want 403", code)
	}
}

// An answer has to be the kind its question takes, and a required question
// answered; what is kept carries the label the question had when it was asked.
func TestAnAnswerIsCheckedAndKeptWithItsLabel(t *testing.T) {
	team := questionsSetup(t)
	room := team.addQuestion(map[string]any{"label": "Room", "type": "choice", "options": []string{"Kitchen", "Hall"}, "required": true, "for": "bug"})
	since := team.addQuestion(map[string]any{"label": "Since", "type": "date", "for": "both"})
	budget := team.addQuestion(map[string]any{"label": "Budget", "type": "short", "for": "idea"})

	public := team.anonymous()
	if code, body := public.trySubmit("bug", "no room given", map[string]string{since: "2026-11-20"}); code != http.StatusBadRequest || body["error"] != "error.field_required" {
		t.Errorf("a problem without its required answer: %d %v", code, body)
	}
	if code, body := public.trySubmit("bug", "a room not on the list", map[string]string{room: "Garage"}); code != http.StatusBadRequest || body["error"] != "error.field_invalid" {
		t.Errorf("a choice that is not on the list: %d %v", code, body)
	}
	if code, body := public.trySubmit("bug", "a date in words", map[string]string{room: "Hall", since: "last week"}); code != http.StatusBadRequest || body["error"] != "error.field_invalid" {
		t.Errorf("words where a date goes: %d %v", code, body)
	}

	code, body := public.trySubmit("bug", "the light in the hall is out", map[string]string{room: "Hall", since: "2026-11-20", budget: "500"})
	if code != http.StatusCreated {
		t.Fatalf("a good problem was refused: %d %v", code, body)
	}
	id, _ := body["public_id"].(string)

	// Reworded and then removed: neither reaches the report already sent.
	if got := status(t, team.do(http.MethodPatch, "/api/orgs/codigosh/repos/atlas/fields/"+room,
		map[string]any{"label": "Which room", "options": []string{"Kitchen", "Hall"}, "required": true, "for": "bug"})); got != http.StatusOK {
		t.Fatalf("rewording answered %d", got)
	}
	if got := status(t, team.do(http.MethodDelete, "/api/orgs/codigosh/repos/atlas/fields/"+since, nil)); got != http.StatusOK {
		t.Fatalf("removing answered %d", got)
	}

	labels, values := answersOf(t, team, id)
	if labels[room] != "Room" || values[room] != "Hall" {
		t.Errorf("the room answer reads %q = %q, want it under the label it was asked with", labels[room], values[room])
	}
	if labels[since] != "Since" || values[since] != "2026-11-20" {
		t.Errorf("the answer to a removed question went: %q = %q", labels[since], values[since])
	}
	if _, kept := values[budget]; kept {
		t.Errorf("a question for ideas was kept on a problem: %v", values)
	}

	// The idea is asked the date and the budget, and not the room it would
	// otherwise have to answer.
	code, body = public.trySubmit("idea", "a second light in the hall", map[string]string{budget: "a little"})
	if code != http.StatusCreated {
		t.Fatalf("an idea was refused a required answer it is never asked: %d %v", code, body)
	}
}

// A question moves one place at a time, and the form is handed them in that
// order.
func TestTheFormAsksTheQuestionsInTheTeamsOrder(t *testing.T) {
	team := questionsSetup(t)
	first := team.addQuestion(map[string]any{"label": "First", "type": "short", "for": "both"})
	team.addQuestion(map[string]any{"label": "Second", "type": "long", "for": "both"})

	if got := status(t, team.do(http.MethodPost, "/api/orgs/codigosh/repos/atlas/fields/"+first+"/move", map[string]string{"direction": "down"})); got != http.StatusOK {
		t.Fatalf("moving answered %d", got)
	}
	fields, _ := decode(t, team.anonymous().do(http.MethodGet, "/api/orgs/codigosh/repos/atlas", nil))["fields"].([]any)
	var order []string
	for _, raw := range fields {
		f, _ := raw.(map[string]any)
		label, _ := f["label"].(string)
		order = append(order, label)
	}
	if len(order) != 2 || order[0] != "Second" || order[1] != "First" {
		t.Errorf("after moving the first down the form reads %v", order)
	}
	if got := status(t, team.do(http.MethodPost, "/api/orgs/codigosh/repos/atlas/fields/"+first+"/move", map[string]string{"direction": "sideways"})); got != http.StatusBadRequest {
		t.Errorf("a direction that is not one answered %d", got)
	}
}
