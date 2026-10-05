package web

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/authz"
	"git.codigosh.com/CodigoSH/LastHelp/internal/core/guard"
)

// The operator's word list and the counts of what the layers refused.
//
// One list for the whole server, with no language on it: it is a notebook of the
// words that turn up in this operator's own queue, not a dictionary, and the
// language something arrives in is not the operator's to choose.

const (
	maxWords      = 500
	maxWordLength = 60
	// wordCountPrefix is the setting a word's count lives in, under the folded
	// form. One row per word, made when the word first refuses something and
	// removed with the word: a count outliving its word is a row nobody can see
	// or clean, and a word added again later would inherit numbers that were
	// never its own.
	wordCountPrefix = "refused_word."
	layerPrefix     = "refused."
)

// refusedWord counts what one listed word refused, beside the count of its layer.
func (s *Server) refusedWord(word string) {
	if word == "" {
		return
	}
	if _, err := s.db.Exec(`INSERT INTO settings (key, value) VALUES (?, '1')
		ON CONFLICT (key) DO UPDATE SET value = CAST(CAST(value AS INTEGER) + 1 AS TEXT)`,
		wordCountPrefix+word); err != nil {
		slog.Error("counting a refusal by word", "error", err)
	}
}

// cleanWords is the list as it will be stored: trimmed, without blanks, and with
// each word once however its case or accents were typed. The failure is a
// translation key.
func cleanWords(in []string) ([]string, string) {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, raw := range in {
		word := strings.TrimSpace(raw)
		if word == "" {
			continue
		}
		if len([]rune(word)) > maxWordLength {
			return nil, "error.word_too_long"
		}
		for _, r := range word {
			// A comma is what the list is stored with; a control character is
			// not a character anybody typed on purpose.
			if r == ',' || unicode.IsControl(r) {
				return nil, "error.word_chars"
			}
		}
		folded := guard.FoldWord(word)
		if seen[folded] {
			continue
		}
		seen[folded] = true
		out = append(out, word)
	}
	if len(out) > maxWords {
		return nil, "error.words_too_many"
	}
	return out, ""
}

// storeWords writes the list, and takes the counts of the words that left it.
func (s *Server) storeWords(words []string) error {
	store := s.settings()
	kept := map[string]bool{}
	for _, w := range words {
		kept[guard.FoldWord(w)] = true
	}
	for _, old := range s.stoplist() {
		folded := guard.FoldWord(old)
		if folded != "" && !kept[folded] {
			if err := store.Delete(wordCountPrefix + folded); err != nil {
				return err
			}
		}
	}
	return store.Set(StoplistSetting, strings.Join(words, ","))
}

// addWord puts one word on the list if it is not there already.
func (s *Server) addWord(word string) string {
	words := append(s.currentWords(), word)
	cleaned, failure := cleanWords(words)
	if failure != "" {
		return failure
	}
	if err := s.storeWords(cleaned); err != nil {
		slog.Error("adding a word to the list", "error", err)
		return "error.internal"
	}
	return ""
}

func (s *Server) currentWords() []string {
	list := s.stoplist()
	out := make([]string, 0, len(list))
	for _, w := range list {
		if w = strings.TrimSpace(w); w != "" {
			out = append(out, w)
		}
	}
	return out
}

type listedWord struct {
	Word  string `json:"word"`
	Count int    `json:"count"`
}

type layerCount struct {
	Org      string `json:"org"`
	OrgName  string `json:"org_name"`
	Repo     string `json:"repo"`
	RepoName string `json:"repo_name"`
	Layer    string `json:"layer"`
	Count    int    `json:"count"`
}

func (s *Server) describeAntiabuse() (map[string]any, error) {
	store := s.settings()
	counts, err := store.WithPrefix(wordCountPrefix)
	if err != nil {
		return nil, err
	}
	words := []listedWord{}
	for _, w := range s.currentWords() {
		n, _ := strconv.Atoi(counts[guard.FoldWord(w)])
		words = append(words, listedWord{Word: w, Count: n})
	}

	raw, err := store.WithPrefix(layerPrefix)
	if err != nil {
		return nil, err
	}
	layers := []layerCount{}
	for key, value := range raw {
		id, layer, ok := strings.Cut(key, ".")
		repoID, err := strconv.ParseInt(id, 10, 64)
		if !ok || err != nil {
			continue
		}
		repo, err := s.orgs().RepositoryByID(repoID)
		if err != nil {
			continue // a repository that was deleted keeps no row on this screen
		}
		organisation, err := s.orgs().OrganizationByID(repo.OrgID)
		if err != nil {
			continue
		}
		n, _ := strconv.Atoi(value)
		layers = append(layers, layerCount{
			Org: organisation.Slug, OrgName: organisation.Name, Repo: repo.Slug, RepoName: repo.Name,
			Layer: layer, Count: n,
		})
	}
	sort.Slice(layers, func(i, j int) bool {
		a, b := layers[i], layers[j]
		if a.Org != b.Org {
			return a.Org < b.Org
		}
		if a.Repo != b.Repo {
			return a.Repo < b.Repo
		}
		return a.Layer < b.Layer
	})
	return map[string]any{"words": words, "layers": layers, "max_words": maxWords, "max_length": maxWordLength}, nil
}

func (s *Server) handleGetAntiabuse(w http.ResponseWriter, r *http.Request) {
	if !s.may(w, r, authz.ManageServer, authz.Scope{}) {
		return
	}
	body, err := s.describeAntiabuse()
	if err != nil {
		slog.Error("reading the anti-abuse screen", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusOK, body)
}

type wordListChange struct {
	Words []string `json:"words"`
}

func (s *Server) handleSetWords(w http.ResponseWriter, r *http.Request) {
	if !s.may(w, r, authz.ManageServer, authz.Scope{}) {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, panelBodyLimit)
	var in wordListChange
	if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
		writeError(w, http.StatusBadRequest, "error.bad_request")
		return
	}
	words, failure := cleanWords(in.Words)
	if failure != "" {
		writeError(w, http.StatusBadRequest, failure)
		return
	}
	if err := s.storeWords(words); err != nil {
		slog.Error("storing the word list", "error", err)
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	body, err := s.describeAntiabuse()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "error.internal")
		return
	}
	writeJSON(w, http.StatusOK, body)
}
