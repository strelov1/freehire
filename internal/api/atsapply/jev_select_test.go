package atsapply

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/candidate/experience"
)

// jevSelectServer replays the given answer for any /v1/systemone call, and records the
// last request body it received so a test can assert on what was actually sent.
func jevSelectServer(t *testing.T, answer map[string]any) (*httptest.Server, *[]byte) {
	t.Helper()
	var lastBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		lastBody, _ = io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":   "jev-latest",
			"answers": map[string]any{jevSelectAnswerKey: answer},
			"usage":   map[string]any{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &lastBody
}

func atom(claim string) experience.Atom {
	return experience.Atom{Claim: claim}
}

func TestJevSelectChooserPicksTheSupportedOption(t *testing.T) {
	srv, _ := jevSelectServer(t, map[string]any{"choice": "LinkedIn"})
	chooser := newJevSelectChooser("test-key", withJevSelectBaseURL(srv.URL))

	question := MergedField{Label: "How did you hear about this position?"}
	options := []string{"LinkedIn", "Referral", "Job Board"}
	facts := []experience.Atom{atom("Found this role through a LinkedIn job posting")}

	got, ok, err := chooser.Choose(context.Background(), question, options, facts)
	if err != nil {
		t.Fatalf("Choose: %v", err)
	}
	if !ok || got != "LinkedIn" {
		t.Errorf("Choose = (%q, %v), want (\"LinkedIn\", true)", got, ok)
	}
}

func TestJevSelectChooserDeclinesWhenNothingSupportsAnOption(t *testing.T) {
	srv, _ := jevSelectServer(t, map[string]any{"choice": jevSelectDeclineKey})
	chooser := newJevSelectChooser("test-key", withJevSelectBaseURL(srv.URL))

	question := MergedField{Label: "What is your preferred start date range?"}
	options := []string{"Immediately", "2-4 weeks", "1-2 months"}
	facts := []experience.Atom{atom("5 years as Backend Engineer at Acme Corp")}

	_, ok, err := chooser.Choose(context.Background(), question, options, facts)
	if err != nil {
		t.Fatalf("Choose: %v", err)
	}
	if ok {
		t.Error("ok = true, want false (decline)")
	}
}

// TestJevSelectChooserSendsTheCategoricalExclusionForEveryCall is the adversarial check
// design.md calls out as the riskiest unknown: the instruction that compensation/
// demographics/identity/legal-status questions must decline EVEN IF a stated fact looks
// superficially supportive has to travel with every select call, not just ones that look
// sensitive — inspecting the outgoing request is the only way to confirm the rule was
// actually transmitted, since a canned mock response can't exercise real model reasoning.
func TestJevSelectChooserSendsTheCategoricalExclusionForEveryCall(t *testing.T) {
	srv, body := jevSelectServer(t, map[string]any{"choice": jevSelectDeclineKey})
	chooser := newJevSelectChooser("test-key", withJevSelectBaseURL(srv.URL))

	question := MergedField{Label: "What is your desired annual compensation?"}
	options := []string{"<80k", "80k-120k", "120k+"}
	facts := []experience.Atom{atom("Negotiated a salary range of $100k-$130k in a previous interview process")}

	_, ok, err := chooser.Choose(context.Background(), question, options, facts)
	if err != nil {
		t.Fatalf("Choose: %v", err)
	}
	if ok {
		t.Error("ok = true, want false (decline) for a compensation question")
	}

	var sentReq jevSelectRequest
	if err := json.Unmarshal(*body, &sentReq); err != nil {
		t.Fatalf("decode sent request body: %v", err)
	}
	// Collapse the instructions' own line wrapping so a phrase split across lines (by
	// gofmt-friendly 80-ish column width in jevSelectInstructions) still matches.
	instructions := strings.Join(strings.Fields(sentReq.Questions[jevSelectAnswerKey].Instructions), " ")
	for _, phrase := range []string{"compensation", "legal work status", "regardless of what else is stated"} {
		if !strings.Contains(instructions, phrase) {
			t.Errorf("instructions do not mention %q — the categorical-exclusion rule was not actually transmitted:\n%s", phrase, instructions)
		}
	}
}

func TestJevSelectChooserErrorsOnUnreachableServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close()

	chooser := newJevSelectChooser("test-key", withJevSelectBaseURL(srv.URL))
	if _, _, err := chooser.Choose(context.Background(), MergedField{Label: "Q"}, []string{"A", "B"}, nil); err == nil {
		t.Fatal("Choose against an unreachable server returned no error")
	}
}

func TestJevSelectChooserErrorsOnMalformedJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"answers": not valid json`))
	}))
	t.Cleanup(srv.Close)

	chooser := newJevSelectChooser("test-key", withJevSelectBaseURL(srv.URL))
	if _, _, err := chooser.Choose(context.Background(), MergedField{Label: "Q"}, []string{"A", "B"}, nil); err == nil {
		t.Fatal("Choose against a malformed JSON body returned no error")
	}
}

func TestJevSelectChooserErrorsWhenAnswerKeyIsMissing(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"model":   "jev-latest",
			"answers": map[string]any{},
			"usage":   map[string]any{"input_tokens": 1, "output_tokens": 1},
		})
	}))
	t.Cleanup(srv.Close)

	chooser := newJevSelectChooser("test-key", withJevSelectBaseURL(srv.URL))
	if _, _, err := chooser.Choose(context.Background(), MergedField{Label: "Q"}, []string{"A", "B"}, nil); err == nil {
		t.Fatal("Choose against a response with no answer entry returned no error")
	}
}

func TestJevSelectChooserErrorsOnNon200Status(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	chooser := newJevSelectChooser("bad-key", withJevSelectBaseURL(srv.URL))
	if _, _, err := chooser.Choose(context.Background(), MergedField{Label: "Q"}, []string{"A", "B"}, nil); err == nil {
		t.Fatal("Choose against a 401 response returned no error")
	}
}
