package verdict

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/strelov1/freehire/internal/llm"
)

const (
	// maxResumeRunes bounds the résumé text sent to the model. The text is
	// user-supplied, so capping it keeps one huge upload from amplifying token cost
	// (mirrors enrich's maxDescriptionRunes).
	maxResumeRunes = 24000

	// maxAdviceRunes bounds each advice string, so a verbose model can't blow up the
	// stored/served payload.
	maxAdviceRunes = 280
)

// Analyzer runs the single LLM call that scores résumé coherence and drafts gap
// advice. A nil client (LLM unconfigured) makes Analyze a no-op so the caller
// degrades to the deterministic verdict.
type Analyzer struct {
	client *llm.Client
}

// NewAnalyzer wraps an llm.Client. client may be nil (LLM unconfigured).
func NewAnalyzer(client *llm.Client) *Analyzer {
	return &Analyzer{client: client}
}

// Analysis is the LLM's structured answer: a coherence score and advice keyed by
// skill slug.
type Analysis struct {
	Coherence int               `json:"coherence"`
	Advice    map[string]string `json:"advice"`
}

// Analyze asks the model, over the résumé text, for a coherence score (0-100) and
// short advice for each of the given must-have gap slugs. It clamps the score to
// 0-100, drops advice for any slug not in gaps, and truncates advice length.
// Returns (nil, nil) when the analyzer is unconfigured so callers can degrade.
func (a *Analyzer) Analyze(ctx context.Context, resume string, gaps []string) (*Analysis, error) {
	if a == nil || a.client == nil {
		return nil, nil
	}

	raw, err := a.client.GenerateJSON(ctx, buildCoherencePrompt(), userPrompt(resume, gaps))
	if err != nil {
		return nil, fmt.Errorf("verdict: analyze: %w", err)
	}

	var out Analysis
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &out); err != nil {
		return nil, fmt.Errorf("verdict: parse analysis: %w", err)
	}

	out.sanitize(gaps)
	return &out, nil
}

// sanitize clamps the coherence score and keeps only advice for the requested
// gap slugs, each length-bounded. Defensive: the model is untrusted output.
func (an *Analysis) sanitize(gaps []string) {
	an.Coherence = clamp(an.Coherence, 0, 100)

	allowed := make(map[string]bool, len(gaps))
	for _, g := range gaps {
		allowed[g] = true
	}
	cleaned := make(map[string]string, len(an.Advice))
	for slug, text := range an.Advice {
		text = strings.TrimSpace(text)
		if text == "" || !allowed[slug] {
			continue
		}
		cleaned[slug] = llm.TruncateRunes(text, maxAdviceRunes)
	}
	an.Advice = cleaned
}

func clamp(n, lo, hi int) int {
	if n < lo {
		return lo
	}
	if n > hi {
		return hi
	}
	return n
}

// buildCoherencePrompt is the system prompt pinning the JSON contract. Kept as a
// function (not a const) to mirror enrich's testable buildSystemPrompt.
func buildCoherencePrompt() string {
	var b strings.Builder
	b.WriteString("You are a technical résumé reviewer. You receive the plain text of a candidate's ")
	b.WriteString("résumé and a list of target skill slugs. Return ONLY a JSON object.\n\n")
	b.WriteString("Return exactly these keys:\n")
	b.WriteString("- \"coherence\": integer 0-100. How well the skills the candidate CLAIMS (in a ")
	b.WriteString("Skills/Technologies section) are actually substantiated by concrete evidence in ")
	b.WriteString("their Experience/Projects (real usage, outcomes, numbers). 100 = every claimed ")
	b.WriteString("skill is backed by experience; low = buzzword stuffing with no backing.\n")
	b.WriteString("- \"advice\": an object mapping each target slug (use the EXACT slug given) to ONE ")
	b.WriteString("short sentence of concrete, actionable advice on how to close that gap and evidence ")
	b.WriteString("it in Experience. Include a key only for a slug you were given; omit slugs you have ")
	b.WriteString("no useful advice for.\n\n")
	b.WriteString("Do not invent skills. Do not include any key other than \"coherence\" and \"advice\". ")
	b.WriteString("Base every judgement only on the résumé text provided.\n")
	return b.String()
}

// userPrompt carries the target gap slugs and the (bounded) résumé text.
func userPrompt(resume string, gaps []string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Target skills to advise on (use these exact slugs as keys): %s\n\n", strings.Join(gaps, ", "))
	fmt.Fprintf(&b, "Résumé:\n%s\n", llm.TruncateRunes(resume, maxResumeRunes))
	return b.String()
}
