package assistant

import "testing"

// UserSaid is what lets a tool verify a citation instead of trusting one. Everything the
// honest wall promises on the write side rests on it.
func TestUserSaid(t *testing.T) {
	transcript := []Message{
		mustUser(t, "I ran the Kafka event bus at Sber for two years"),
		mustAssistant(t, "Tell me more about that."),
		mustUser(t, "We handled about 84K RPS across 21 services"),
	}

	tests := []struct {
		name  string
		quote string
		want  bool
	}{
		{"a verbatim quote is found", "ran the Kafka event bus at Sber", true},
		{"case does not matter", "RAN THE KAFKA EVENT BUS", true},
		{"a line wrap does not matter", "ran the  Kafka\nevent bus", true},
		{"a later message counts too", "84K RPS across 21 services", true},
		{"a paraphrase does not pass", "was responsible for Kafka messaging", false},
		{"an invention does not pass", "I led a team of fifty", false},
		{"an empty quote does not pass", "", false},
		{"whitespace alone does not pass", "   ", false},
		{"a single word does not pass even if it is a verbatim hit", "Kafka", false},
		{"two short words do not pass even if the substring is real", "the Kafka", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := UserSaid(transcript, tt.quote); got != tt.want {
				t.Errorf("UserSaid(%q) = %v, want %v", tt.quote, got, tt.want)
			}
		})
	}
}

// The assistant's own words are not the candidate's. Without this the model could quote
// itself into a confirmed claim.
func TestUserSaidIgnoresTheAssistantsOwnWords(t *testing.T) {
	transcript := []Message{
		mustUser(t, "What should I highlight?"),
		mustAssistant(t, "You clearly led the migration to Kubernetes"),
	}
	if UserSaid(transcript, "led the migration to Kubernetes") {
		t.Error("the assistant's own sentence was accepted as the candidate's statement")
	}
}

func TestUserSaidOnAnEmptyTranscript(t *testing.T) {
	if UserSaid(nil, "anything") {
		t.Error("an empty transcript confirmed a quote")
	}
}

// A quote that is a literal substring of a sentence the candidate used to DENY the
// thing must not count as the candidate asserting it — the exact false-positive the
// design's negation reasoning previously skipped over.
func TestUserSaidRejectsAQuoteInsideANegatedSentence(t *testing.T) {
	tests := []struct {
		name       string
		transcript []Message
		quote      string
	}{
		{
			"semicolon-separated denial",
			[]Message{mustUser(t, "I did not lead the migration to Kubernetes; my colleague did all of it")},
			"lead the migration to Kubernetes",
		},
		{
			"contraction denial",
			[]Message{mustUser(t, "I didn't work with Kubernetes at that job")},
			"worked with Kubernetes",
		},
		{
			"explicit NOT, case-insensitive",
			[]Message{mustUser(t, "I have NOT worked with Kubernetes")},
			"worked with Kubernetes",
		},
		{
			"never",
			[]Message{mustUser(t, "I never touched the payments service")},
			"touched the payments service",
		},
		{
			"cannot",
			[]Message{mustUser(t, "I cannot claim experience with distributed tracing")},
			"claim experience with distributed tracing",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if UserSaid(tt.transcript, tt.quote) {
				t.Errorf("UserSaid confirmed %q against a sentence that denies it", tt.quote)
			}
		})
	}
}

// The negation check is scoped to the sentence carrying the quote — an unrelated
// negation elsewhere in the same message must not suppress a genuine, separate
// assertion.
func TestUserSaidStillPassesAGenuineAssertionNearAnUnrelatedNegation(t *testing.T) {
	transcript := []Message{mustUser(t, "I never touched the payments service. I led the migration to Kubernetes.")}
	if !UserSaid(transcript, "led the migration to Kubernetes") {
		t.Error("an unrelated negation in an earlier sentence suppressed a genuine, separate assertion")
	}
}

// A negated first occurrence must not block a later, unnegated occurrence of the exact
// same wording elsewhere in the transcript.
func TestUserSaidFindsALaterUnnegatedOccurrenceAfterAnEarlierDenial(t *testing.T) {
	transcript := []Message{
		mustUser(t, "I did not lead the migration to Kubernetes at my last job"),
		mustAssistant(t, "Got it."),
		mustUser(t, "Actually, I did lead the migration to Kubernetes at my CURRENT job"),
	}
	if !UserSaid(transcript, "lead the migration to Kubernetes") {
		t.Error("a genuine later assertion was rejected because an earlier message denied the same wording")
	}
}

func mustUser(t *testing.T, text string) Message {
	t.Helper()
	m, err := EncodeUser(text)
	if err != nil {
		t.Fatalf("EncodeUser: %v", err)
	}
	return m
}

func mustAssistant(t *testing.T, text string) Message {
	t.Helper()
	m, err := EncodeAssistant(text, nil)
	if err != nil {
		t.Fatalf("EncodeAssistant: %v", err)
	}
	return m
}
