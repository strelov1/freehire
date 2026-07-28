package mailclassify

import (
	"context"
	"slices"
	"testing"
)

func TestRequestSchema_ConstrainsSignalToTheVocabulary(t *testing.T) {
	s, err := requestSchema()
	if err != nil {
		t.Fatalf("requestSchema: %v", err)
	}

	props, ok := s["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema carries no properties")
	}
	signal, ok := props["signal"].(map[string]any)
	if !ok {
		t.Fatal("schema carries no signal property")
	}
	values, ok := signal["enum"].([]any)
	if !ok {
		t.Fatal("signal carries no enum")
	}

	got := make([]string, 0, len(values))
	for _, v := range values {
		if s, ok := v.(string); ok {
			got = append(got, s)
		}
	}
	if !slices.Equal(got, SignalValues) {
		t.Errorf("signal enum = %v, want the vocabulary %v", got, SignalValues)
	}
}

// The vocabulary must reach the schema and the receipt-side check from one place, or a
// label added to one would be rejected by the other.
func TestSignalValues_AreExactlyTheValidSignals(t *testing.T) {
	if len(SignalValues) != len(validSignals) {
		t.Fatalf("SignalValues has %d entries, validSignals %d", len(SignalValues), len(validSignals))
	}
	for _, s := range SignalValues {
		if !IsValidSignal(s) {
			t.Errorf("%q is offered to the model but rejected on receipt", s)
		}
	}
}

// The schema forecloses an out-of-vocabulary label; the check on receipt is what
// survives a gateway that stops honouring the schema.
func TestClassify_OutOfVocabularyLabelIsStillCoerced(t *testing.T) {
	f := &fakeGen{raw: `{"signal":"ghosted_probably","confidence":0.9,"matched_job_id":0}`}

	c := &Classifier{gen: f}

	got, err := c.Classify(context.Background(), Input{Subject: "s", Body: "b"})
	if err != nil {
		t.Fatalf("Classify: %v", err)
	}
	if got.Signal != SignalOther {
		t.Errorf("signal = %q, want %q — validation must not lean on the schema", got.Signal, SignalOther)
	}
}
