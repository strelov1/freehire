package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/llm"
)

// fakeGen answers with a fixed body, and records the prompt it was given so a test
// can assert what the model was allowed to see.
type fakeGen struct {
	raw    string
	err    error
	system string
	user   string
	calls  int
}

func (f *fakeGen) GenerateJSON(_ context.Context, system, user string, _ ...llm.GenOption) (string, error) {
	f.calls++
	f.system = system
	f.user = user
	return f.raw, f.err
}

func TestSuggestFollowUpsReturnsAtMostThree(t *testing.T) {
	gen := &fakeGen{raw: `{"follow_ups":["one?","two?","three?","four?"]}`}
	f := &FollowUps{gen: gen}

	got, err := f.Suggest(context.Background(), Exchange{Prompt: "hi", Answer: "hello"})
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if len(got) != maxFollowUps {
		t.Fatalf("got %d suggestions, want %d", len(got), maxFollowUps)
	}
	if got[0] != "one?" || got[2] != "three?" {
		t.Errorf("suggestions = %q, want the first three", got)
	}
}

func TestSuggestFollowUpsDiscardsAnOverlongItemRatherThanTruncatingIt(t *testing.T) {
	// A truncated question is a different question, and clicking one speaks in the
	// caller's voice — so the item is dropped, not shortened.
	long := strings.Repeat("a", maxFollowUpLen+1)
	gen := &fakeGen{raw: `{"follow_ups":["short?","` + long + `","also short?"]}`}
	f := &FollowUps{gen: gen}

	got, err := f.Suggest(context.Background(), Exchange{Prompt: "hi", Answer: "hello"})
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if len(got) != 2 || got[0] != "short?" || got[1] != "also short?" {
		t.Errorf("suggestions = %q, want the two short ones", got)
	}
}

func TestSuggestFollowUpsDropsBlankItems(t *testing.T) {
	gen := &fakeGen{raw: `{"follow_ups":["","   ","real?"]}`}
	f := &FollowUps{gen: gen}

	got, err := f.Suggest(context.Background(), Exchange{Prompt: "hi", Answer: "hello"})
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if len(got) != 1 || got[0] != "real?" {
		t.Errorf("suggestions = %q, want only the non-blank one", got)
	}
}

func TestSuggestFollowUpsDropsARepeatedSuggestion(t *testing.T) {
	// Two identical chips are a worse strip than one, and the client keys its list by
	// the suggestion itself — a duplicate there is a render error, not a cosmetic one.
	gen := &fakeGen{raw: `{"follow_ups":["same?","same?","different?"]}`}
	f := &FollowUps{gen: gen}

	got, err := f.Suggest(context.Background(), Exchange{Prompt: "hi", Answer: "hello"})
	if err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if len(got) != 2 || got[0] != "same?" || got[1] != "different?" {
		t.Errorf("suggestions = %q, want the repeat dropped", got)
	}
}

func TestSuggestFollowUpsReportsAModelFailure(t *testing.T) {
	f := &FollowUps{gen: &fakeGen{err: errors.New("gateway down")}}

	got, err := f.Suggest(context.Background(), Exchange{Prompt: "hi", Answer: "hello"})
	if err == nil {
		t.Fatal("Suggest returned no error for a failing model")
	}
	if got != nil {
		t.Errorf("suggestions = %q, want none", got)
	}
}

func TestSuggestFollowUpsReportsAnUnreadableAnswer(t *testing.T) {
	f := &FollowUps{gen: &fakeGen{raw: `not json`}}

	if _, err := f.Suggest(context.Background(), Exchange{Prompt: "hi", Answer: "hello"}); err == nil {
		t.Fatal("Suggest returned no error for an unparseable answer")
	}
}

func TestSuggestFollowUpsTrimsWhatTheModelSees(t *testing.T) {
	// The whole point of a separate call is that it is cheap. Handing it an entire
	// tailoring answer would make it the opposite.
	gen := &fakeGen{raw: `{"follow_ups":["ok?"]}`}
	f := &FollowUps{gen: gen}

	long := strings.Repeat("x", maxExchangeLen*2)
	if _, err := f.Suggest(context.Background(), Exchange{Prompt: long, Answer: long}); err != nil {
		t.Fatalf("Suggest: %v", err)
	}
	if len([]rune(gen.user)) > 2*maxExchangeLen+200 {
		t.Errorf("the model was handed %d runes, want the exchange trimmed", len([]rune(gen.user)))
	}
}

