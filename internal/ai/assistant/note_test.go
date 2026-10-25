package assistant

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/tmc/langchaingo/llms"

	"github.com/strelov1/freehire/internal/platform/db"
)

// A status note is freehire telling the model what happened, not the candidate speaking:
// it is stored apart from their messages, never names the conversation, and the client is
// not told the candidate said it.
func TestRunNoteStoresTheNoteApartFromTheCandidatesWords(t *testing.T) {
	model := &scriptedModel{replies: []*llms.ContentChoice{textReply("Your CV is tailored; two questions are open.")}}
	q := &fakeQueries{session: db.AssistantSession{ID: sessionID, UserID: 7, Preset: PresetApply}}
	r := testRunner(model, q)
	sess := Session{ID: sessionID, UserID: 7, Preset: PresetApply}

	var events []Event
	if err := r.RunNote(context.Background(), sess, NewRegistry(), "system prompt", "tailored; blocked on 2 questions", TurnConfig{},
		func(e Event) { events = append(events, e) }); err != nil {
		t.Fatalf("RunNote: %v", err)
	}

	if len(q.messages) == 0 || q.messages[0].Role != RoleNote {
		t.Fatalf("first stored message = %+v, want a %q note", q.messages, RoleNote)
	}
	if q.labelSet != "" {
		t.Errorf("the note named the conversation %q", q.labelSet)
	}
	if hasKind(events, EventUserPrompt) {
		t.Error("the client was told the candidate said the note")
	}
	if !hasKind(events, EventResult) {
		t.Error("the turn emitted no result")
	}
}

// The model reads the note as freehire's, never as the candidate's — otherwise "the
// application was submitted" would read as the candidate asking it to be.
func TestANoteReachesTheModelMarkedAsFreehires(t *testing.T) {
	msg, err := EncodeNote("submitted to the employer")
	if err != nil {
		t.Fatalf("EncodeNote: %v", err)
	}
	got, err := msg.Decode()
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.Role != llms.ChatMessageTypeHuman {
		t.Fatalf("role = %q, want a human-turn wrapper every provider accepts mid-conversation", got.Role)
	}
	text := got.Parts[0].(llms.TextContent).Text
	if !strings.Contains(text, "submitted to the employer") || !strings.Contains(text, "not written by the candidate") {
		t.Errorf("decoded note = %q", text)
	}
}

// The same narration retried after the model failed it is not stored again: during an outage
// the client retries every fifteen seconds, and each copy would be replayed on every turn.
func TestRunNoteDoesNotStoreTheSameNoteTwice(t *testing.T) {
	model := &scriptedModel{err: errors.New("upstream is down")}
	q := &fakeQueries{session: db.AssistantSession{ID: sessionID, UserID: 7, Preset: PresetApply}}
	r := testRunner(model, q)
	sess := Session{ID: sessionID, UserID: 7, Preset: PresetApply}

	for range 3 {
		_ = r.RunNote(context.Background(), sess, NewRegistry(), "system prompt", "blocked on 2 questions", TurnConfig{}, func(Event) {})
	}

	notes := 0
	for _, m := range q.messages {
		if m.Role == RoleNote {
			notes++
		}
	}
	if notes != 1 {
		t.Errorf("stored %d copies of the same note, want 1", notes)
	}
}

// A conversation whose only turn so far is a note can still be retried.
func TestContinueAcceptsANoteStartedConversation(t *testing.T) {
	msg, _ := EncodeNote("queued")
	if !hasUserMessage([]Message{msg}) {
		t.Error("a note-started transcript reports nothing to continue")
	}
}
