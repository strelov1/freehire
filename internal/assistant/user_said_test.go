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
