package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/strelov1/freehire/internal/llm"
)

// maxInputRunes caps the post text sent to the model. Channel posts are short; this
// bounds a hostile/oversized post so it cannot amplify per-call token cost.
const maxInputRunes = 24000

// LangChainExtractor implements Extractor over any OpenAI-compatible endpoint via
// the shared llm client. The model is asked for a JSON object matching the
// Extraction contract.
type LangChainExtractor struct {
	client *llm.Client
}

// NewLangChainExtractor wraps a prebuilt llm.Client (constructed once via
// llm.NewClient, which also wires optional tracing) as a telegram Extractor.
func NewLangChainExtractor(c *llm.Client) *LangChainExtractor {
	return &LangChainExtractor{client: c}
}

// Extract asks the model to classify the post and extract its vacancies. It does
// not validate the result — the runner validates before persisting.
func (e *LangChainExtractor) Extract(ctx context.Context, text string, kind Kind) (Extraction, error) {
	raw, err := e.client.GenerateJSON(ctx, extractSystemPrompt(kind), llm.TruncateRunes(text, maxInputRunes))
	if err != nil {
		return Extraction{}, fmt.Errorf("telegram: %w", err)
	}
	return parseExtraction(raw)
}

// parseExtraction unmarshals the model's already-fence-stripped JSON response,
// first repairing raw control characters some models emit inside string literals.
func parseExtraction(raw string) (Extraction, error) {
	s := repairJSONControlChars(strings.TrimSpace(raw))
	var ex Extraction
	if err := json.Unmarshal([]byte(s), &ex); err != nil {
		return Extraction{}, fmt.Errorf("telegram: parse response: %w", err)
	}
	return ex, nil
}

// repairJSONControlChars escapes raw control characters that appear inside JSON
// string literals. Some models, even in JSON mode, emit literal newlines or tabs
// inside string values — multi-line job descriptions are the common case — which
// violates the JSON spec and makes the strict decoder reject the whole payload.
// This rewrites those raw control runes into their valid escape sequences,
// leaving the JSON structure and already-escaped sequences untouched.
func repairJSONControlChars(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inString, escaped := false, false
	for _, r := range s {
		switch {
		case escaped:
			b.WriteRune(r)
			escaped = false
		case r == '\\' && inString:
			b.WriteRune(r)
			escaped = true
		case r == '"':
			b.WriteRune(r)
			inString = !inString
		case inString && r < 0x20:
			switch r {
			case '\n':
				b.WriteString(`\n`)
			case '\t':
				b.WriteString(`\t`)
			case '\r':
				b.WriteString(`\r`)
			default:
				fmt.Fprintf(&b, `\u%04x`, r)
			}
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// extractSystemPrompt instructs the model to classify the post and extract each
// vacancy. The channel kind steers expectations: a board post is one vacancy; an
// authored post may bundle several (or none — channels mix in ads and digests).
func extractSystemPrompt(kind Kind) string {
	var b strings.Builder
	b.WriteString("You read a Telegram channel post (Russian or English) and decide whether it advertises job vacancies.\n")
	b.WriteString("Return ONLY a JSON object: {\"jobs\": [...]}. If the post is not a job advertisement ")
	b.WriteString("(news, digest, course ad, meme), return {\"jobs\": []}.\n\n")

	switch kind {
	case KindAuthored:
		b.WriteString("This channel posts editorial stories that may describe SEVERAL distinct roles at one company ")
		b.WriteString("in a single post. Extract each distinct role as its own job.\n\n")
	default:
		b.WriteString("This channel is a job board: a post normally describes exactly one vacancy.\n\n")
	}

	b.WriteString("Each job object has these keys:\n")
	b.WriteString("- title (required): the role title, e.g. \"Senior Go Engineer\".\n")
	b.WriteString("- company: the hiring company named in the post; omit if not stated. Never use the channel name.\n")
	b.WriteString("- location: city/country if stated, e.g. \"London\".\n")
	b.WriteString("- remote (boolean): true only if the post says the role is remote.\n")
	b.WriteString("- description (required): the post's text relevant to THIS role — responsibilities, requirements, ")
	b.WriteString("salary, benefits, and how to apply (contacts, links). Plain text, keep the original language. ")
	b.WriteString("Preserve the original line structure: keep bullet points, numbered lists, and paragraphs on ")
	b.WriteString("separate lines (use \\n; a blank line between paragraphs). Do NOT collapse the post into one ")
	b.WriteString("line. Do not invent anything that is not in the post.\n")
	return b.String()
}
