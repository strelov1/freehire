package handler

import (
	"slices"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/assistant"
)

// presetAPI wires enough of the API for a registry to be built.
func presetAPI() *assistantHandlers {
	return &assistantHandlers{
		search: &searchHandlers{search: &recordingSearcher{}, descriptions: fixedDescriptions{}, facets: &stubFacets{}},
		resume: &resumeHandlers{facets: &stubFacets{}},
		cv:     &cvHandlers{},
	}
}

func TestChatPresetOffersDiscoveryAndTrackingTools(t *testing.T) {
	reg := presetAPI().registry(assistant.Session{UserID: 3, Preset: assistant.PresetChat}, uuid.New())

	for _, want := range []string{"facets", "search_jobs", "get_job", "get_company", "market_fit",
		"present_jobs", "save_job", "unsave_job", "apply_job", "track_job", "my_jobs"} {
		if !slices.Contains(reg.Names(), want) {
			t.Errorf("chat preset is missing the %q tool; registered: %v", want, reg.Names())
		}
	}
}

// A tailoring session still recommends vacancies — the workspace's chat is the
// same chat. Without present_jobs it would have no way to show one at all, since
// a job link in prose is no longer rendered as a card.
func TestTailorPresetCanAlsoPresentJobs(t *testing.T) {
	cvID, jobID := uuid.MustParse("66666666-6666-4666-8666-666666666666"), int64(9)
	reg := presetAPI().registry(assistant.Session{
		UserID: 3, Preset: assistant.PresetTailor, CVID: &cvID, JobID: &jobID,
	}, uuid.New())

	if !slices.Contains(reg.Names(), "present_jobs") {
		t.Errorf("tailor preset is missing present_jobs; registered: %v", reg.Names())
	}
}

// The panel's agent is the only one with a browser on the other end of the relay.
func TestBrowsePresetOffersThePageTool(t *testing.T) {
	reg := presetAPI().registry(assistant.Session{UserID: 3, Preset: assistant.PresetBrowse}, uuid.New())

	if !slices.Contains(reg.Names(), "read_current_page") {
		t.Errorf("browse preset is missing read_current_page; registered: %v", reg.Names())
	}
	// It is still the job-search assistant — the page is an addition, not a swap.
	if !slices.Contains(reg.Names(), "search_jobs") {
		t.Errorf("browse preset lost the discovery tools; registered: %v", reg.Names())
	}
}

// A session held anywhere but the panel has no page to read. A tool that can only
// fail spends the model's context and teaches it to stop calling tools.
func TestOnlyTheBrowsePresetOffersThePageTool(t *testing.T) {
	cvID, jobID := uuid.MustParse("66666666-6666-4666-8666-666666666666"), int64(9)

	for _, sess := range []assistant.Session{
		{UserID: 3, Preset: assistant.PresetChat},
		{UserID: 3, Preset: assistant.PresetTailor, CVID: &cvID, JobID: &jobID},
	} {
		reg := presetAPI().registry(sess, uuid.New())
		if slices.Contains(reg.Names(), "read_current_page") {
			t.Errorf("preset %q offers read_current_page, but has no browser to read", sess.Preset)
		}
	}
}

// effectivePreset is the one place that decides whether a browse session is running
// over the extension's own connection. Both the prompt and the tool set are built
// from its answer — never from sess.Preset directly — or the two could disagree
// about what preset actually governs the turn.
func TestEffectivePresetDemotesBrowseWithoutTheExtension(t *testing.T) {
	if got := effectivePreset(assistant.PresetBrowse, false); got != assistant.PresetChat {
		t.Errorf("effectivePreset(browse, asExtension=false) = %q, want chat", got)
	}
	if got := effectivePreset(assistant.PresetBrowse, true); got != assistant.PresetBrowse {
		t.Errorf("effectivePreset(browse, asExtension=true) = %q, want browse", got)
	}
}

func TestEffectivePresetLeavesOtherPresetsAlone(t *testing.T) {
	for _, preset := range []string{assistant.PresetChat, assistant.PresetTailor, assistant.PresetProfile, assistant.PresetInterview, assistant.PresetDebrief} {
		for _, asExtension := range []bool{true, false} {
			if got := effectivePreset(preset, asExtension); got != preset {
				t.Errorf("effectivePreset(%q, asExtension=%v) = %q, want it unchanged", preset, asExtension, got)
			}
		}
	}
}

