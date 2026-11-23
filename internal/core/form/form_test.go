package form

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"git.codigosh.com/CodigoSH/LastHelp/internal/platform/database"
)

// store opens a real database with two projects in it, 1 and 2.
func store(t *testing.T) Store {
	t.Helper()
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	for _, stmt := range []string{
		`INSERT INTO organization (id, slug, name, forge_kind, base_url, forge_owner, created_at) VALUES (1, 'acme', 'Acme', 'none', '', '', 0)`,
		`INSERT INTO repository (id, org_id, slug, name, forge_repo, created_at) VALUES (1, 1, 'atlas', 'Atlas', '', 0)`,
		`INSERT INTO repository (id, org_id, slug, name, forge_repo, created_at) VALUES (2, 1, 'orbit', 'Orbit', '', 0)`,
	} {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatal(err)
		}
	}
	return Store{DB: db}
}

func TestAnAnswerIsTheKindOfAnswerTheQuestionTakes(t *testing.T) {
	line := Field{Type: Short}
	choice := Field{Type: Choice, Options: []string{"Kitchen", "Hall"}}
	date := Field{Type: Date}
	cases := []struct {
		name  string
		field Field
		value string
		want  error
	}{
		{"an empty answer to a question that is not required", line, "  ", nil},
		{"an empty answer to a required one", Field{Type: Short, Required: true}, " ", ErrRequired},
		// Counted in characters: a limit in bytes lets a third as much Japanese in.
		{"a line of exactly the limit, in Japanese", line, strings.Repeat("ぜ", MaxShort), nil},
		{"a line past the limit", line, strings.Repeat("ぜ", MaxShort+1), ErrTooLong},
		{"a paragraph is not a line", Field{Type: Long}, strings.Repeat("ぜ", MaxShort+1), nil},
		{"one of the choices", choice, "Hall", nil},
		{"something that is not a choice", choice, "Garage", ErrNotAnOption},
		{"a date", date, "2026-11-23", nil},
		{"a day that does not exist", date, "2026-02-30", ErrNotADate},
		{"words where a date goes", date, "yesterday", ErrNotADate},
	}
	for _, c := range cases {
		if got := c.field.Check(c.value); !errors.Is(got, c.want) {
			t.Errorf("%s: %v, want %v", c.name, got, c.want)
		}
	}
}

func TestAQuestionIsAskedOfTheReportsItIsFor(t *testing.T) {
	for _, c := range []struct {
		for_      For
		bug, idea bool
	}{{Problems, true, false}, {Ideas, false, true}, {Both, true, true}} {
		f := Field{For: c.for_}
		if f.Asks("bug") != c.bug || f.Asks("idea") != c.idea {
			t.Errorf("a question for %q asks bug=%v idea=%v", c.for_, f.Asks("bug"), f.Asks("idea"))
		}
	}
}

func TestAQuestionThatCannotBeAskedIsRefused(t *testing.T) {
	s := store(t)
	for name, f := range map[string]Field{
		"no label":            {Label: "  ", Type: Short, For: Both},
		"a label too long":    {Label: strings.Repeat("ぜ", MaxLabel+1), Type: Short, For: Both},
		"no such kind":        {Label: "Where", Type: "colour", For: Both},
		"no such report":      {Label: "Where", Type: Short, For: "comment"},
		"a list with nothing": {Label: "Where", Type: Choice, For: Both, Options: []string{" ", ""}},
		"an option too long":  {Label: "Where", Type: Choice, For: Both, Options: []string{strings.Repeat("ぜ", MaxOption+1)}},
		"too many options":    {Label: "Where", Type: Choice, For: Both, Options: manyOptions(MaxOptions + 1)},
	} {
		f.RepoID = 1
		if _, err := s.Create(f); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: %v, want ErrInvalid", name, err)
		}
	}

	// A label of exactly the limit, in Japanese, is a label.
	if _, err := s.Create(Field{RepoID: 1, Label: strings.Repeat("ぜ", MaxLabel), Type: Short, For: Both}); err != nil {
		t.Errorf("a label of exactly the limit was refused: %v", err)
	}
}

func manyOptions(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = strings.Repeat("o", i+1)
	}
	return out
}

