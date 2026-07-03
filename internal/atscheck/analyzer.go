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

// Analyze asks the model, over the CV text, for a content-quality score (0-100)
// and a few concrete improvement suggestions. Returns (nil, nil) when unconfigured
// so callers degrade. The model is untrusted output — the score is clamped and
// suggestions are trimmed, length-bounded, and capped.
func (a *Analyzer) Analyze(ctx context.Context, cvText string) (*Review, error) {
	if a == nil || a.client == nil {
		return nil, nil
	}
	raw, err := a.client.GenerateJSON(ctx, reviewSystemPrompt(), reviewUserPrompt(cvText))
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
	b.WriteString("You are a senior technical recruiter reviewing the plain text of a candidate's CV. ")
	b.WriteString("Return ONLY a JSON object.\n\n")
	b.WriteString("Return exactly these keys:\n")
	b.WriteString("- \"content_quality\": integer 0-100. How strong the writing is for a human recruiter: ")
	b.WriteString("action verbs over passive phrasing, quantified achievements over responsibility lists, ")
	b.WriteString("and clean readable structure. Penalise garbled/interleaved text (a sign of a multi-column ")
	b.WriteString("or table layout an ATS may scramble). 100 = excellent; low = weak/garbled.\n")
	b.WriteString("- \"suggestions\": an array of 3 to 6 short, concrete, actionable improvement sentences ")
	b.WriteString("(e.g. replace a weak verb, quantify a bullet, fix a section that reads as garbled, ")
	b.WriteString("flag a date inconsistency). Each ≤ 200 characters. Base every judgement only on the CV ")
	b.WriteString("text provided; do not invent facts.\n")
	return b.String()
}

// reviewUserPrompt carries the (bounded) CV text.
func reviewUserPrompt(cvText string) string {
	return "CV:\n" + llm.TruncateRunes(cvText, maxCVRunes) + "\n"
}
