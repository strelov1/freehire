package handler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/assistant"
	"github.com/strelov1/freehire/internal/experience"
)

// experienceToolsFor builds the bank tools over a stub, and returns the registry so tool
// failures can be observed the way the model sees them.
func experienceToolsFor(t *testing.T, bank *stubBank) (*assistant.Registry, *assistantHandlers) {
	t.Helper()
	h := &assistantHandlers{experience: bank}
	return assistant.NewRegistry(h.assistantExperienceTools(uuid.New())...), h
}

// A tool failure is not a turn failure: an unusable call comes back as a result the model
// can read and correct within the same turn.
func TestExperienceToolsReportFailuresToTheModel(t *testing.T) {
	reg, _ := experienceToolsFor(t, newStubBank())
	ctx := context.Background()

	tests := []struct {
		name string
		tool string
		args string
		want string
	}{
		{
			name: "a search with nothing to search for",
			tool: "experience_search",
			args: `{}`,
			want: "query",
		},
		{
			name: "an achievement with no claim",
			tool: "experience_add",
			args: `{"claim":"   "}`,
			want: "claim",
		},
		{
			// The bank in this stand has no roles, so the refusal says that rather than
			// pointing at a lookup that would come back empty.
			name: "an employment id that is not an id",
			tool: "experience_add",
			args: `{"claim":"Did a thing","employment_id":"the sber one"}`,
			want: "no roles on file",
		},
		{
			name: "an unknown argument",
			tool: "experience_add",
			args: `{"claim":"Did a thing","provenance":"stated_in_chat"}`,
			want: "provenance",
		},
		{
			name: "an update addressing nothing",
			tool: "experience_update",
			args: `{"id":"not-a-uuid"}`,
			want: "experience_search",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := reg.Call(ctx, 1, tt.tool, json.RawMessage(tt.args))
			var decoded struct {
				Error string `json:"error"`
			}
			if err := json.Unmarshal([]byte(out.Content), &decoded); err != nil {
				t.Fatalf("tool result is not JSON: %v (%s)", err, out.Content)
			}
			if decoded.Error == "" {
				t.Fatalf("call succeeded, want an error result: %s", out.Content)
			}
			if !strings.Contains(decoded.Error, tt.want) {
				t.Errorf("error %q does not name %q — the model cannot correct itself from it", decoded.Error, tt.want)
			}
		})
	}
}

// The model may not stamp its own writes. Passing a provenance is rejected outright by the
// strict decoder, and an achievement recorded without a verifiable quote is the model's
// own — which the result says out loud so the agent knows to ask.
func TestExperienceAddWithoutAQuoteIsTheAgentsOwn(t *testing.T) {
	bank := newStubBank()
	reg, _ := experienceToolsFor(t, bank)

	out := reg.Call(context.Background(), 1, "experience_add",
		json.RawMessage(`{"claim":"Led the Kubernetes migration"}`))

	var decoded struct {
		CanWriteCV *bool  `json:"can_write_cv"`
		Next       string `json:"next"`
		Error      string `json:"error"`
	}
	if err := json.Unmarshal([]byte(out.Content), &decoded); err != nil {
		t.Fatalf("tool result is not JSON: %v (%s)", err, out.Content)
	}
	if decoded.Error != "" {
		t.Fatalf("uncited write failed instead of being recorded: %s", decoded.Error)
	}
	if decoded.CanWriteCV == nil || *decoded.CanWriteCV {
		t.Error("an uncited achievement was reported as usable on a CV")
	}
	if !strings.Contains(decoded.Next, "confirm") {
		t.Errorf("result does not tell the agent to seek confirmation: %q", decoded.Next)
	}
}

// A claim the bank already holds is a fact the model should learn, not an error it should
// retry — otherwise a re-told story burns turns.
func TestExperienceAddReportsAnAlreadyBankedClaim(t *testing.T) {
	bank := newStubBank()
	bank.addErr = experience.ErrAlreadyBanked
	reg, _ := experienceToolsFor(t, bank)

	out := reg.Call(context.Background(), 1, "experience_add",
		json.RawMessage(`{"claim":"Cut latency 20s to 1s"}`))

	var decoded struct {
		AlreadyBanked bool   `json:"already_banked"`
		Error         string `json:"error"`
	}
	if err := json.Unmarshal([]byte(out.Content), &decoded); err != nil {
		t.Fatalf("tool result is not JSON: %v (%s)", err, out.Content)
	}
	if decoded.Error != "" {
		t.Fatalf("an already-banked claim came back as an error: %s", decoded.Error)
	}
	if !decoded.AlreadyBanked {
		t.Error("the result does not tell the model the claim is already recorded")
	}
}