// msg builds a stored message of the given role with the given text.
func msg(t *testing.T, seq int32, role, text string) Message {
	t.Helper()
	var raw []byte
	var err error
	switch role {
	case RoleUser:
		raw, err = json.Marshal(userContent{Text: text})
	case RoleAssistant:
		raw, err = json.Marshal(assistantContent{Text: text})
	default:
		t.Fatalf("msg: unsupported role %q", role)
	}
	if err != nil {
		t.Fatalf("marshal %s content: %v", role, err)
	}
	return Message{Seq: seq, Role: role, Content: raw}
}

func TestLastExchange(t *testing.T) {
	t.Run("the most recent question and its answer", func(t *testing.T) {
		got, ok := LastExchange([]Message{
			msg(t, 1, RoleUser, "first"),
			msg(t, 2, RoleAssistant, "first answer"),
			msg(t, 3, RoleUser, "second"),
			msg(t, 4, RoleAssistant, "second answer"),
		})
		if !ok {
			t.Fatal("LastExchange found nothing in a complete transcript")
		}
		if got.Prompt != "second" || got.Answer != "second answer" {
			t.Errorf("exchange = %+v, want the second pair", got)
		}
	})

	t.Run("skips a tool-calling turn that said nothing", func(t *testing.T) {
		// A turn may end with the model calling tools and writing no prose. The answer
		// worth following up on is the last one with words in it.
		got, ok := LastExchange([]Message{
			msg(t, 1, RoleUser, "find me jobs"),
			msg(t, 2, RoleAssistant, ""),
			msg(t, 3, RoleAssistant, "here are three"),
		})
		if !ok || got.Answer != "here are three" || got.Prompt != "find me jobs" {
			t.Errorf("exchange = %+v, ok = %v, want the spoken answer", got, ok)
		}
	})

	t.Run("an unattended run has an answer but no question", func(t *testing.T) {
		// Autopilot and the rehearsal opening are opened by the server, so there is no
		// user message to pair with. The answer alone is still worth suggesting from.
		got, ok := LastExchange([]Message{msg(t, 1, RoleAssistant, "I tailored your CV")})
		if !ok {
			t.Fatal("LastExchange found nothing for a server-opened turn")
		}
		if got.Prompt != "" || got.Answer != "I tailored your CV" {
			t.Errorf("exchange = %+v, want an answer with no prompt", got)
		}
	})

	t.Run("nothing to follow up on", func(t *testing.T) {
		for name, messages := range map[string][]Message{
			"empty transcript":  {},
			"question only":     {msg(t, 1, RoleUser, "hello")},
			"no spoken answer":  {msg(t, 1, RoleUser, "hi"), msg(t, 2, RoleAssistant, "")},
			"whitespace answer": {msg(t, 1, RoleUser, "hi"), msg(t, 2, RoleAssistant, "   ")},
		} {
			if _, ok := LastExchange(messages); ok {
				t.Errorf("%s: LastExchange reported an exchange, want none", name)
			}
		}
	})
}

func TestNewFollowUpsIsNilWithoutAModel(t *testing.T) {
	// Nil is "this deployment cannot suggest", which the handler renders as an empty
	// list rather than as a failure.
	if f := NewFollowUps(nil); f != nil {
		t.Fatalf("NewFollowUps(nil) = %v, want nil", f)
	}
}

func TestSuggestFollowUpsOnANilReceiverIsEmptyNotAPanic(t *testing.T) {
	var f *FollowUps
	got, err := f.Suggest(context.Background(), Exchange{Prompt: "hi", Answer: "hello"})
	if err != nil {
		t.Errorf("Suggest on a nil receiver returned %v, want no error", err)
	}
	if got != nil {
		t.Errorf("suggestions = %q, want none", got)
	}
}
