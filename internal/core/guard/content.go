package guard

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"git.codigosh.com/CodigoSH/LastHelp/internal/core/item"
)

// Verdict is what the content checks concluded. Reason is a translation key, so
// what reaches the sender is a sentence in their language rather than one this
// package composed in English.
type Verdict struct {
	OK     bool
	Reason string
	// Layer names which check refused, for the counter the operator reads. A
	// layer that never refuses anything is a layer to delete; one that refuses
	// nearly everything is biting people who did nothing wrong.
	Layer string
}

var accepted = Verdict{OK: true}

// MaxLinks is how many URLs a genuine report carries. Two is a stack trace with
// a paste link and a screenshot; a dozen is an advert.
const MaxLinks = 4

// DuplicateWindow is how far back an identical submission still counts as one.
const DuplicateWindow = 24 * time.Hour

var linkPattern = regexp.MustCompile(`(?i)\bhttps?://`)

// Inspect looks at what was written, using only what is already on this machine.
//
// Three cheap signals, in the order they cost:
//
//  1. The same text again. A flood is the same paragraph over and over, and the
//     hash makes noticing it a lookup rather than a read.
//  2. Too many links. Spam is links; that is what it is for.
//  3. Words the operator listed. Their board, their vocabulary — a word that
//     means nothing here is somebody else's daily traffic.
func (g Guard) Inspect(repoID int64, title, body, answers string, stoplist []string, now time.Time) (Verdict, error) {
	hash := item.BodyHash(title, body)

	var seen int
	err := g.DB.QueryRow(`SELECT count(*) FROM item WHERE body_hash = ? AND created_at > ?`,
		hash, now.Add(-DuplicateWindow).Unix()).Scan(&seen)
	if err != nil {
		return Verdict{}, fmt.Errorf("checking for duplicates: %w", err)
	}
	if seen > 0 {
		return Verdict{Reason: "error.already_sent", Layer: "duplicate"}, nil
	}

	// The answers to the form count as what was written, because they are: they
	// are free text a stranger typed, they go on the public page and into the
	// forge issue, and a limit that reads only the title and the body is a
	// limit with a box beside it that nobody checks.
	//
	// The duplicate hash above deliberately does not include them. It is stored
	// on the row and compared against every earlier one, so widening it would
	// mean nothing older could ever match again.
	written := title + " " + body + " " + answers

	if links := len(linkPattern.FindAllString(written, -1)); links > MaxLinks {
		return Verdict{Reason: "error.too_many_links", Layer: "links"}, nil
	}

	haystack := strings.ToLower(written)
	for _, word := range stoplist {
		word = strings.TrimSpace(strings.ToLower(word))
		if word == "" {
			continue
		}
		if strings.Contains(haystack, word) {
			return Verdict{Reason: "error.rejected", Layer: "stoplist"}, nil
		}
	}

	_ = repoID
	return accepted, nil
}

// Honeypot is a field a person never sees and never fills in.
//
// It catches the naive half of automated submission and costs one hidden input.
// It is not counted on for anything: anybody who looked at the page once knows
// which field to leave alone.
func Honeypot(value string) error {
	if strings.TrimSpace(value) != "" {
		return ErrHoneypot
	}
	return nil
}
