package pii

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

// Contacts are the caller's authoritative contact values (e.g. from a structured résumé).
// They are always maskable even when they do not appear verbatim in the CV text Build reads,
// so the same Redactor masks them wherever they surface (raw CV and structured JSON alike).
type Contacts struct {
	FullName string
	Email    string
	Phone    string
	Location string
	Links    []string
}

// Redactor masks a fixed set of detected PII values into stable numbered placeholders and
// restores them. Build it once per CV; reuse it for every text that flows to the LLM.
type Redactor struct {
	reps     []replacement // longest value first, so a value contained in another masks first
	contacts Contacts      // contact values recovered from the detected spans
}

// Contacts returns the contact values recovered from the detected spans (first name/email/
// phone/location, all links). It lets a caller — e.g. resumeextract — fill contact fields
// from deterministic detection instead of the LLM, which only ever sees the redacted CV.
func (r *Redactor) Contacts() Contacts {
	if r == nil {
		return Contacts{}
	}
	return r.contacts
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
	// boundarySafe[value] is true only when EVERY detected occurrence of value sits between
	// non-word chars, so a \b anchor masks each one. A detected span that abuts a word char
	// (e.g. an email touching a trailing digit, or a NAME span inside a larger token) makes
	// the value unsafe for \b — it is then masked plainly so the occurrence can never leak.
	// Its key set is exactly the detected values, so the fail-closed self-check ranges it too.
	boundarySafe := make(map[string]bool)
	var found Contacts
	for _, s := range spans {
		v := strings.TrimSpace(text[s.Start:s.End])
		if v == "" {
			continue
		}
		add(v, s.Kind)
		fillContact(&found, s.Kind, v)
		ok := (s.Start == 0 || !isWord(text[s.Start-1])) && (s.End == len(text) || !isWord(text[s.End]))
		if _, seen := boundarySafe[v]; seen {
			boundarySafe[v] = boundarySafe[v] && ok
		} else {
			boundarySafe[v] = ok
		}
	}
	// A known value never passes through the spans loop above, so boundarySafe has no
	// entry for it unless the identical value also happened to be detected. Without one,
	// wordish(v) && wordyKind[kind] && boundarySafe[v] is always false for it, so a known
	// NAME/ADDRESS falls back to plain substring replacement even when every literal
	// occurrence in text is boundary-complete — the over-redaction the \b-anchored path
	// exists to avoid. Scan text directly for the value and apply the same completeness
	// rule the spans loop uses, so a known contact gets the identical boundary-aware
	// treatment a detected one does. A value absent from text is left out of boundarySafe
	// entirely (not even set to false): both replacement strategies are a no-op for it, and
	// adding it would make the fail-closed self-check below spuriously fail on a value that
	// was never there to redact — "known contacts are always maskable" holds regardless.
	addKnown := func(v, kind string) {
		v = strings.TrimSpace(v)
		if v == "" {
			return
		}
		add(v, kind)
		if _, ok := boundarySafe[v]; !ok && strings.Contains(text, v) {
			boundarySafe[v] = valueBoundarySafe(text, v)
		}
	}
	addKnown(known.FullName, KindName)
	addKnown(known.Email, KindEmail)
	addKnown(known.Phone, KindPhone)
	addKnown(known.Location, KindAddress)
	for _, l := range known.Links {
		addKnown(l, KindLink)
	}

	counts := make(map[string]int)
	reps := make([]replacement, 0, len(vals))
	for _, vk := range vals {
		counts[vk.kind]++
		rep := replacement{
			value:       vk.value,
			placeholder: fmt.Sprintf("[REDACTED_%s_%d]", vk.kind, counts[vk.kind]),
		}
		// Word-boundary only for the "wordy" kinds AND only when every detected occurrence
		// is boundary-complete; everything else is masked plainly (leak-proof). Specific
		// values (email/phone/link) are always plain — they never occur inside a real word.
		if wordish(vk.value) && wordyKind[vk.kind] && boundarySafe[vk.value] {
			rep.re = regexp.MustCompile(`\b` + regexp.QuoteMeta(vk.value) + `\b`)
		}
		reps = append(reps, rep)
	}
	sort.SliceStable(reps, func(i, j int) bool { return len(reps[i].value) > len(reps[j].value) })
	r := &Redactor{reps: reps, contacts: found}

	// Fail-closed self-check: masking MUST remove every detected value from the source.
	// If any survives (a boundary quirk we did not foresee), refuse rather than leak.
	redacted := r.Redact(text)
	for v := range boundarySafe {
		if strings.Count(redacted, v) >= strings.Count(text, v) {
			return nil, fmt.Errorf("pii: redaction left detected value unmasked")
		}
	}
	return r, nil
}