// The website authenticates by cookie; the extension by Bearer session JWT (never an
// API key — a key is never the extension's own credential either). Preset alone is
// not enough — a browse session reached any other way (a stale rail entry, a direct
// URL, an API key) must degrade to an ordinary chat, PROMPT AND TOOLS BOTH, rather
// than expose the one tool that only works over the extension's connection while
// still being told to call it first. See the confine-browse-preset-to-extension
// change.
func TestBrowsePresetOverCookieRunsAsAnOrdinaryChat(t *testing.T) {
	resolved := effectivePreset(assistant.PresetBrowse, false)

	reg := presetAPI().registry(assistant.Session{UserID: 3, Preset: resolved}, uuid.New())
	if slices.Contains(reg.Names(), "read_current_page") {
		t.Error("a demoted browse session still offers read_current_page")
	}
	// It still degrades to an ordinary, working chat rather than losing everything.
	if !slices.Contains(reg.Names(), "search_jobs") {
		t.Errorf("a demoted browse session lost the discovery tools too; registered: %v", reg.Names())
	}

	if assistant.SystemPrompt(resolved, "en") != assistant.SystemPrompt(assistant.PresetChat, "en") {
		t.Error("a demoted browse session must run under the chat prompt, not one that opens by " +
			"insisting on a tool it was never given")
	}
}

func TestChatPresetHasNoCVTools(t *testing.T) {
	reg := presetAPI().registry(assistant.Session{UserID: 3, Preset: assistant.PresetChat}, uuid.New())

	for _, name := range reg.Names() {
		if strings.HasPrefix(name, "cv_") {
			t.Errorf("chat session offers %q; CV editing belongs to a tailoring session only", name)
		}
	}
}

func TestTailorPresetAddsTheCVTools(t *testing.T) {
	cvID, jobID := uuid.MustParse("66666666-6666-4666-8666-666666666666"), int64(9)
	reg := presetAPI().registry(assistant.Session{
		UserID: 3, Preset: assistant.PresetTailor, CVID: &cvID, JobID: &jobID,
	}, uuid.New())

	for _, want := range []string{"cv_context", "cv_get", "cv_edit", "search_jobs"} {
		if !slices.Contains(reg.Names(), want) {
			t.Errorf("tailor preset is missing the %q tool; registered: %v", want, reg.Names())
		}
	}
}

func TestTailorPresetWithoutABindingHasNoCVTools(t *testing.T) {
	// A tailoring session whose CV was deleted must degrade to a plain chat rather
	// than register CV tools bound to a zero id.
	reg := presetAPI().registry(assistant.Session{UserID: 3, Preset: assistant.PresetTailor}, uuid.New())

	for _, name := range reg.Names() {
		if strings.HasPrefix(name, "cv_") {
			t.Errorf("unbound tailoring session offers %q", name)
		}
	}
}

func TestNoModeratorToolIsEverRegistered(t *testing.T) {
	// Job authoring and submission review are moderator surfaces; the agent must
	// not reach them whatever the session or the caller's role.
	forbidden := []string{"create_job", "edit_job", "submit_job", "submissions", "approve_submission", "reject_submission"}
	cvID, jobID := uuid.MustParse("66666666-6666-4666-8666-666666666666"), int64(9)

	for _, sess := range []assistant.Session{
		{UserID: 3, Preset: assistant.PresetChat},
		{UserID: 3, Preset: assistant.PresetTailor, CVID: &cvID, JobID: &jobID},
	} {
		reg := presetAPI().registry(sess, uuid.New())
		for _, name := range reg.Names() {
			if slices.Contains(forbidden, name) {
				t.Errorf("preset %q registers the moderator tool %q", sess.Preset, name)
			}
		}
	}
}

// A debrief binds to an application the caller already owns, so a client may name it
// the same way it names a rehearsal. Tailoring stays out: its binding is a CV that does
// not exist until the tailoring bootstrap makes one.
func TestCreatablePresetsAdmitADebriefAndStillRefuseTailoring(t *testing.T) {
	for _, asked := range []string{assistant.PresetDebrief, assistant.PresetInterview, assistant.PresetProfile, assistant.PresetBrowse, assistant.PresetChat} {
		got, err := creatablePreset(asked)
		if err != nil {
			t.Errorf("creatablePreset(%q) refused a preset a client may mint: %v", asked, err)
		}
		if got != asked {
			t.Errorf("creatablePreset(%q) = %q", asked, got)
		}
	}
	if _, err := creatablePreset(assistant.PresetTailor); err == nil {
		t.Error("creatablePreset admitted tailoring, whose binding this endpoint cannot supply")
	}
}

