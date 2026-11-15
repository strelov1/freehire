package search

import (
	"strings"
	"unicode/utf8"
)

// chunkBudgetRunes bounds one chunk's runes before the "passage: ..." prefix is added.
// A heuristic, not a token count: ~2000 runes stays under e5's 512-token limit even for
// dense scripts; TEI's --auto-truncate catches a slight overshoot.
const chunkBudgetRunes = 2000

// chunkText splits stripToPlainText's output into chunks of at most chunkBudgetRunes, so
// a long description becomes several vectors instead of being truncated by TEI. It breaks
// between paragraphs, and on a word boundary only inside a single over-budget paragraph.
func chunkText(text string) []string {
	var chunks []string
	var cur strings.Builder
	curLen := 0
	flush := func() {
		if cur.Len() > 0 {
			chunks = append(chunks, strings.TrimSpace(cur.String()))
			cur.Reset()
			curLen = 0
		}
	}
	for _, para := range strings.Split(text, "\n") {
		para = strings.TrimSpace(para)
		if para == "" {
			continue
		}
		paraLen := utf8.RuneCountInString(para)
		if paraLen > chunkBudgetRunes {
			// Over budget on its own: flush the pending chunk, then split on words.
			flush()
			chunks = append(chunks, wrapWords(para, chunkBudgetRunes)...)
			continue
		}
		if curLen > 0 && curLen+1+paraLen > chunkBudgetRunes {
			flush()
		}
		if curLen > 0 {
			cur.WriteString("\n")
			curLen++
		}
		cur.WriteString(para)
		curLen += paraLen
	}
	flush()
	return chunks
}

// wrapWords greedily packs words into lines of at most budget runes. A single word over
// budget (a long URL, say) is split by rune count so it cannot make an unbounded chunk.
func wrapWords(s string, budget int) []string {
	var lines []string
	var cur strings.Builder
	curLen := 0
	for _, w := range strings.Fields(s) {
		wLen := utf8.RuneCountInString(w)
		if wLen > budget {
			if curLen > 0 {
				lines = append(lines, cur.String())
				cur.Reset()
				curLen = 0
			}
			runes := []rune(w)
			for len(runes) > budget {
				lines = append(lines, string(runes[:budget]))
				runes = runes[budget:]
			}
			if len(runes) > 0 {
				cur.WriteString(string(runes))
				curLen = len(runes)
			}
			continue
		}
		if curLen > 0 && curLen+1+wLen > budget {
			lines = append(lines, cur.String())
			cur.Reset()
			curLen = 0
		}
		if curLen > 0 {
			cur.WriteString(" ")
			curLen++
		}
		cur.WriteString(w)
		curLen += wLen
	}
	if cur.Len() > 0 {
		lines = append(lines, cur.String())
	}
	return lines
}
