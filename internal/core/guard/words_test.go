package guard

import "testing"

// TestAWordIsLookedForWholeWhereWordsHaveSpaces: "ass" does not refuse "class",
// and does refuse "ass" wherever it stands on its own.
func TestAWordIsLookedForWholeWhereWordsHaveSpaces(t *testing.T) {
	for _, tc := range []struct {
		text string
		want bool
	}{
		{"a class on its own", false},
		{"classic and grass", false},
		{"what an ass", true},
		{"Ass.", true},
		{"(ass)", true},
		{"ass", true},
		{"a-ass-b", true},
		{"pass_ass", false},
		{"ass1", false},
	} {
		if _, got := FindWord([]string{"ass"}, tc.text); got != tc.want {
			t.Errorf("%q against ass: got %v, want %v", tc.text, got, tc.want)
		}
	}
}

// TestAWordInAScriptWithoutSpacesIsLookedForAsAFragment: a Chinese or Japanese
// word has no space to be bounded by, so it is found inside the sentence.
func TestAWordInAScriptWithoutSpacesIsLookedForAsAFragment(t *testing.T) {
	for _, tc := range []struct{ word, text string }{
		{"赌博", "这个网站提供赌博服务"},
		{"カジノ", "オンラインカジノで稼ごう"},
		{"도박", "도박사이트입니다"},
	} {
		if got, ok := FindWord([]string{tc.word}, tc.text); !ok || got != tc.word {
			t.Errorf("%q was not found in %q", tc.word, tc.text)
		}
	}
	if _, ok := FindWord([]string{"赌博"}, "这个网站提供音乐服务"); ok {
		t.Error("a word was found in a sentence without it")
	}
}

// TestAccentsAndCaseDoNotHideAWord.
func TestAccentsAndCaseDoNotHideAWord(t *testing.T) {
	for _, tc := range []struct{ word, text string }{
		{"casino", "CASIÑO gratis"},
		{"Casiño", "visita el casino"},
		{"Ölçü", "bir olcu daha"},
		{"ёлка", "купить елка дёшево"},
	} {
		if _, ok := FindWord([]string{tc.word}, tc.text); !ok {
			t.Errorf("%q was not found in %q", tc.word, tc.text)
		}
	}
}

// TestAWordWithPunctuationIsBoundedByWhatIsAroundIt: a domain or a phrase is a
// word of the list too, whole.
func TestAWordWithPunctuationIsBoundedByWhatIsAroundIt(t *testing.T) {
	list := []string{"free money", "bit.ly"}
	if _, ok := FindWord(list, "get FREE   money now"); ok {
		t.Error("a phrase matched across a different amount of space")
	}
	if _, ok := FindWord(list, "get free money now"); !ok {
		t.Error("a phrase was not found")
	}
	if _, ok := FindWord(list, "see http://bit.ly/x"); !ok {
		t.Error("a domain was not found")
	}
	if _, ok := FindWord(list, "orbit.lyric"); ok {
		t.Error("a domain matched inside another word")
	}
}

func TestTheFoldTableIsWhole(t *testing.T) {
	if got := FoldWord("ÀÉÎÕÜÑçŠžøł"); got != "aeiouncszol" {
		t.Errorf("folded to %q", got)
	}
}
