package pii

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// Contacts are the caller's authoritative contact values (e.g. from a structured résumé).
// They are always maskable even when they do not appear verbatim in the CV text Build reads,
// so the same Redactor masks them wherever they surface (raw CV and structured JSON alike).
type Contacts struct {
	FullName string
	Email    string
	Phone    string
	Links    []string
}

// Redactor masks a fixed set of detected PII values into stable numbered placeholders and
// restores them. Build it once per CV; reuse it for every text that flows to the LLM.
type Redactor struct {
	reps []replacement // longest value first, so a value contained in another masks first
}

type replacement struct {
	value       string
	placeholder string
	re          *regexp.Regexp // non-nil ⇒ word-boundary match (bounds over-redaction)
}

// Build detects PII in text (regex floor ∪ model spans ∪ known contacts) and returns a
// Redactor. It is fail-closed: a nil detector or a detector error returns an error rather
// than a partial (regex-only) redactor, so callers can refuse to send the CV to the LLM.
func Build(ctx context.Context, text string, known Contacts, d Detector) (*Redactor, error) {
	if d == nil {
		return nil, errors.New("pii: detector not configured")
	}
	modelSpans, err := d.Detect(ctx, text)
	if err != nil {
		return nil, fmt.Errorf("pii: detect: %w", err)
	}

	spans := append(regexSpans(text), sanitizeSpans(modelSpans, len(text))...)
	sort.SliceStable(spans, func(i, j int) bool { return spans[i].Start < spans[j].Start })

	type valueKind struct{ value, kind string }
	var vals []valueKind
	seen := make(map[string]bool)
	add := func(v, kind string) {
		if v = strings.TrimSpace(v); v == "" || seen[v] {
			return
		}
		seen[v] = true
		vals = append(vals, valueKind{v, kind})
	}
	for _, s := range spans {
		add(text[s.Start:s.End], s.Kind)
	}
	add(known.FullName, KindName)
	add(known.Email, KindEmail)
	add(known.Phone, KindPhone)
	for _, l := range known.Links {
		add(l, KindLink)
	}

	counts := make(map[string]int)
	reps := make([]replacement, 0, len(vals))
	for _, vk := range vals {
		counts[vk.kind]++
		rep := replacement{
			value:       vk.value,
			placeholder: fmt.Sprintf("[REDACTED_%s_%d]", vk.kind, counts[vk.kind]),
		}
		if wordish(vk.value) {
			rep.re = regexp.MustCompile(`\b` + regexp.QuoteMeta(vk.value) + `\b`)
		}
		reps = append(reps, rep)
	}
	sort.SliceStable(reps, func(i, j int) bool { return len(reps[i].value) > len(reps[j].value) })
	return &Redactor{reps: reps}, nil
}

// Redact replaces every detected PII value in text with its placeholder. A nil Redactor is
// a no-op (callers that fail closed never reach Redact with nil).
func (r *Redactor) Redact(text string) string {
	if r == nil {
		return text
	}
	for _, rep := range r.reps {
		if rep.re != nil {
			text = rep.re.ReplaceAllString(text, rep.placeholder)
		} else {
			text = strings.ReplaceAll(text, rep.value, rep.placeholder)
		}
	}
	return text
}

// Restore maps every placeholder back to its original value. Placeholders are unique, so
// restore order is irrelevant.
func (r *Redactor) Restore(text string) string {
	if r == nil {
		return text
	}
	for _, rep := range r.reps {
		text = strings.ReplaceAll(text, rep.placeholder, rep.value)
	}
	return text
}

// wordish reports whether v starts and ends with a word char, so word-boundary matching is
// safe and desirable (a name/word value should not mask a substring of a larger word).
func wordish(v string) bool {
	return v != "" && isWord(v[0]) && isWord(v[len(v)-1])
}

func isWord(c byte) bool {
	return c == '_' || (c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

// sanitizeSpans drops model spans with out-of-range or inverted offsets.
func sanitizeSpans(spans []Span, n int) []Span {
	out := spans[:0:0]
	for _, s := range spans {
		if s.Start >= 0 && s.End <= n && s.Start < s.End {
			out = append(out, s)
		}
	}
	return out
}
