package handler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/resumeextract"
	"github.com/strelov1/freehire/internal/userprofile"
)

// assistantWithProfile builds the agent handlers over an in-memory profile repo and the
// given résumé reader, which is all the profile tool touches.
func assistantWithProfile(repo *fakeProfileRepo, cv structuredResumeReader) *assistantHandlers {
	return &assistantHandlers{profile: &profileHandlers{
		userProfile: userprofile.New(repo),
		resume:      cv,
	}}
}

func TestGetProfileTool_IsRegisteredForEverySession(t *testing.T) {
	h := assistantWithProfile(savedProfile(), nil)

	// Not in the tailoring branch — the agent needs the user's stated preferences on
	// every conversation, which is the whole point of not interrogating them for it.
	toolByName(t, h.assistantDiscoveryTools(), "get_profile")
}

func TestGetProfileTool_ReturnsThePreferencesAndTheContactFreeCV(t *testing.T) {
	h := assistantWithProfile(savedProfile(), fakeStructuredResume{
		ret: resumeextract.Structured{
			FullName:   "Ada Lovelace",
			Email:      "ada@example.com",
			Phone:      "+351 900 000 000",
			Links:      []string{"https://github.com/ada"},
			Headline:   "Staff Backend Engineer",
			TotalYears: 11,
		},
		ok: true,
	})

	out, err := toolByName(t, h.assistantDiscoveryTools(), "get_profile").
		Run(context.Background(), 1, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("get_profile: %v", err)
	}
	blob, err := json.Marshal(out)
	if err != nil {
		t.Fatalf("marshal tool result: %v", err)
	}

	for _, want := range []string{"backend", "go", "Staff Backend Engineer"} {
		if !strings.Contains(string(blob), want) {
			t.Errorf("tool result is missing %q:\n%s", want, blob)
		}
	}
	// The transcript is persisted and replayed into the model's context, so a contact
	// that reaches a tool result stays in the conversation for good.
	for _, leaked := range []string{"Ada Lovelace", "ada@example.com", "+351 900 000 000", "github.com/ada"} {
		if strings.Contains(string(blob), leaked) {
			t.Errorf("tool result leaks the contact %q:\n%s", leaked, blob)
		}
	}
}

// A user with no profile must not be interrogated for what the profile page collects:
// the tool says so in terms the model can act on, rather than failing.
func TestGetProfileTool_TellsTheAgentToSendTheUserToTheProfilePage(t *testing.T) {
	h := assistantWithProfile(&fakeProfileRepo{getErr: userprofile.ErrNotFound}, nil)

	out, err := toolByName(t, h.assistantDiscoveryTools(), "get_profile").
		Run(context.Background(), 1, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("a missing profile is an answer, not a tool failure: %v", err)
	}
	blob, _ := json.Marshal(out)
	if !strings.Contains(string(blob), "/my/profile") {
		t.Errorf("result should point the agent at the profile page:\n%s", blob)
	}
}