// wordyKind marks the kinds whose values can legitimately be a substring of a normal word
// (a name, an address), so word-boundary matching is worth attempting to avoid over-redaction.
var wordyKind = map[string]bool{KindName: true, KindAddress: true}

// fillContact records a detected value into c: the first plausible name/email/phone/location
// wins, and each distinct clean link is collected. Called only for detected spans, so c
// reflects the CV, not known input. It is defensive because the model mis-tags handles/slugs
// as a person and its URL spans sometimes swallow neighbouring text — the redactor still
// masks every span, but only well-formed values become the caller's stored contact fields.
func fillContact(c *Contacts, kind, v string) {
	switch kind {
	case KindName:
		if c.FullName == "" && isPlausibleName(v) {
			c.FullName = v
		}
	case KindEmail:
		if c.Email == "" {
			c.Email = v
		}
	case KindPhone:
		if c.Phone == "" {
			c.Phone = v
		}
	case KindAddress:
		// Residence text is messy ("Lisbon, PT"); accept the first non-empty ADDRESS span.
		if c.Location == "" && strings.TrimSpace(v) != "" {
			c.Location = strings.TrimSpace(v)
		}
	case KindLink:
		if isCleanLink(v) && !containsString(c.Links, v) {
			c.Links = append(c.Links, v)
		}
	}
}

// isPlausibleName reports whether v reads like a real full name: at least two
// whitespace-separated tokens of letters (and name punctuation), and none of the @ / :
// characters that mark a handle, slug, or URL the model sometimes classifies as a person.
func isPlausibleName(v string) bool {
	if strings.ContainsAny(v, "@/:") {
		return false
	}
	fields := strings.Fields(v)
	if len(fields) < 2 {
		return false
	}
	for _, f := range fields {
		for _, r := range f {
			if !unicode.IsLetter(r) && r != '-' && r != '.' && r != '\'' {
				return false
			}
		}
	}
	return true
}

// isCleanLink rejects a link/handle carrying internal whitespace — the sign of a model span
// that grabbed surrounding text (a well-formed URL or @handle has none).
func isCleanLink(v string) bool {
	return v != "" && !strings.ContainsAny(v, " \t\n\r")
}

func containsString(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
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

// valueBoundarySafe reports whether every literal occurrence of v in text sits between
// non-word characters — the same completeness check the spans loop applies per detected
// span, but by scanning text directly for v instead of relying on a supplied span. Callers
// must only invoke it when v is known to occur in text at least once.
func valueBoundarySafe(text, v string) bool {
	safe := true
	for start := 0; ; {
		i := strings.Index(text[start:], v)
		if i < 0 {
			return safe
		}
		s, e := start+i, start+i+len(v)
		if (s != 0 && isWord(text[s-1])) || (e != len(text) && isWord(text[e])) {
			safe = false
		}
		start = e
	}
}

// sanitizeSpans drops model spans with out-of-range or inverted offsets.
func sanitizeSpans(spans []Span, n int) []Span {
	var out []Span
	for _, s := range spans {
		if s.Start >= 0 && s.End <= n && s.Start < s.End {
			out = append(out, s)
		}
	}
	return out
}
