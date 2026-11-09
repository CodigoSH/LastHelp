package web

import (
	"encoding/json"
	"io/fs"
	"regexp"
	"strings"
	"sync"
)

// The words a notification is written from, for one person.
//
// A notification that goes to a single person is written in their language: the
// kind of report ("Idea", "Fallo") and the sentence of a test come from the same
// catalogues the page reads, which are in the binary. A channel shared by a whole
// team has no one language and says it with marks instead (see notify.Message).

var catalogues sync.Map // language code → map[key]string

var languageCode = regexp.MustCompile(`^[a-z]{2,3}$`)

// words is the catalogue of a language, or nil if the binary has none. The code
// is checked before it is made into a path: it comes out of the database.
func (s *Server) words(lang string) map[string]string {
	if !languageCode.MatchString(lang) {
		return nil
	}
	if cached, ok := catalogues.Load(lang); ok {
		return cached.(map[string]string)
	}
	raw, err := fs.ReadFile(s.assets, "locales/"+lang+".json")
	if err != nil {
		return nil
	}
	var table map[string]string
	if json.Unmarshal(raw, &table) != nil {
		return nil
	}
	catalogues.Store(lang, table)
	return table
}

// say is one sentence in a language, with its placeholders filled. A key a
// language lacks falls back to English, and one English lacks is the key itself:
// a notification that says its key is a bug somebody can see and report, which
// is better than one that says nothing.
func (s *Server) say(lang, key string, params map[string]string) string {
	text, ok := s.words(lang)[key]
	if !ok {
		if text, ok = s.words("en")[key]; !ok {
			return key
		}
	}
	for name, value := range params {
		text = strings.ReplaceAll(text, "{"+name+"}", isolate(lang, value))
	}
	return text
}

// isolate keeps a name that is written the other way from dragging the words
// around it with it: in a language that reads right to left, a repository called
// "Atlas" would otherwise pull the punctuation beside it to the wrong side.
func isolate(lang, value string) string {
	if direction(lang) != "rtl" {
		return value
	}
	return "⁨" + value + "⁩"
}

// languageOf is the language an account reads the product in, English when it
// has not chosen or chose one this binary does not have.
func (s *Server) languageOf(userID int64) string {
	lang, err := s.accounts().Language(userID)
	if err != nil || s.words(lang) == nil {
		return "en"
	}
	return lang
}

// kindWord is what a kind of report is called in a language.
func (s *Server) kindWord(lang, kind string) string {
	switch kind {
	case "idea", "bug":
		return s.say(lang, "kind."+kind, nil)
	case "comment":
		return s.say(lang, "notify.comment", nil)
	}
	return ""
}
