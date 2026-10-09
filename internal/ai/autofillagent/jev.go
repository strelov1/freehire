package autofillagent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// jevDeclineKey is offered to Jev alongside the page's real options, standing in for
// Choose's own "decline" outcome. Its wording cannot collide with a real form option —
// no ATS renders a combobox choice phrased as a description of itself declining to answer.
const jevDeclineKey = "none of these / not stated"

// No external Jev client here by design — see
// openspec/changes/autofill-choose-jev-provider/design.md for why.
const (
	jevDefaultBaseURL = "https://api.typesafe.ai"
	jevDefaultTimeout = 15 * time.Second
	// jevModel is required by the API — a request without it is rejected with 422,
	// confirmed against the live endpoint.
	jevModel = "jev-latest"
	// jevAnswerKey names the single question this package ever asks Jev, used both to
	// build the request and to read the matching answer back out of the response.
	jevAnswerKey = "answer"
)

// jevChooseInstructions mirrors choosePrompt's rules (see planner.go), narrowed to the
// single question Jev is asked. The spike that validated this wording against the live
// API is recorded in openspec/changes/autofill-choose-jev-provider/design.md.
func jevChooseInstructions(label string) string {
	return fmt.Sprintf(`You are answering ONE question on a job-application form by
picking from the options it offers.

Question: %s

Pick the single option the profile supports. If nothing in the profile supports any
option, pick %q — declining is the correct answer whenever nothing in the profile
supports any of the options. Never infer a claim the profile does not state.`, label, jevDeclineKey)
}

// jevChooser answers one Choose question through the Typesafe AI Jev model's
// /v1/systemone endpoint. One attempt, no retries: a slow or failing Jev call falls back
// to the LLM at the JevPlanner level, so retrying here would only delay that fallback.
type jevChooser struct {
	httpClient *http.Client
	baseURL    string
	apiKey     string
}

type jevChooserOption func(*jevChooser)

// withJevBaseURL points a jevChooser at a test server instead of the live API.
func withJevBaseURL(url string) jevChooserOption {
	return func(c *jevChooser) { c.baseURL = url }
}

func newJevChooser(apiKey string, opts ...jevChooserOption) *jevChooser {
	c := &jevChooser{
		httpClient: &http.Client{Timeout: jevDefaultTimeout},
		baseURL:    jevDefaultBaseURL,
		apiKey:     apiKey,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// jevRequest is the request body for POST /v1/systemone, narrowed to the single
// "choice" question type this package ever asks.
type jevRequest struct {
	State     any                    `json:"state"`
	Model     string                 `json:"model"`
	Questions map[string]jevQuestion `json:"questions"`
}

type jevQuestion struct {
	Type         string            `json:"type"`
	Instructions string            `json:"instructions"`
	Criteria     map[string]string `json:"criteria"`
}

// jevResponse is the response body, narrowed to the one named question ("answer") and
// the one field (its picked choice) this package reads.
type jevResponse struct {
	Answers map[string]struct {
		Choice string `json:"choice"`
	} `json:"answers"`
}

// Choose asks Jev to pick one of options for question, grounded in profile. It returns ""
// for both the decline answer and any sentinel it could not recognize — the caller
// (driveWidget) already re-validates the result against the offered list and the profile,
// so a hallucinated value outside jevDeclineKey or options is caught there, not here.
func (j *jevChooser) Choose(ctx context.Context, question Field, options []string, profile Profile) (string, error) {
	criteria := make(map[string]string, len(options)+1)
	for _, opt := range options {
		criteria[opt] = opt
	}
	criteria[jevDeclineKey] = "nothing in the profile supports any of the listed options"

	body, err := json.Marshal(jevRequest{
		State: map[string]any{"profile": profile},
		Model: jevModel,
		Questions: map[string]jevQuestion{
			jevAnswerKey: {Type: "choice", Instructions: jevChooseInstructions(question.Label), Criteria: criteria},
		},
	})
	if err != nil {
		return "", fmt.Errorf("jev choose: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, j.baseURL+"/v1/systemone", bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("jev choose: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+j.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := j.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("jev choose: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, resp.Body) // drain so the connection can be reused
		return "", fmt.Errorf("jev choose: unexpected status %d", resp.StatusCode)
	}

	var parsed jevResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("jev choose: decode response: %w", err)
	}
	answer, ok := parsed.Answers[jevAnswerKey]
	if !ok {
		return "", fmt.Errorf("jev choose: response carried no %q answer", jevAnswerKey)
	}
	if answer.Choice == jevDeclineKey {
		return "", nil
	}
	return answer.Choice, nil
}