// The search result has to carry what the agent needs to ACT: the id to cite in cv_edit,
// and whether the evidence may be written at all.
func TestExperienceSearchResultCarriesWhatTheAgentMustCite(t *testing.T) {
	role := experience.Employment{ID: uuid.New(), Company: "RingCentral", Role: "SWE", Start: "2023-09", End: "Present"}
	bank := newStubBank()
	bank.matches = []experience.Match{
		{
			Atom: experience.Atom{
				ID: uuid.New(), Claim: "Cut latency 20s to 1s",
				Skills: []string{"mongodb"}, Provenance: experience.ProvenanceCVImport,
			},
			Employment: &role,
			Score:      13,
		},
		{
			Atom: experience.Atom{
				ID: uuid.New(), Claim: "Probably led the migration",
				Provenance: experience.ProvenanceAgentInferred,
			},
			Score: 4,
		},
	}
	reg, _ := experienceToolsFor(t, bank)

	out := reg.Call(context.Background(), 1, "experience_search", json.RawMessage(`{"query":"MongoDB at scale"}`))

	var decoded struct {
		Evidence []struct {
			ID         string `json:"id"`
			Claim      string `json:"claim"`
			Company    string `json:"company"`
			CanWriteCV bool   `json:"can_write_cv"`
		} `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(out.Content), &decoded); err != nil {
		t.Fatalf("tool result is not JSON: %v (%s)", err, out.Content)
	}
	if len(decoded.Evidence) != 2 {
		t.Fatalf("evidence = %d entries, want 2", len(decoded.Evidence))
	}
	if decoded.Evidence[0].ID == "" || decoded.Evidence[0].Company != "RingCentral" {
		t.Errorf("top result lacks the id or the place: %+v", decoded.Evidence[0])
	}
	if !decoded.Evidence[0].CanWriteCV {
		t.Error("confirmed evidence was reported as unusable")
	}
	// The agent's own hypothesis is returned so it can ask about it — flagged, not hidden.
	if decoded.Evidence[1].CanWriteCV {
		t.Error("an agent_inferred entry was reported as usable on a CV")
	}
}

// A wrong employment id used to cost a whole tool round: the model guessed, was told only
// that it was wrong, spent a round on experience_employments, and retried. The refusal now
// carries the ids it could have used, so the correction happens inside the same round.
func TestARejectedEmploymentIdNamesTheValidOnes(t *testing.T) {
	bank := newStubBank()
	bank.employments = []experience.Employment{
		{ID: uuid.MustParse("22222222-2222-4222-8222-222222222222"), Company: "RingCentral", Role: "Tech Lead"},
		{ID: uuid.MustParse("33333333-3333-4333-8333-333333333333"), Company: "Acme", Role: "Engineer"},
	}
	reg, _ := experienceToolsFor(t, bank)

	out := reg.Call(context.Background(), 1, "experience_add",
		json.RawMessage(`{"claim":"Ran the Java containers","employment_id":"the ringcentral one"}`))
	payload, _ := json.Marshal(out)

	for _, want := range []string{"22222222-2222-4222-8222-222222222222", "RingCentral", "Acme"} {
		if !strings.Contains(string(payload), want) {
			t.Errorf("the refusal does not mention %q — the model has to spend a round finding it:\n%s", want, payload)
		}
	}
}

// A candidate with no roles on file gets a refusal that says so, rather than an empty list
// that reads as "the ids exist, you just guessed wrong".
func TestARejectedEmploymentIdWithNoRolesSaysSo(t *testing.T) {
	reg, _ := experienceToolsFor(t, newStubBank())

	out := reg.Call(context.Background(), 1, "experience_add",
		json.RawMessage(`{"claim":"Did a thing","employment_id":"nope"}`))
	payload, _ := json.Marshal(out)
	if !strings.Contains(strings.ToLower(string(payload)), "no roles") {
		t.Errorf("the refusal should say the bank holds no roles yet:\n%s", payload)
	}
}
