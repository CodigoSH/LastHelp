package web

import (
	"sort"
	"strconv"
	"strings"
)

// locales are the languages this build ships. The catalogues live in
// web/public/locales and are copied into the embedded assets, so this list and
// what exists on disk cannot be checked against each other at compile time —
// which is why a test does it.
var locales = []string{
	"en", "es", "de", "fr", "it", "pt", "nl", "pl", "tr", "el",
	"ru", "zh", "ja", "ko", "hi", "bn", "id", "ar", "fa", "ur",
}

// knownLocale says whether a code is one this build ships.
//
// The gate on anything stored: a language is written into a row and then into
// <html lang>, and a value nobody checked is a value the browser is told to
// hyphenate and a screen reader is told to pronounce.
func knownLocale(code string) bool {
	for _, c := range locales {
		if c == code {
			return true
		}
	}
	return false
}

// rtlLocales read right to left.
//
// There are two of these lists, this one and the frontend's, because the first
// paint happens before any script runs and the language can change afterwards
// without a reload. Two hand-written lists drift the first time one of them
// gains a language, so a test compares them.
var rtlLocales = []string{"ar", "fa", "ur"}

func isRTL(code string) bool {
	for _, c := range rtlLocales {
		if c == code {
			return true
		}
	}
	return false
}

func direction(code string) string {
	if isRTL(code) {
		return "rtl"
	}
	return "ltr"
}

// negotiateLanguage picks the language for the first paint from Accept-Language.
//
// The server cannot read the choice the browser stored: that lives in
// localStorage, which is what the script reads once it runs. So the first paint
// uses what the browser asks for, and the script corrects it if the person
// chose something else. Serving lang="en" to everybody would be telling every
// screen reader that an Arabic page is English.
func negotiateLanguage(header string) string {
	type candidate struct {
		code    string
		quality float64
	}

	var candidates []candidate
	for _, part := range strings.Split(header, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		tag, params, _ := strings.Cut(part, ";")
		tag = strings.TrimSpace(tag)
		quality := 1.0
		if q, ok := strings.CutPrefix(strings.TrimSpace(params), "q="); ok {
			if parsed, err := strconv.ParseFloat(q, 64); err == nil {
				quality = parsed
			}
		}

		// "es-419" and "pt-BR" are the language we ship plus a region we do not
		// distinguish.
		base, _, _ := strings.Cut(strings.ToLower(tag), "-")
		candidates = append(candidates, candidate{code: base, quality: quality})
	}

	// Stable sort by quality, so equal weights keep the order they were sent in
	// — which is the order the person put them in.
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].quality > candidates[j].quality
	})

	for _, c := range candidates {
		if c.quality <= 0 {
			continue // q=0 means "not this one"
		}
		for _, shipped := range locales {
			if c.code == shipped {
				return shipped
			}
		}
	}
	return "en"
}
