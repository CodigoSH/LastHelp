package bridge

import "strings"

// neutralise makes what a stranger wrote inert on a forge.
//
// An issue body is rendered as Markdown, and Markdown does things: an `@name` tells
// somebody on the forge they were mentioned, `![](url)` fetches a picture from a
// host the writer chose, `[text](url)` hides where a link goes, `#12` links another
// issue, raw HTML is passed to the forge's own sanitiser, and on one forge a line that
// starts with `/` is a command (`/close`, `/assign`). The text arrives from somebody
// nobody has met, after a person on the team approved the report, and none of that
// was ever meant to happen on the organisation's own forge.
//
// So the characters that start those things are escaped with a backslash, which every
// forge reads as "this character, as it is": `@` `<` `[` `\`, a `#` before a digit,
// and a `/` that starts a line. What it leaves alone is what carries no effect —
// emphasis, lists, quotes, bare addresses — and, deliberately, code: a span in
// backticks and a fenced block are copied as they are, because mentions and links
// are already inert inside them and escaping there would put a stray backslash in
// every `array[0]` of a bug report.
//
// A fence the writer never closed would swallow the lines this product appends after
// the text, so one left open is closed here.
func neutralise(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines)+1)

	var (
		inFence   bool
		fenceChar byte
		fenceLen  int
	)
	for _, line := range lines {
		indent := len(line) - len(strings.TrimLeft(line, " "))
		rest := line[indent:]

		if inFence {
			out = append(out, line)
			if indent <= 3 && closesFence(rest, fenceChar, fenceLen) {
				inFence = false
			}
			continue
		}
		if char, n, ok := opensFence(rest); ok && indent <= 3 {
			inFence, fenceChar, fenceLen = true, char, n
			out = append(out, line)
			continue
		}
		out = append(out, escapeLine(line))
	}
	if inFence {
		out = append(out, strings.Repeat(string(fenceChar), fenceLen))
	}
	return strings.Join(out, "\n")
}

// opensFence reports a line that opens a fenced block: three or more backticks or
// tildes, and for backticks an info string with no backtick in it.
func opensFence(rest string) (char byte, n int, ok bool) {
	if len(rest) < 3 || (rest[0] != '`' && rest[0] != '~') {
		return 0, 0, false
	}
	char = rest[0]
	for n < len(rest) && rest[n] == char {
		n++
	}
	if n < 3 {
		return 0, 0, false
	}
	if char == '`' && strings.ContainsRune(rest[n:], '`') {
		return 0, 0, false
	}
	return char, n, true
}

// closesFence reports the line that ends a fence: at least as many of the same
// character, and nothing after them but spaces.
func closesFence(rest string, char byte, length int) bool {
	n := 0
	for n < len(rest) && rest[n] == char {
		n++
	}
	return n >= length && strings.TrimSpace(rest[n:]) == ""
}

// escapeLine escapes one line of prose, leaving its code spans as written.
func escapeLine(line string) string {
	var b strings.Builder
	b.Grow(len(line) + 8)
	atStart := true

	for i := 0; i < len(line); {
		c := line[i]

		if c == '`' {
			run := 1
			for i+run < len(line) && line[i+run] == '`' {
				run++
			}
			if end := closingRun(line, i+run, run); end >= 0 {
				b.WriteString(line[i:end])
				i, atStart = end, false
				continue
			}
			b.WriteString(line[i : i+run])
			i, atStart = i+run, false
			continue
		}

		switch {
		case c == '\\', c == '<', c == '[', c == '@':
			b.WriteByte('\\')
			b.WriteByte(c)
		case c == '#' && i+1 < len(line) && line[i+1] >= '0' && line[i+1] <= '9' && (i == 0 || line[i-1] != '/'):
			b.WriteString(`\#`)
		case c == '/' && atStart:
			b.WriteString(`\/`)
		default:
			b.WriteByte(c)
		}
		if c != ' ' && c != '\t' {
			atStart = false
		}
		i++
	}
	return b.String()
}

// closingRun finds where the code span opened by a run of `length` backticks ends:
// the next run of exactly that many. -1 when it never closes, and then the opening
// backticks are just characters.
func closingRun(line string, from, length int) int {
	for i := from; i < len(line); {
		if line[i] != '`' {
			i++
			continue
		}
		run := 0
		for i+run < len(line) && line[i+run] == '`' {
			run++
		}
		if run == length {
			return i + run
		}
		i += run
	}
	return -1
}
