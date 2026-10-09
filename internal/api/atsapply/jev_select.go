package atsapply

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/candidate/experience"
)

// No external Jev client here either, and deliberately not shared with
// internal/ai/autofillagent/jev.go — see design.md's "No shared Jev client" decision:
// atsapply (api block) and autofillagent (ai block) sit at different layers in
// internal/platform/arch/layering, and the two calls already carry different
// instructions.
const (
	jevSelectBaseURL = "https://api.typesafe.ai"
	jevSelectTimeout = 15 * time.Second
	// jevSelectModel is required by the API — a request without it is rejected with 422,
	// confirmed against the live endpoint.
	jevSelectModel = "jev-latest"
	// jevSelectAnswerKey names the single question this file ever asks Jev.
	jevSelectAnswerKey = "answer"
	// jevSelectDeclineKey covers two reasons at once — nothing stated supports any
	// option, or the question falls in a categorically excluded topic — because the
	// caller only ever needs "answered or not", never which reason applied. Its wording
	// cannot collide with a real option label for the same reason jevDeclineKey
	// (autofillagent/jev.go) cannot.
	jevSelectDeclineKey = "none of these / not stated"
)

// jevSelectInstructions mirrors draftSystemPrompt's rules (llm_drafter.go), narrowed to a
// single select-kind question. The categorical exclusion travels with every call — see
// design.md: a select field reaching this drafter already passed draftable's sensitive/
// geography label gate, so this is a second, independent safeguard against a label
// heuristic missing a real compensation/eligibility question.
func jevSelectInstructions(label string) string {
	return fmt.Sprintf(`You answer ONE job-application question on behalf of a candidate, by
picking the single option the candidate's own stated facts support.

Rules, all absolute:
- Use ONLY the facts listed under "What the candidate has stated". Never invent a fact.
- If nothing given lets you answer honestly, pick %q — this is a correct, expected
  outcome, not a failure.
- Never answer with an assumption about identity, demographics, compensation, or legal
  work status even if a stated fact seems to suggest one — treat every question in those
  categories as ungroundable regardless of what else is stated, and pick %q.

Question: %s`, jevSelectDeclineKey, jevSelectDeclineKey, label)
}

type jevSelectChooser struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
}

type jevSelectChooserOption func(*jevSelectChooser)

func withJevSelectBaseURL(url string) jevSelectChooserOption {
	return func(c *jevSelectChooser) { c.baseURL = url }
}

func newJevSelectChooser(apiKey string, opts ...jevSelectChooserOption) *jevSelectChooser {
	c := &jevSelectChooser{
		httpClient: &http.Client{Timeout: jevSelectTimeout},
		baseURL:    jevSelectBaseURL,
		apiKey:     apiKey,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

type jevSelectRequest struct {
	State     any                          `json:"state"`
	Model     string                       `json:"model"`
	Questions map[string]jevSelectQuestion `json:"questions"`
}

type jevSelectQuestion struct {
	Type         string            `json:"type"`
	Instructions string            `json:"instructions"`
	Criteria     map[string]string `json:"criteria"`
}

type jevSelectResponse struct {
	Answers map[string]struct {
		Choice string `json:"choice"`
	} `json:"answers"`
}

// Choose asks Jev to pick one of options (option labels) for question, grounded in facts.
// It returns ok=false for the decline sentinel (or a blank choice); any other answer is
// returned as ok=true even if it names no offered option — matchOption re-validates the
// result against the field's own offered options downstream, so a hallucinated label is
// caught there, not here.
func (j *jevSelectChooser) Choose(ctx context.Context, question MergedField, options []string, facts []experience.Atom) (string, bool, error) {
	criteria := make(map[string]string, len(options)+1)
	for _, opt := range options {
		criteria[opt] = opt
	}
	criteria[jevSelectDeclineKey] = "nothing stated supports any option, or this question is a categorically excluded topic"

	claims := make([]string, 0, len(facts))
	for _, a := range facts {
		claim := a.Claim
		if a.Context != "" {
			claim = claim + " (" + a.Context + ")"
		}
		claims = append(claims, claim)
	}

	body, err := json.Marshal(jevSelectRequest{
		State: map[string]any{"facts": claims},
		Model: jevSelectModel,
		Questions: map[string]jevSelectQuestion{
			jevSelectAnswerKey: {Type: "choice", Instructions: jevSelectInstructions(question.Label), Criteria: criteria},
		},
	})
	if err != nil {
		return "", false, fmt.Errorf("atsapply: jev select: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, j.baseURL+"/v1/systemone", bytes.NewReader(body))
	if err != nil {
		return "", false, fmt.Errorf("atsapply: jev select: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+j.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := j.httpClient.Do(req)
	if err != nil {
		return "", false, fmt.Errorf("atsapply: jev select: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body)
		return "", false, fmt.Errorf("atsapply: jev select: unexpected status %d", resp.StatusCode)
	}

	var parsed jevSelectResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", false, fmt.Errorf("atsapply: jev select: decode response: %w", err)
	}
	answer, ok := parsed.Answers[jevSelectAnswerKey]
	if !ok {
		return "", false, fmt.Errorf("atsapply: jev select: response carried no %q answer", jevSelectAnswerKey)
	}
	if answer.Choice == jevSelectDeclineKey || strings.TrimSpace(answer.Choice) == "" {
		return "", false, nil
	}
	return answer.Choice, true, nil
}