func TestAListIsTidiedAndOnlyAListKeepsOptions(t *testing.T) {
	s := store(t)
	list, err := s.Create(Field{RepoID: 1, Label: " Room ", Type: Choice, For: Both, Options: []string{" Kitchen", "Hall", "Kitchen", ""}})
	if err != nil {
		t.Fatal(err)
	}
	if list.Label != "Room" || strings.Join(list.Options, "|") != "Kitchen|Hall" {
		t.Errorf("stored as %q %v, want the label trimmed and each option once", list.Label, list.Options)
	}
	line, err := s.Create(Field{RepoID: 1, Label: "Name", Type: Short, For: Both, Options: []string{"x"}})
	if err != nil {
		t.Fatal(err)
	}
	all, _ := s.List(1)
	if len(all) != 2 || len(all[1].Options) != 0 || all[1].Key != line.Key {
		t.Errorf("a line kept options it has no use for: %+v", all)
	}
	if strings.Join(all[0].Options, "|") != "Kitchen|Hall" {
		t.Errorf("the list did not come back as it went in: %v", all[0].Options)
	}
}

func TestAProjectHasAtMostItsShareOfQuestions(t *testing.T) {
	s := store(t)
	keys := map[string]bool{}
	for i := 0; i < MaxFields; i++ {
		f, err := s.Create(Field{RepoID: 1, Label: "Q", Type: Short, For: Both})
		if err != nil {
			t.Fatalf("question %d: %v", i+1, err)
		}
		if f.Key == "version" || f.Key == "steps" || !strings.HasPrefix(f.Key, "f") || keys[f.Key] {
			t.Errorf("question %d got the key %q", i+1, f.Key)
		}
		keys[f.Key] = true
		if f.Position != i {
			t.Errorf("question %d went to position %d, not the end", i+1, f.Position)
		}
	}
	if _, err := s.Create(Field{RepoID: 1, Label: "Q", Type: Short, For: Both}); !errors.Is(err, ErrTooMany) {
		t.Errorf("one past the limit: %v, want ErrTooMany", err)
	}
	// The limit is a project's, not the installation's.
	if _, err := s.Create(Field{RepoID: 2, Label: "Q", Type: Short, For: Both}); err != nil {
		t.Errorf("another project could not add its first question: %v", err)
	}
}

func TestAnEditKeepsTheKindOfAnswer(t *testing.T) {
	s := store(t)
	f, err := s.Create(Field{RepoID: 1, Label: "When", Type: Date, For: Problems})
	if err != nil {
		t.Fatal(err)
	}
	changed, err := s.Update(1, f.Key, Field{Label: "Since when", Type: Short, Required: true, For: Both})
	if err != nil {
		t.Fatal(err)
	}
	if changed.Type != Date || changed.Label != "Since when" || !changed.Required || changed.For != Both {
		t.Errorf("after the edit: %+v, want a date still, reworded, required and for both", changed)
	}
	if _, err := s.Update(2, f.Key, Field{Label: "x", For: Both}); !errors.Is(err, ErrNotFound) {
		t.Errorf("another project edited this one's question: %v", err)
	}
	if err := s.Delete(2, f.Key); !errors.Is(err, ErrNotFound) {
		t.Errorf("another project removed this one's question: %v", err)
	}
	if err := s.Delete(1, f.Key); err != nil {
		t.Fatal(err)
	}
	if all, _ := s.List(1); len(all) != 0 {
		t.Errorf("a removed question is still asked: %+v", all)
	}
}

func TestAQuestionMovesOnePlaceAndNotPastTheEnds(t *testing.T) {
	s := store(t)
	var keys []string
	for _, label := range []string{"A", "B", "C"} {
		f, err := s.Create(Field{RepoID: 1, Label: label, Type: Short, For: Both})
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, f.Key)
	}
	order := func() string {
		all, _ := s.List(1)
		var out []string
		for _, f := range all {
			out = append(out, f.Label)
		}
		return strings.Join(out, "")
	}
	steps := []struct {
		key  string
		up   bool
		want string
	}{
		{keys[2], true, "ACB"},
		{keys[2], true, "CAB"},
		{keys[2], true, "CAB"},  // already first
		{keys[1], false, "CAB"}, // already last
		{keys[0], false, "CBA"},
	}
	for i, st := range steps {
		if err := s.Move(1, st.key, st.up); err != nil {
			t.Fatal(err)
		}
		if got := order(); got != st.want {
			t.Errorf("step %d: the form reads %s, want %s", i+1, got, st.want)
		}
	}
	if err := s.Move(2, keys[0], true); !errors.Is(err, ErrNotFound) {
		t.Errorf("another project moved this one's question: %v", err)
	}
}
