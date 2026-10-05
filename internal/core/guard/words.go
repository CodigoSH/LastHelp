package guard

import (
	"strings"
	"unicode"
)

// The operator's word list, and how a word is looked for.
//
// A word is matched whole in the scripts that put spaces between words, and as a
// fragment in the ones that do not. Matching by fragment everywhere refused
// "class" for listing "ass" and turned away the people the list was never meant
// for; matching whole everywhere would never find a word in Chinese or Japanese,
// which have no spaces to bound it. The script of the word decides, so nobody is
// asked which language they are writing in.

// FoldWord is the form a word is compared and counted in: lower case, with the
// accents of the Latin alphabets taken off, so "Casino" and "casiño" are one word.
// Written out rather than taken from a library, for the few ranges that matter.
func FoldWord(word string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(word)) {
		b.WriteRune(foldRune(r))
	}
	return b.String()
}

// foldGroups lists, for each plain letter, the accented forms that fold to it.
// Grouped by the letter they become, so a form cannot slip out of step with it.
var foldGroups = map[rune]string{
	'a': "àáâãäåāăą", 'c': "çćĉċč", 'd': "ďđ", 'e': "èéêëēĕėęě", 'g': "ĝğġģ", 'h': "ĥħ",
	'i': "ìíîïĩīĭįı", 'j': "ĵ", 'k': "ķ", 'l': "ĺļľŀł", 'n': "ñńņň", 'o': "òóôõöøōŏő",
	'r': "ŕŗř", 's': "śŝşš", 't': "ţťŧ", 'u': "ùúûüũūŭůűų", 'w': "ŵ", 'y': "ýÿŷ", 'z': "źżž",
	// Cyrillic ё is written е in running text more often than not.
	'е': "ё",
}

var foldTable = func() map[rune]rune {
	m := map[rune]rune{}
	for plain, forms := range foldGroups {
		for _, r := range forms {
			m[r] = plain
		}
	}
	return m
}()

func foldRune(r rune) rune {
	if to, ok := foldTable[r]; ok {
		return to
	}
	return r
}

// unspaced says whether a word is written in a script with no spaces between
// words, where it can only be looked for as a fragment.
func unspaced(word string) bool {
	for _, r := range word {
		if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul, unicode.Thai, unicode.Lao, unicode.Khmer, unicode.Myanmar) {
			return true
		}
	}
	return false
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsMark(r) || r == '_'
}

// FindWord returns the first word of the list the text contains, as its folded
// form (which is how it is counted), and whether there was one.
func FindWord(list []string, text string) (string, bool) {
	haystack := []rune(FoldWord(text))
	for _, entry := range list {
		word := FoldWord(entry)
		if word == "" {
			continue
		}
		if contains(haystack, []rune(word), unspaced(word)) {
			return word, true
		}
	}
	return "", false
}

func contains(haystack, word []rune, fragment bool) bool {
	for at := 0; at+len(word) <= len(haystack); at++ {
		if !equal(haystack[at:at+len(word)], word) {
			continue
		}
		if fragment {
			return true
		}
		before := at == 0 || !isWordRune(haystack[at-1])
		after := at+len(word) == len(haystack) || !isWordRune(haystack[at+len(word)])
		if before && after {
			return true
		}
	}
	return false
}

func equal(a, b []rune) bool {
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
