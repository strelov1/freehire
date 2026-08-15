package assistant

import (
	"strings"
	"testing"
)

func TestEachPresetHasItsOwnPrompt(t *testing.T) {
	chat := SystemPrompt(PresetChat, "en")
	tailor := SystemPrompt(PresetTailor, "en")
	browse := SystemPrompt(PresetBrowse, "en")

	if chat == "" || tailor == "" || browse == "" {
		t.Fatal("every preset needs a system prompt; an unprompted agent has no job to do")
	}
	if chat == tailor || chat == browse || tailor == browse {
		t.Error("two presets share a prompt; the preset is what makes them different")
	}
}

// Wording is handled outside the conversation. Without this sentence the agent rediscovers
// IaC ↔ infrastructure as code and burns the turn on it. The sentence must not claim the
// alignment already ran: a tailored copy minted before the prepass existed, or one for a
// vacancy that names no interchangeable skill, reaches this prompt unaligned.
func TestTailorPromptTellsTheAgentNotToSpendEditsOnWording(t *testing.T) {
	p := SystemPrompt(PresetTailor, "en")
	if !strings.Contains(p, "handled outside this conversation") {
		t.Error("tailor prompt never says skill wording is handled elsewhere")
	}
	if !strings.Contains(p, "Do NOT spend an edit renaming a skill for wording") {
		t.Error("tailor prompt never tells the agent not to spend edits on wording")
	}
	if strings.Contains(p, "already aligned") {
		t.Error("tailor prompt asserts the CV is already aligned, which is not true on every path")
	}
}

// The panel's agent is the only one with eyes. A prompt that does not say so
// produces an agent that asks the candidate to paste the vacancy it could have
// read itself.
func TestBrowsePromptTellsTheAgentToReadThePage(t *testing.T) {
	p := SystemPrompt(PresetBrowse, "en")

	if !strings.Contains(p, "read_current_page") {
		t.Error("the browse prompt never names read_current_page, so the agent will not know it can see the page")
	}
}

// The candidate has already told the product their roles, skills and geography on
// the profile page. A prompt that does not say to read it produces an agent that
// opens every conversation with a questionnaire.
func TestChatPromptReadsTheProfileBeforeInterrogating(t *testing.T) {
	p := SystemPrompt(PresetChat, "en")

	if !strings.Contains(p, "get_profile") {
		t.Error("the chat prompt never mentions get_profile, so the agent will ask for what the profile already answers")
	}
}

func TestChatPromptCarriesTheSearchPlaybook(t *testing.T) {
	p := SystemPrompt(PresetChat, "en")

	// Read the vocabulary before filtering — otherwise the model invents facet
	// values and gets a confidently unfiltered result set.
	if !strings.Contains(p, "facets") {
		t.Error("the chat prompt does not tell the agent to read the facet vocabulary first")
	}
}

func TestChatPromptShowsVacanciesOnlyThroughTheTool(t *testing.T) {
	p := SystemPrompt(PresetChat, "en")

	if !strings.Contains(p, "present_jobs") {
		t.Error("the chat prompt does not tell the agent to show vacancies through present_jobs")
	}
	// A link in prose no longer renders as a card, so an instruction to write one
	// would produce exactly the bare link this change exists to remove.
	if strings.Contains(p, "/jobs/") {
		t.Errorf("the chat prompt still asks for a job URL in prose; a vacancy reaches the user only through present_jobs\n%s", p)
	}
}

func TestTailorPromptCarriesTheHonestyRule(t *testing.T) {
	p := SystemPrompt(PresetTailor, "en")

	for _, want := range []string{"missing_have", "missing_gap"} {
		if !strings.Contains(p, want) {
			t.Errorf("the tailoring prompt does not mention %q; the reframe-vs-ask split is the whole safeguard", want)
		}
	}
	if !strings.Contains(strings.ToLower(p), "ask") {
		t.Error("the tailoring prompt must require asking the candidate before writing an unevidenced claim")
	}
}

func TestTailorPromptStatesTheBulletCeilingRule(t *testing.T) {
	p := SystemPrompt(PresetTailor, "en")
	lower := strings.ToLower(p)
	if !strings.Contains(lower, "bullet ceiling") && !strings.Contains(lower, "at most") {
		t.Error("the tailor prompt must tell the model each experience has a bullet ceiling")
	}
	if !strings.Contains(lower, "refused") {
		t.Error("the tailor prompt must say inserting past the cap is refused")
	}
	if !strings.Contains(p, "`set`") || !strings.Contains(p, "`remove`") {
		t.Error("the tailor prompt must tell the model to set or remove before inserting when full")
	}
}

