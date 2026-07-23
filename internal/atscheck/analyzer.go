package atscheck

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/strelov1/freehire/internal/llm"
)

const (
	// maxCVRunes bounds the CV text sent to the model (user-supplied → cap token cost).
	maxCVRunes = 24000
	// maxSuggestionRunes / maxSuggestions bound the model's advice so a verbose answer
	// can't blow up the stored/served payload.
	maxSuggestionRunes = 240
	maxSuggestions     = 6
)

// Analyzer runs the optional LLM qualitative review of a CV. A nil client (LLM
// unconfigured) makes Analyze a no-op so the caller degrades to the deterministic
// score.
type Analyzer struct {
	client *llm.Client
}

// NewAnalyzer wraps an llm.Client. client may be nil (LLM unconfigured).
func NewAnalyzer(client *llm.Client) *Analyzer {
	return &Analyzer{client: client}
}

// Review is the LLM's qualitative answer: a content-quality score and short,
// actionable suggestions. JSON is the wire contract (generated to TS + persisted).
type Review struct {
	ContentQuality int      `json:"content_quality"`
	Suggestions    []string `json:"suggestions"`
}

// Analyze asks the model, over the de-identified structured résumé, for a content-quality
// score (0-100) and a few concrete improvement suggestions. It NEVER sends the raw CV, and
// strips the contact fields from the structure first, so no direct identifier reaches the
// model. Returns (nil, nil) when unconfigured or when there is no usable structured résumé,
// so callers degrade to the deterministic score. The model is untrusted output — the score
// is clamped and suggestions are trimmed, length-bounded, and capped.
func (a *Analyzer) Analyze(ctx context.Context, structuredJSON string) (*Review, error) {
	if a == nil || a.client == nil {
		return nil, nil
	}
	candidate := stripContacts(structuredJSON)
	if candidate == "" {
		return nil, nil
	}
	raw, err := a.client.GenerateJSON(ctx, reviewSystemPrompt(), reviewUserPrompt(candidate))
	if err != nil {
		return nil, fmt.Errorf("atscheck: analyze: %w", err)
	}
	var out Review
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &out); err != nil {
		return nil, fmt.Errorf("atscheck: parse review: %w", err)
	}
	out.sanitize()
	return &out, nil
}

// sanitize clamps the score and trims/bounds/caps the suggestions.
func (r *Review) sanitize() {
	r.ContentQuality = clamp(r.ContentQuality)
	cleaned := make([]string, 0, len(r.Suggestions))
	for _, s := range r.Suggestions {
		if s = strings.TrimSpace(s); s == "" {
			continue
		}
		cleaned = append(cleaned, llm.TruncateRunes(s, maxSuggestionRunes))
		if len(cleaned) >= maxSuggestions {
			break
		}
	}
	r.Suggestions = cleaned
}

// reviewSystemPrompt pins the JSON contract. Kept as a function (mirrors enrich's
// testable buildSystemPrompt).
func reviewSystemPrompt() string {
	var b strings.Builder
	b.WriteString("You are a senior technical recruiter reviewing a candidate's structured résumé ")
	b.WriteString("(JSON — experience highlights, summary, skills). Return ONLY a JSON object.\n\n")
	b.WriteString("Return exactly these keys:\n")
	b.WriteString("- \"content_quality\": integer 0-100. How strong the writing is for a human recruiter: ")
	b.WriteString("action verbs over passive phrasing, quantified achievements over responsibility lists. ")
	b.WriteString("100 = excellent; low = weak.\n")
	b.WriteString("- \"suggestions\": an array of 3 to 6 short, concrete, actionable improvement sentences ")
	b.WriteString("(e.g. replace a weak verb, quantify a bullet, flag a date inconsistency). Each ≤ 200 ")
	b.WriteString("characters. Base every judgement only on the résumé provided; do not invent facts.\n")
	return b.String()
}

// reviewUserPrompt carries the (bounded) de-identified structured résumé.
func reviewUserPrompt(structured string) string {
	return "Résumé (structured, JSON):\n" + llm.TruncateRunes(structured, maxCVRunes) + "\n"
}

// stripContacts drops the contact fields from the structured-résumé JSON, returning the
// de-identified remainder. Empty on empty or unparseable input (the caller then serves the
// deterministic score only), so no raw/contact identifier can reach the model.
func stripContacts(structuredJSON string) string {
	s := strings.TrimSpace(structuredJSON)
	if s == "" {
		return ""
	}
	var m map[string]json.RawMessage
	if json.Unmarshal([]byte(s), &m) != nil {
		return ""
	}
	for _, k := range []string{"full_name", "email", "phone", "links"} {
		delete(m, k)
	}
	b, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(b)
}
