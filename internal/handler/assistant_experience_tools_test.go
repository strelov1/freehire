package handler

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"strconv"
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
	role := experience.Employment{ID: uuid.New(), Kind: experience.KindJob, Company: "RingCentral", Role: "SWE", Start: "2023-09", End: "Present"}
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

func TestExperienceSearchProjectPlaceUsesName(t *testing.T) {
	project := experience.Employment{
		ID: uuid.New(), Kind: experience.KindProject, Company: "telagon.io", Link: "https://telagon.io",
	}
	bank := newStubBank()
	bank.matches = []experience.Match{{
		Atom: experience.Atom{
			ID: uuid.New(), Claim: "Shipped the analytics funnel",
			Provenance: experience.ProvenanceCVImport,
		},
		Employment: &project,
		Score:      10,
	}}
	reg, _ := experienceToolsFor(t, bank)

	out := reg.Call(context.Background(), 1, "experience_search", json.RawMessage(`{"query":"analytics"}`))
	var decoded struct {
		Evidence []struct {
			Name    string `json:"name"`
			Company string `json:"company"`
		} `json:"evidence"`
	}
	if err := json.Unmarshal([]byte(out.Content), &decoded); err != nil {
		t.Fatalf("decode: %v (%s)", err, out.Content)
	}
	if len(decoded.Evidence) != 1 || decoded.Evidence[0].Name != "telagon.io" {
		t.Fatalf("evidence = %+v, want name telagon.io", decoded.Evidence)
	}
	if decoded.Evidence[0].Company != "" {
		t.Errorf("project evidence must not carry company, got %q", decoded.Evidence[0].Company)
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

// getResult is what experience_get answers, as the model reads it.
type getResult struct {
	Achievements []struct {
		ID         string   `json:"id"`
		Claim      string   `json:"claim"`
		Context    string   `json:"context"`
		Metrics    []string `json:"metrics"`
		Company    string   `json:"company"`
		CanWriteCV bool     `json:"can_write_cv"`
	} `json:"achievements"`
	Unresolved []string `json:"unresolved"`
	Unread     []string `json:"unread"`
	Error      string   `json:"error"`
}

func readAtoms(t *testing.T, reg *assistant.Registry, ids ...string) (getResult, string) {
	t.Helper()
	args, err := json.Marshal(map[string]any{"ids": ids})
	if err != nil {
		t.Fatalf("marshal args: %v", err)
	}
	out := reg.Call(context.Background(), 1, "experience_get", args)
	var decoded getResult
	if err := json.Unmarshal([]byte(out.Content), &decoded); err != nil {
		t.Fatalf("tool result is not JSON: %v (%s)", err, out.Content)
	}
	return decoded, out.Content
}

// The defect this tool exists for: the interviewer's opening message names achievements by
// id, and until now nothing could turn one into the achievement it names. The agent put the
// ids into experience_search, which retrieves by meaning, and was told the bank held
// nothing — so it answered about achievements the candidate had not selected.
func TestExperienceGetReadsTheAchievementsItIsNamed(t *testing.T) {
	bank := newStubBank()
	role := experience.Employment{ID: uuid.New(), Kind: experience.KindJob, Company: "RingCentral", Role: "Tech Lead", Start: "2023-09", End: "Present"}
	bank.employments = []experience.Employment{role}
	first := bank.add(1, experience.Atom{
		EmploymentID: &role.ID,
		Claim:        "Cut message-posting latency from 20s to 1s",
		Context:      "The posting pipeline fanned out synchronously to every subscriber.",
		Metrics:      []string{"20s -> 1s"},
		Skills:       []string{"mongodb"},
		Provenance:   experience.ProvenanceCVImport,
	})
	second := bank.add(1, experience.Atom{
		Claim: "Probably led the migration", Provenance: experience.ProvenanceAgentInferred,
	})
	bank.reindex()
	reg, _ := experienceToolsFor(t, bank)

	decoded, payload := readAtoms(t, reg, first.ID.String(), second.ID.String())

	if decoded.Error != "" {
		t.Fatalf("reading two of the candidate's own achievements failed: %s", decoded.Error)
	}
	if len(decoded.Achievements) != 2 {
		t.Fatalf("achievements = %d, want 2: %s", len(decoded.Achievements), payload)
	}
	// Answered in the order asked: these ids arrive as a selection, and a reply that
	// follows the question is the one the agent can act on.
	if decoded.Achievements[0].ID != first.ID.String() || decoded.Achievements[1].ID != second.ID.String() {
		t.Errorf("achievements came back out of the order they were asked for: %s", payload)
	}
	got := decoded.Achievements[0]
	if got.Context == "" || len(got.Metrics) == 0 || got.Company != "RingCentral" {
		t.Errorf("the read is missing what makes it worth reading (context, metrics, role): %+v", got)
	}
	if !got.CanWriteCV {
		t.Error("a cv_import achievement was reported as unusable on a CV")
	}
	// The agent's own hypothesis is readable and flagged, exactly as a search reports it.
	if decoded.Achievements[1].CanWriteCV {
		t.Error("an agent_inferred achievement was reported as usable on a CV")
	}
}

// A partial answer is the useful one. Failing the whole call because one id has been merged
// away would cost the agent a round and tell it nothing about which id was wrong.
func TestExperienceGetReportsUnresolvableIdsWithoutFailing(t *testing.T) {
	bank := newStubBank()
	mine := bank.add(1, experience.Atom{Claim: "Ran the Kafka cluster", Provenance: experience.ProvenanceManual})
	bank.reindex()
	reg, _ := experienceToolsFor(t, bank)

	gone := uuid.New().String()
	decoded, payload := readAtoms(t, reg, mine.ID.String(), gone, "not-a-uuid")

	if decoded.Error != "" {
		t.Fatalf("one bad id failed the whole read: %s", decoded.Error)
	}
	if len(decoded.Achievements) != 1 || decoded.Achievements[0].ID != mine.ID.String() {
		t.Fatalf("the resolvable achievement did not come back: %s", payload)
	}
	for _, want := range []string{gone, "not-a-uuid"} {
		if !slices.Contains(decoded.Unresolved, want) {
			t.Errorf("unresolved does not name %q, so the agent cannot tell what it got wrong: %s", want, payload)
		}
	}
}

// The tool must not become an existence oracle for other people's rows: an achievement
// belonging to someone else reads exactly like one that was deleted.
func TestExperienceGetTreatsAnotherAccountsAchievementAsAbsent(t *testing.T) {
	bank := newStubBank()
	theirs := bank.add(2, experience.Atom{
		Claim: "Ran payroll for a FTSE 100", Provenance: experience.ProvenanceManual,
	})
	bank.reindex() // readAs is 1, so their atom is not in this caller's bank

	reg, _ := experienceToolsFor(t, bank)

	foreign, foreignPayload := readAtoms(t, reg, theirs.ID.String())
	deleted, _ := readAtoms(t, reg, uuid.New().String())

	if len(foreign.Achievements) != 0 {
		t.Fatalf("another account's achievement was returned: %s", foreignPayload)
	}
	if strings.Contains(foreignPayload, "payroll") {
		t.Errorf("another account's claim text leaked into the result: %s", foreignPayload)
	}
	if len(foreign.Unresolved) != len(deleted.Unresolved) || len(deleted.Achievements) != 0 {
		t.Errorf("a foreign id and a deleted id are distinguishable:\nforeign %+v\ndeleted %+v", foreign, deleted)
	}
}

// One read is bounded like one search, because the result is replayed into the model's
// context every later turn. What it must not do is drop the excess quietly — the ids were
// named deliberately, so the agent has to learn which ones it has not seen.
func TestExperienceGetCapsOneReadAndNamesTheRemainder(t *testing.T) {
	bank := newStubBank()
	ids := make([]string, 0, experienceReadLimit+2)
	for i := range experienceReadLimit + 2 {
		a := bank.add(1, experience.Atom{
			Claim: "Achievement " + strconv.Itoa(i), Provenance: experience.ProvenanceManual,
		})
		ids = append(ids, a.ID.String())
	}
	bank.reindex()
	reg, _ := experienceToolsFor(t, bank)

	decoded, payload := readAtoms(t, reg, ids...)

	if len(decoded.Achievements) != experienceReadLimit {
		t.Fatalf("achievements = %d, want the cap of %d: %s", len(decoded.Achievements), experienceReadLimit, payload)
	}
	if len(decoded.Unread) != 2 {
		t.Fatalf("unread = %v, want the 2 ids beyond the cap: %s", decoded.Unread, payload)
	}
	for _, want := range ids[experienceReadLimit:] {
		if !slices.Contains(decoded.Unread, want) {
			t.Errorf("unread does not name %q, so the agent believes it saw everything", want)
		}
	}
}

// Two shapes for one concept is how a model ends up believing a searched achievement and a
// read achievement are different kinds of thing. They share a builder; this is the guard
// that keeps them sharing it.
func TestAReadAchievementLooksLikeASearchedOne(t *testing.T) {
	role := experience.Employment{ID: uuid.New(), Company: "Acme", Role: "Engineer", Start: "2021-01", End: "2024-06"}
	atom := experience.Atom{
		ID: uuid.New(), EmploymentID: &role.ID,
		Claim: "Halved the build", Context: "CI ran every suite on every push.",
		Metrics: []string{"22m -> 11m"}, Skills: []string{"go"},
		Provenance: experience.ProvenanceCVImport,
	}
	bank := newStubBank()
	bank.employments = []experience.Employment{role}
	bank.list = []experience.Atom{atom}
	bank.matches = []experience.Match{{Atom: atom, Employment: &role, Score: 9}}
	reg, _ := experienceToolsFor(t, bank)

	searched := reg.Call(context.Background(), 1, "experience_search", json.RawMessage(`{"query":"build times"}`))
	read := reg.Call(context.Background(), 1, "experience_get",
		json.RawMessage(`{"ids":["`+atom.ID.String()+`"]}`))

	var fromSearch struct {
		Evidence []map[string]any `json:"evidence"`
	}
	var fromRead struct {
		Achievements []map[string]any `json:"achievements"`
	}
	if err := json.Unmarshal([]byte(searched.Content), &fromSearch); err != nil {
		t.Fatalf("search result is not JSON: %v", err)
	}
	if err := json.Unmarshal([]byte(read.Content), &fromRead); err != nil {
		t.Fatalf("read result is not JSON: %v", err)
	}
	if len(fromSearch.Evidence) != 1 || len(fromRead.Achievements) != 1 {
		t.Fatalf("want one achievement from each: search %v, read %v", fromSearch.Evidence, fromRead.Achievements)
	}
	if !maps.Equal(
		fieldNames(fromSearch.Evidence[0]),
		fieldNames(fromRead.Achievements[0]),
	) {
		t.Errorf("the same achievement carries different fields depending on how it was fetched:\nsearch %v\nread   %v",
			fromSearch.Evidence[0], fromRead.Achievements[0])
	}
}

// fieldNames reduces an entry to its field names, so the comparison is about shape.
func fieldNames(entry map[string]any) map[string]bool {
	out := make(map[string]bool, len(entry))
	for k := range entry {
		out[k] = true
	}
	return out
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

// Editing the claim of an atom the candidate already confirmed must re-earn provenance —
// otherwise the model can quietly rewrite what a confirmed achievement says (a different
// number, a bigger scope) and the CV gate still treats it as the candidate's own words,
// because it only ever looks at the provenance column, not at whether today's text is what
// was confirmed.
func TestExperienceUpdateDropsProvenanceWhenTheClaimChangesWithoutANewQuote(t *testing.T) {
	bank := newStubBank()
	stored := bank.add(1, experience.Atom{Claim: "Cut latency 20s to 1s", Provenance: experience.ProvenanceStatedInChat})
	reg, _ := experienceToolsFor(t, bank)

	out := reg.Call(context.Background(), 1, "experience_update",
		json.RawMessage(`{"id":"`+stored.ID.String()+`","claim":"Cut latency 20s to 100ms"}`))
	var decoded struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(out.Content), &decoded); err != nil {
		t.Fatalf("tool result is not JSON: %v (%s)", err, out.Content)
	}
	if decoded.Error != "" {
		t.Fatalf("update failed: %s", decoded.Error)
	}
	if got := bank.atoms[stored.ID].Provenance; got != experience.ProvenanceAgentInferred {
		t.Errorf("provenance = %q after an unconfirmed claim edit, want %q", got, experience.ProvenanceAgentInferred)
	}
}

// A field that carries no factual assertion — which role an achievement belongs to — must
// not cost the atom its confirmed provenance. Only rewriting what the atom claims should
// require re-confirmation.
func TestExperienceUpdateKeepsProvenanceWhenOnlyEmploymentChanges(t *testing.T) {
	bank := newStubBank()
	stored := bank.add(1, experience.Atom{Claim: "Cut latency 20s to 1s", Provenance: experience.ProvenanceStatedInChat})
	reg, _ := experienceToolsFor(t, bank)

	out := reg.Call(context.Background(), 1, "experience_update",
		json.RawMessage(`{"id":"`+stored.ID.String()+`","employment_id":"`+uuid.New().String()+`"}`))
	var decoded struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(out.Content), &decoded); err != nil {
		t.Fatalf("tool result is not JSON: %v (%s)", err, out.Content)
	}
	if decoded.Error != "" {
		t.Fatalf("update failed: %s", decoded.Error)
	}
	if got := bank.atoms[stored.ID].Provenance; got != experience.ProvenanceStatedInChat {
		t.Errorf("provenance = %q after a no-op update, want unchanged %q", got, experience.ProvenanceStatedInChat)
	}
}