// Agents kept inventing a PROJECTS heading field and parking portfolio work under experience.
// Templates already emit section titles from non-empty arrays; the prompt must say so.
func TestTailorPromptOwnsSectionHeadingsAndProjectsPlacement(t *testing.T) {
	p := SystemPrompt(PresetTailor, "en")
	lower := strings.ToLower(p)
	if !strings.Contains(lower, "section headings") || !strings.Contains(lower, "template") {
		t.Error("the tailor prompt must say section headings are template-owned")
	}
	if !strings.Contains(lower, "do not invent") {
		t.Error("the tailor prompt must forbid inventing heading/title/section fields")
	}
	if !strings.Contains(p, "`projects[]`") || !strings.Contains(p, "projects[i]") {
		t.Error("the tailor prompt must name projects[] / projects[i] for portfolio placement")
	}
	if !strings.Contains(p, "`experience[]`") {
		t.Error("the tailor prompt must contrast projects placement with experience[]")
	}
}

// Three surfaces hand the interviewer an achievement id — the opening message of a
// selection, get_profile's soft_duplicate_clusters, and the id a merge returns — and this
// prompt used to tell it to SEARCH them. Search retrieves by meaning and drops what scores
// zero, so a UUID matched nothing, which the search tool documents as "the bank holds
// nothing on that point". The agent concluded that four achievements the candidate was
// looking at did not exist, and answered about a different set instead.
func TestProfilePromptReadsIdsRatherThanSearchingThem(t *testing.T) {
	p := SystemPrompt(PresetProfile, "en")

	if !strings.Contains(p, "experience_get") {
		t.Fatal("the profile prompt never names experience_get, so nothing turns an id into the achievement it names")
	}
	if strings.Contains(p, "(search them") {
		t.Error("the prompt still tells the agent to search achievement ids; that is the failure this replaces")
	}
	// Knowing the tool exists is not enough — it has to know which of the two takes an id.
	if !strings.Contains(p, "CANNOT find an achievement by its id") {
		t.Error("the prompt does not say experience_search cannot resolve an id, so the agent may still try")
	}
}

// Merging is decided entirely server-side, so an agent that never read the pair can still
// merge it — and cannot make the one judgement a merge needs: whether the two are the same
// work. Updating is narrower and just as destructive, because metrics and skills are set as
// whole lists rather than appended to.
func TestProfilePromptReadsBeforeMergingOrUpdating(t *testing.T) {
	p := SystemPrompt(PresetProfile, "en")
	lower := strings.ToLower(p)

	if !strings.Contains(lower, "never merge or update an achievement you have not read") {
		t.Error("the profile prompt does not require reading an achievement before merging or updating it")
	}
	if !strings.Contains(lower, "replaced whole") {
		t.Error("the prompt does not warn that metrics and skills are replaced whole, so a blind update erases recorded numbers")
	}
}

func TestAnUnknownPresetFallsBackToTheChatPrompt(t *testing.T) {
	if SystemPrompt("wizard", "en") != SystemPrompt(PresetChat, "en") {
		t.Error("an unknown preset must still get a prompt; a session with none would answer unguided")
	}
}

// The candidate opened the panel ON something. The chat playbook this preset
// extends opens with `get_profile` and a "what are you looking for?" — correct on
// the website, wrong in a side panel, where the answer is on the screen behind it.
// The extension must say so explicitly, because it is appended AFTER that
// instruction and the model follows what it read.
func TestBrowsePromptOpensOnThePageNotTheProfile(t *testing.T) {
	p := SystemPrompt(PresetBrowse, "en")

	if !strings.Contains(p, "FIRST thing you do") {
		t.Error("the browse prompt does not override how the conversation opens, so the agent will run the chat playbook's opening and ask what the candidate is looking for")
	}
	// It has to name the instruction it is overriding, or the two read as advice
	// about different moments rather than one replacing the other.
	browseOnly := strings.TrimPrefix(p, chatPrompt)
	if !strings.Contains(browseOnly, "get_profile") {
		t.Error("the browse prompt never mentions get_profile, so nothing tells the agent that the opening it just read does not apply here")
	}
}

