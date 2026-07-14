package mailclassify

import (
	"context"
	"strings"
	"testing"
)

type fakeGen struct {
	raw       string
	gotSystem string
	gotUser   string
	err       error
}

func (f *fakeGen) GenerateJSON(_ context.Context, system, user string) (string, error) {
	f.gotSystem, f.gotUser = system, user
	return f.raw, f.err
}

func TestClassifySanitizesAndValidatesMatch(t *testing.T) {
	f := &fakeGen{raw: `{"signal":"garbage","confidence":2,"matched_job_id":999}`}
	c := &Classifier{gen: f}
	in := Input{Subject: "Hi", Candidates: []Candidate{{JobID: 1, Company: "Acme"}}}

	got, err := c.Classify(context.Background(), in)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if got.Signal != SignalOther {
		t.Errorf("signal = %q, want coerced to other", got.Signal)
	}
	if got.Confidence != 1 {
		t.Errorf("confidence = %v, want clamped to 1", got.Confidence)
	}
	if got.MatchedJobID != 0 {
		t.Errorf("matched id = %d, want 0 (999 is not an offered candidate)", got.MatchedJobID)
	}
}

func TestClassifyKeepsValidMatch(t *testing.T) {
	f := &fakeGen{raw: `{"signal":"interview_invitation","confidence":0.9,"matched_job_id":1}`}
	c := &Classifier{gen: f}
	in := Input{Subject: "Interview", Candidates: []Candidate{{JobID: 1, Company: "Acme"}}}

	got, err := c.Classify(context.Background(), in)
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if got.Signal != SignalInterviewInvitation || got.MatchedJobID != 1 {
		t.Errorf("got (%q, %d), want (interview_invitation, 1)", got.Signal, got.MatchedJobID)
	}
}

func TestClassifyUserPromptCarriesCandidates(t *testing.T) {
	f := &fakeGen{raw: `{"signal":"other","confidence":0.1}`}
	c := &Classifier{gen: f}
	in := Input{Subject: "S", Candidates: []Candidate{{JobID: 7, Company: "Globex"}}}

	if _, err := c.Classify(context.Background(), in); err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if !strings.Contains(f.gotUser, "Globex") || !strings.Contains(f.gotUser, "7") {
		t.Errorf("user prompt does not carry candidate id/company:\n%s", f.gotUser)
	}
}
