package sources

import (
	"html"
	"strings"
	"unicode"
	"unicode/utf8"
)

// plainTextBulletMarkers are the line-leading glyphs ATS company editors use to mark list
// items in an otherwise plain-text description (bullet, middle dot, triangular/hollow/filled
// bullets, hyphen, asterisk, en/em dash). A marker counts only when it leads a line and is
// followed by whitespace, so a mid-sentence dash, a hyphenated word, or a "*Note" footnote
// prefix stays prose.
var plainTextBulletMarkers = map[rune]bool{
	'•': true, '·': true, '‣': true, '◦': true, '▪': true,
	'-': true, '*': true, '–': true, '—': true,
}

// plainTextBullet reports whether a trimmed line is a bullet, returning its text with the
// leading marker (and the space after it) removed.
func plainTextBullet(line string) (string, bool) {
	r, size := utf8.DecodeRuneInString(line)
	if !plainTextBulletMarkers[r] {
		return "", false
	}
	next, _ := utf8.DecodeRuneInString(line[size:])
	if !unicode.IsSpace(next) {
		return "", false // "-word" / "*Note" without a following space is prose, not a bullet
	}
	return strings.TrimSpace(line[size:]), true
}

// plainTextToHTML rebuilds the structural HTML the {@html} consumer renders from a source's
// plain-text description. Such feeds carry the body as newline-delimited text — blank lines
// separate blocks, assorted leading glyphs mark bullets — with no markup, so rendered as-is
// every newline collapses into one unbroken wall of text. This reconstructs it: a run of
// bullet lines becomes a <ul>, a run of other text lines becomes a <p> (wrapped lines joined
// by <br>), and a blank line closes the open block. Text is HTML-escaped because it is literal
// prose, not markup. Callers still pass the result through sanitizeHTML.
func plainTextToHTML(text string) string {
	var out strings.Builder
	var para, bullets []string
	flushPara := func() {
		if len(para) > 0 {
			out.WriteString("<p>" + strings.Join(para, "<br>") + "</p>")
			para = para[:0]
		}
	}
	flushBullets := func() {
		if len(bullets) > 0 {
			out.WriteString("<ul>")
			for _, li := range bullets {
				out.WriteString("<li>" + li + "</li>")
			}
			out.WriteString("</ul>")
			bullets = bullets[:0]
		}
	}
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			flushBullets()
			flushPara()
			continue
		}
		if item, ok := plainTextBullet(line); ok {
			flushPara()
			bullets = append(bullets, html.EscapeString(item))
			continue
		}
		flushBullets()
		para = append(para, html.EscapeString(line))
	}
	flushBullets()
	flushPara()
	return out.String()
}