// An unattended run is the same method at a different rhythm. What the prompt has to state
// is exactly what a run would otherwise get wrong: keep going, do not ask mid-pass, and
// account for every requirement at the end — including the ones nothing could close.
func TestTailorPromptDescribesTheUnattendedRun(t *testing.T) {
	p := SystemPrompt(PresetTailor, "en")

	for _, want := range []string{"tailor_report", "closed_bank", "closed_candidate", "open", "not_reached"} {
		if !strings.Contains(p, want) {
			t.Errorf("the tailoring prompt never mentions %q; a run cannot report an outcome it was never told exists", want)
		}
	}
	if !strings.Contains(p, "evidence_id") {
		t.Error("the run section must restate that a bullet needs evidence — the wall holds when nobody is watching")
	}
	// One question at the end, not a list: the rest of the remainder is visible beside the CV.
	if !strings.Contains(strings.ToLower(p), "one question") && !strings.Contains(p, "FIRST open one") {
		t.Error("the prompt must ask for ONE closing question; a numbered list of gaps gets one answer")
	}
}

// TestTailorPromptSelfChecksWithJobMatchBeforeReporting: an unattended run must verify its
// own edits against the deterministic job_match score before it reports — the agent
// cannot be trusted to know whether an edit actually reads as closing a requirement (see
// openspec/changes/fit-analysis-post-autopilot-verify/design.md).
func TestTailorPromptSelfChecksWithJobMatchBeforeReporting(t *testing.T) {
	p := SystemPrompt(PresetTailor, "en")

	unattendedIdx := strings.Index(p, "UNATTENDED RUNS")
	if unattendedIdx == -1 {
		t.Fatal("the tailoring prompt lost its UNATTENDED RUNS section")
	}
	unattended := p[unattendedIdx:]

	for _, want := range []string{"job_match", "missing_have", "missing_gap", "tailor_report"} {
		if !strings.Contains(unattended, want) {
			t.Errorf("the unattended-run section never mentions %q; the agent has no instruction to verify its own edits before reporting", want)
		}
	}
	if strings.Index(unattended, "job_match") > strings.Index(unattended, "tailor_report") {
		t.Error("job_match must be checked BEFORE tailor_report, not after — the report should reflect what the check found")
	}
}

// The evidence-citation gate only checks that a bullet cites something real; it says
// nothing about whether the wording stays inside what that evidence actually claims. The
// instruction to check that itself has to sit beside the honesty rule it backs up — not
// merely exist somewhere in the prompt — and has to say what to do about a bad result.
func TestTailorPromptChecksItsOwnWordingAgainstTheEvidence(t *testing.T) {
	p := SystemPrompt(PresetTailor, "en")

	honestyIdx := strings.Index(p, "Never invent, inflate or imply")
	if honestyIdx == -1 {
		t.Fatal("the tailoring prompt lost its honesty-wall paragraph")
	}
	window := p[honestyIdx:min(len(p), honestyIdx+700)]

	if !strings.Contains(window, "check_evidence_fidelity") {
		t.Error("check_evidence_fidelity is not mentioned near the honesty rule, so the agent has no instruction to re-read what it cited")
	}
	if !strings.Contains(window, "cv_edit") {
		t.Error("the prompt never tells the agent to revise with cv_edit after checking fidelity")
	}
	lower := strings.ToLower(window)
	if !strings.Contains(lower, "scope") && !strings.Contains(lower, "seniority") && !strings.Contains(lower, "metric") {
		t.Error("the instruction does not say what kind of overstatement to look for")
	}
}

// An unattended run has no candidate in the loop to notice an overstated bullet either —
// the check matters just as much there, and the section states its other self-checks
// (job_match) explicitly rather than leaving them implied by "the method does not change".
func TestTailorPromptChecksFidelityDuringUnattendedRuns(t *testing.T) {
	p := SystemPrompt(PresetTailor, "en")

	unattendedIdx := strings.Index(p, "UNATTENDED RUNS")
	if unattendedIdx == -1 {
		t.Fatal("the tailoring prompt lost its UNATTENDED RUNS section")
	}
	unattended := p[unattendedIdx:]

	if !strings.Contains(unattended, "check_evidence_fidelity") {
		t.Error("the unattended-run section never mentions check_evidence_fidelity; a run has no server-side backstop for overstated wording either")
	}
}

