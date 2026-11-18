package mailclassify

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/platform/jev"
)

// jevRequest is the wire body as the test server reads it.
type jevRequest struct {
	Model     string                  `json:"model"`
	State     jevState                `json:"state"`
	Questions map[string]jev.Question `json:"questions"`
}

func TestJevRubricDescribesEveryValidSignal(t *testing.T) {
	for signal := range validSignals {
		if signalRubric[signal] == "" {
			t.Errorf("signal %q has no Jev rubric", signal)
		}
	}
}

func TestJevClientUsesChosenSignalProbability(t *testing.T) {
	var got jevRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/systemone" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer test-key" {
			t.Errorf("authorization = %q", r.Header.Get("Authorization"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode request: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"answers": map[string]any{
				jevSignalQuestion: map[string]any{
					"choice":     "offer",
					"confidence": 0.2,
					"probabilities": map[string]float64{
						"offer": 0.88,
						"other": 0.12,
					},
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	client := newJevClient("test-key", jev.WithBaseURL(srv.URL))
	out, err := client.Classify(context.Background(), Input{
		FromName: "Workable",
		Subject:  "Thanks for applying to Derq",
		Body:     "We would like to offer you the role.",
	})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if out.Signal != SignalOffer || out.Confidence != 0.88 || out.MatchedJobID != 0 {
		t.Errorf("got (%q, %v, %d), want (offer, 0.88, 0)", out.Signal, out.Confidence, out.MatchedJobID)
	}
	if _, ok := got.Questions[jevApplicationQuestion]; ok {
		t.Error("application question was asked without candidates")
	}
	if got.Model != jevModel || got.State.Subject != "Thanks for applying to Derq" {
		t.Errorf("request model=%q subject=%q", got.Model, got.State.Subject)
	}
	for _, name := range SignalValues {
		if got.Questions[jevSignalQuestion].Criteria[name] == "" {
			t.Errorf("signal %q missing from criteria", name)
		}
	}
}

func TestJevClientPicksOfferedApplication(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"answers": map[string]any{
				jevSignalQuestion: map[string]any{
					"choice":        "acknowledgement",
					"probabilities": map[string]float64{"acknowledgement": 0.97},
				},
				jevApplicationQuestion: map[string]any{
					"choice":        "7",
					"probabilities": map[string]float64{"7": 0.8, "0": 0.2},
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	client := newJevClient("test-key", jev.WithBaseURL(srv.URL))
	out, err := client.Classify(context.Background(), Input{
		Subject:    "Thanks for applying",
		Body:       "We received your application.",
		Candidates: []Candidate{{JobID: 7, Company: "Globex"}, {JobID: 8, Company: "Initech"}},
	})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if out.Signal != SignalAcknowledgement || out.MatchedJobID != 7 || out.Confidence != 0.97 {
		t.Errorf("got (%q, %d, %v)", out.Signal, out.MatchedJobID, out.Confidence)
	}
	if out.MatchConfidence != 0.8 {
		t.Errorf("match confidence = %v, want 0.8 (the application answer's probability)", out.MatchConfidence)
	}
}

func TestJevClientRejectsApplicationWithoutProbability(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"answers": map[string]any{
				jevSignalQuestion: map[string]any{
					"choice":        "acknowledgement",
					"probabilities": map[string]float64{"acknowledgement": 0.97},
				},
				jevApplicationQuestion: map[string]any{"choice": "7"},
			},
		})
	}))
	t.Cleanup(srv.Close)

	client := newJevClient("test-key", jev.WithBaseURL(srv.URL))
	if _, err := client.Classify(context.Background(), Input{
		Subject:    "Thanks for applying",
		Candidates: []Candidate{{JobID: 7, Company: "Globex"}},
	}); err == nil {
		t.Fatal("a picked application without a probability must be rejected, not linked at 0")
	}
}

func TestJevClientNoneApplicationStaysUnlinked(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"answers": map[string]any{
				jevSignalQuestion: map[string]any{
					"choice":        "other",
					"probabilities": map[string]float64{"other": 0.6},
				},
				jevApplicationQuestion: map[string]any{"choice": jevNoneApplication},
			},
		})
	}))
	t.Cleanup(srv.Close)

	client := newJevClient("test-key", jev.WithBaseURL(srv.URL))
	out, err := client.Classify(context.Background(), Input{
		Subject:    "Code",
		Candidates: []Candidate{{JobID: 7, Company: "Globex"}},
	})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if out.MatchedJobID != 0 || out.Confidence != 0.6 {
		t.Errorf("got job=%d confidence=%v", out.MatchedJobID, out.Confidence)
	}
}

func TestJevClientRejectsUnknownApplication(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"answers": map[string]any{
				jevSignalQuestion: map[string]any{
					"choice":        "rejection",
					"probabilities": map[string]float64{"rejection": 0.9},
				},
				jevApplicationQuestion: map[string]any{"choice": "999"},
			},
		})
	}))
	t.Cleanup(srv.Close)

	client := newJevClient("test-key", jev.WithBaseURL(srv.URL))
	_, err := client.Classify(context.Background(), Input{
		Subject:    "No",
		Candidates: []Candidate{{JobID: 1, Company: "Acme"}},
	})
	if err == nil {
		t.Fatal("Classify accepted an application that was not offered")
	}
}

func TestJevClientBoundsSenderFields(t *testing.T) {
	var raw []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var err error
		raw, err = io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read body: %v", err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"answers": map[string]any{
				jevSignalQuestion: map[string]any{
					"choice":        "other",
					"probabilities": map[string]float64{"other": 1},
				},
			},
		})
	}))
	t.Cleanup(srv.Close)

	huge := strings.Repeat("s", 50_000)
	client := newJevClient("test-key", jev.WithBaseURL(srv.URL))
	if _, err := client.Classify(context.Background(), Input{FromName: huge, Subject: huge, Body: huge}); err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if strings.Contains(string(raw), huge) {
		t.Error("a sender-controlled field reached Jev at its own length")
	}
}

func TestJevClientRefusesTooManyApplications(t *testing.T) {
	candidates := make([]Candidate, jevMaxOptions)
	for i := range candidates {
		candidates[i] = Candidate{JobID: int64(i + 1), Company: "C"}
	}
	client := newJevClient("test-key")
	if _, err := client.Classify(context.Background(), Input{Candidates: candidates}); err == nil {
		t.Fatal("Classify accepted more applications than a choice question allows")
	}
}

func TestJevClientErrorsOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	client := newJevClient("bad-key", jev.WithBaseURL(srv.URL))
	if _, err := client.Classify(context.Background(), Input{Subject: "Hi"}); err == nil {
		t.Fatal("Classify against a 401 returned no error")
	}
}
