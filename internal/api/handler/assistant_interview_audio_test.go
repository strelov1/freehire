package handler

import (
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/ai/assistant"
)

func TestLatestInterviewReply(t *testing.T) {
	user, err := assistant.EncodeUser("My private recording")
	if err != nil {
		t.Fatal(err)
	}
	reply, err := assistant.EncodeAssistant(" What did you measure? ", nil)
	if err != nil {
		t.Fatal(err)
	}
	tool, err := assistant.EncodeAssistant("", nil)
	if err != nil {
		t.Fatal(err)
	}
	long, err := assistant.EncodeAssistant(strings.Repeat("я", 2_001), nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		name     string
		messages []assistant.Message
		want     string
	}{
		{name: "only the final interviewer reply", messages: []assistant.Message{user, tool, reply}, want: "What did you measure?"},
		{name: "does not speak a pending user answer", messages: []assistant.Message{reply, user, tool}},
		{name: "caps the audio request by runes", messages: []assistant.Message{long}, want: strings.Repeat("я", 2_000)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := latestInterviewReply(tt.messages); got != tt.want {
				t.Fatalf("interviewer reply = %q, want %q", got, tt.want)
			}
		})
	}
}