// Two recorded sessions opened with a long restatement of the fit analysis the candidate had
// open beside the chat, then spent every remaining round searching the bank one requirement at
// a time — and edited nothing. The prompt now says where the evidence already is, and to spend
// rounds on edits rather than on a summary.
func TestTailorPromptSpendsRoundsOnEditsNotSummaries(t *testing.T) {
	p := SystemPrompt(PresetTailor, "en")

	if !strings.Contains(p, "cv_context") || !strings.Contains(p, "evidence") {
		t.Error("the prompt must point the agent at the evidence cv_context already carries")
	}
	lower := strings.ToLower(p)
	if !strings.Contains(lower, "do not restate") && !strings.Contains(lower, "not repeat") {
		t.Error("the prompt must forbid restating the fit analysis — the candidate has it open beside the chat")
	}
	if !strings.Contains(lower, "as you go") && !strings.Contains(lower, "one requirement at a time") {
		t.Error("the prompt must ask for edits as each requirement is closed, not after all the research")
	}
}

// freehire#1837: the assistant must follow the candidate's saved profile language for
// its own words rather than defaulting to English or mirroring whatever language they
// happen to type in.
func TestSystemPromptNamesTheRequestedLanguage(t *testing.T) {
	for _, tc := range []struct {
		code, want string
	}{
		{"ru", "Russian"},
		{"es", "Spanish"},
		{"en", "English"},
		{"xx", "English"}, // unrecognised code falls back rather than dropping the directive
		{"", "English"},
	} {
		p := SystemPrompt(PresetChat, tc.code)
		if !strings.Contains(p, "Reply to the candidate in "+tc.want) {
			t.Errorf("SystemPrompt(PresetChat, %q) does not tell the model to reply in %s\n%s", tc.code, tc.want, p)
		}
	}
}

// Every preset must carry the directive — a candidate should not lose their saved
// language preference just because their session is an interview or a debrief rather
// than the general chat.
func TestEveryPresetCarriesTheLanguageDirective(t *testing.T) {
	for _, preset := range []string{PresetChat, PresetTailor, PresetProfile, PresetBrowse, PresetInterview, PresetDebrief, "unknown"} {
		p := SystemPrompt(preset, "ru")
		if !strings.Contains(p, "Reply to the candidate in Russian") {
			t.Errorf("preset %q does not carry the language directive", preset)
		}
	}
}

// The honest wall in tailorPrompt already tells the agent to write CV bullets in the
// vacancy's own language (see the "reframe it into a bullet in the vacancy's language"
// instruction). The candidate's profile language must govern the assistant's own words
// ONLY — a candidate reading freehire in Russian must not get Russian bullets on an
// English-language CV, so the tailor preset's directive has to state the exception
// explicitly rather than leaving the model to reconcile two separate instructions.
func TestTailorLanguageDirectiveExemptsCVContent(t *testing.T) {
	p := SystemPrompt(PresetTailor, "ru")
	if !strings.Contains(p, "vacancy's own language instead") {
		t.Error("the tailor preset's language directive does not carve out an exception for cv_edit bullets")
	}
	if !strings.Contains(p, "in the vacancy's language") {
		t.Error("the tailor prompt lost its own instruction to write bullets in the vacancy's language")
	}
}

// Every other preset has nothing to exempt — a chat reply and an interview critique are
// both entirely the assistant's own words, so the directive must not carry the tailor
// preset's cv_edit carve-out where there is no cv_edit tool to carve out.
func TestOnlyTailorLanguageDirectiveCarvesOutCVContent(t *testing.T) {
	for _, preset := range []string{PresetChat, PresetProfile, PresetBrowse, PresetInterview, PresetDebrief} {
		p := SystemPrompt(preset, "ru")
		if strings.Contains(p, "cv_edit` follows the vacancy's own language instead") {
			t.Errorf("preset %q carries the tailor-only CV-content exception", preset)
		}
	}
}

func TestLanguageNameFallsBackToEnglish(t *testing.T) {
	for _, code := range []string{"xx", "", "EN", "ru "} {
		if got := LanguageName(code); got != "English" {
			t.Errorf("LanguageName(%q) = %q, want %q", code, got, "English")
		}
	}
	if got := LanguageName("ru"); got != "Russian" {
		t.Errorf(`LanguageName("ru") = %q, want "Russian"`, got)
	}
}
