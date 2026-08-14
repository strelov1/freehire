package search

import (
	"strings"
	"unicode/utf8"
)

// chunkBudgetRunes bounds how many runes go in one chunk before jobPassages' "passage:
// {title} at {company}. " prefix is added. It is a conservative heuristic, not an exact
// token count: e5's tokenizer averages ~1.3-1.5 tokens per English word, so ~2000 runes
// (~350 words) leaves comfortable headroom under the model's 512-token limit even for
// denser scripts, without per-language tuning (design.md Decision 2). TEI's own
// --auto-truncate remains the safety net for a chunk that slightly overshoots.
const chunkBudgetRunes = 2000

// chunkText splits stripToPlainText's output into pieces no chunk grossly exceeds
// chunkBudgetRunes, so the full description reaches the embedding model as multiple
// vectors instead of being silently truncated by TEI past its token limit. It prefers
// paragraph boundaries (the newlines stripToPlainText inserts at block-element edges) —
// a chunk boundary lands between paragraphs whenever the text allows it, and only falls
// back to a word boundary within a single over-budget paragraph.
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
			// A single paragraph past budget on its own can't be kept whole — split it on
			// word boundaries instead, after flushing whatever chunk was building so far.
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

// wrapWords greedily packs whitespace-delimited words into lines no longer than budget
// runes, never splitting a word across two lines — except a single word that itself
// exceeds budget (e.g. a long URL or token with no whitespace to break on), which is
// split by rune count so it cannot produce an unbounded chunk on its own.
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
