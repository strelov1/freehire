package handler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/identity/userprofile"
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

func TestMaskAccountEmail(t *testing.T) {
	cases := map[string]string{
		"jane@gmail.com": "j***@gmail.com",
		"a@x.io":         "a***@x.io",
		"élan@mail.ru":   "é***@mail.ru",
		"@x.io":          "",
		"no-at-sign":     "",
		"":               "",
	}
	for in, want := range cases {
		if got := maskAccountEmail(in); got != want {
			t.Errorf("maskAccountEmail(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNoProfileNextNamesTheAccountOnlyWhenKnown(t *testing.T) {
	if got := noProfileNext("j***@gmail.com"); !strings.Contains(got, "j***@gmail.com") || !strings.Contains(got, "reconnect") || !strings.Contains(got, "/my/profile") {
		t.Errorf("next = %q", got)
	}
	if got := noProfileNext(""); strings.Contains(got, "connected as") {
		t.Errorf("next names an account it does not know: %q", got)
	}
}

func TestGetProfileTool_ReturnsSenioritiesAndJobLanguages(t *testing.T) {
	repo := savedProfile()
	repo.getRet.Seniorities = []string{"senior"}
	repo.getRet.JobLanguages = []string{"en", "pt"}
	h := assistantWithProfile(repo, nil)

	out, err := toolByName(t, h.assistantDiscoveryTools(), "get_profile").
		Run(context.Background(), 1, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("get_profile: %v", err)
	}
	blob, _ := json.Marshal(out)
	// update_profile replaces these lists whole, so the agent must be able to read them first.
	for _, want := range []string{`"seniorities":["senior"]`, `"job_languages":["en","pt"]`} {
		if !strings.Contains(string(blob), want) {
			t.Errorf("get_profile is missing %s:\n%s", want, blob)
		}
	}
}

func TestGetProfileTool_ServesTheBackgroundAndKeepsCVAsItsAlias(t *testing.T) {
	h := assistantWithProfile(savedProfile(), fakeStructuredResume{
		ok: true, ret: resumeextract.Structured{Headline: "Staff Backend Engineer"},
	})
	out, err := toolByName(t, h.assistantDiscoveryTools(), "get_profile").
		Run(context.Background(), 1, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("get_profile: %v", err)
	}
	var got struct {
		Background, CV *resumeextract.Professional
	}
	blob, _ := json.Marshal(out)
	if err := json.Unmarshal(blob, &struct {
		B **resumeextract.Professional `json:"background"`
		C **resumeextract.Professional `json:"cv"`
	}{&got.Background, &got.CV}); err != nil {
		t.Fatal(err)
	}
	if got.Background == nil || got.CV == nil || got.Background.Headline != "Staff Backend Engineer" || got.CV.Headline != got.Background.Headline {
		t.Errorf("background and its deprecated cv alias must carry the same data:\n%s", blob)
	}
}

func TestGetProfileTool_NoPreferencesRowStillServesTheSavedText(t *testing.T) {
	repo := &fakeProfileRepo{getErr: userprofile.ErrNotFound}
	h := assistantWithProfile(repo, fakeStructuredResume{
		ok: true, ret: resumeextract.Structured{Headline: "Staff Backend Engineer"},
	})
	out, err := toolByName(t, h.assistantDiscoveryTools(), "get_profile").
		Run(context.Background(), 1, json.RawMessage(`{}`))
	if err != nil {
		t.Fatalf("get_profile: %v", err)
	}
	blob, _ := json.Marshal(out)
	// update_profile can save the headline without a preferences row; hiding it behind
	// "no profile yet" would make the agent write it again.
	if !strings.Contains(string(blob), `"background":{"headline":"Staff Backend Engineer"`) || !strings.Contains(string(blob), `"next"`) {
		t.Errorf("want the saved text and the no-preferences hint:\n%s", blob)
	}
}