// The rejection names what a client MAY ask for, because this endpoint serves the web
// app and the extension both and a bare 400 leaves the author guessing. A preset added
// to the switch and left out of the message is the failure this pins shut.
func TestTheRefusalNamesEveryCreatablePreset(t *testing.T) {
	_, err := creatablePreset("rehearsal")
	if err == nil {
		t.Fatal("creatablePreset admitted an unknown preset")
	}
	for _, want := range []string{assistant.PresetChat, assistant.PresetProfile, assistant.PresetBrowse, assistant.PresetInterview, assistant.PresetDebrief} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("the refusal never names %q as a preset a client may create: %s", want, err)
		}
	}
}

// The brief is what makes the agent speak first, and it is chosen server-side so a
// client cannot dictate what the session is told to do. A debrief opened under the
// rehearsal's brief would ask the candidate which round to rehearse — for an interview
// they have already sat.
func TestTheOpeningBriefFollowsThePreset(t *testing.T) {
	rehearsal := openingBriefFor(assistant.PresetInterview)
	debrief := openingBriefFor(assistant.PresetDebrief)

	if rehearsal == "" || debrief == "" {
		t.Fatal("an application-bound preset opens with no brief, so the agent never speaks first")
	}
	if rehearsal == debrief {
		t.Error("the debrief opens under the rehearsal's brief, which asks which round to rehearse")
	}
	if !strings.Contains(strings.ToLower(debrief), "asked") {
		t.Errorf("the debrief's brief does not ask what the candidate was asked: %q", debrief)
	}
}

// A debrief and a rehearsal reason from the same material — the vacancy, the stage, the
// requirements with the bank's evidence — so they carry the same tools, and the prompt
// is the whole of the difference. This pins that: a tool added to one and forgotten in
// the other is a debrief that cannot read the interview it is reviewing.
func TestTheDebriefCarriesTheRehearsalsTools(t *testing.T) {
	jobID := int64(9)
	rehearsal := presetAPI().registry(assistant.Session{
		UserID: 3, Preset: assistant.PresetInterview, JobID: &jobID,
	}, uuid.New())
	debrief := presetAPI().registry(assistant.Session{
		UserID: 3, Preset: assistant.PresetDebrief, JobID: &jobID,
	}, uuid.New())

	want, got := rehearsal.Names(), debrief.Names()
	slices.Sort(want)
	slices.Sort(got)
	if !slices.Equal(want, got) {
		t.Errorf("the debrief's tools differ from the rehearsal's:\n debrief: %v\n rehearsal: %v", got, want)
	}
	if !slices.Contains(got, "interview_context") {
		t.Errorf("the debrief cannot read the application it is reviewing; registered: %v", got)
	}
}

// Without a vacancy there is nothing to review, so the context tool would be bound to
// nothing — the same rule a CV-less tailoring session follows.
func TestADebriefWithoutAVacancyHasNoContextTool(t *testing.T) {
	reg := presetAPI().registry(assistant.Session{UserID: 3, Preset: assistant.PresetDebrief}, uuid.New())

	if slices.Contains(reg.Names(), "interview_context") {
		t.Errorf("an unbound debrief offers interview_context, which points at no vacancy")
	}
}

// The invitation reaches a debrief through its context, carrying its untrusted marking.
// Handing it mail tools as well would give a prompt injection an outbound channel.
func TestTheDebriefCannotReachTheMailbox(t *testing.T) {
	jobID := int64(9)
	reg := presetAPI().registry(assistant.Session{
		UserID: 3, Preset: assistant.PresetDebrief, JobID: &jobID,
	}, uuid.New())

	for _, name := range reg.Names() {
		if strings.HasPrefix(name, "inbox_") {
			t.Errorf("the debrief offers the mail tool %q", name)
		}
	}
}

func TestRegistryCarriesTheResultCap(t *testing.T) {
	reg := presetAPI().registry(assistant.Session{UserID: 3, Preset: assistant.PresetChat}, uuid.New())
	if reg.MaxResultBytes <= 0 {
		t.Error("the registry has no result cap; one search over full descriptions can fill the context window")
	}
}
