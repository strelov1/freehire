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

func TestRegistryCarriesTheResultCap(t *testing.T) {
	reg := presetAPI().registry(assistant.Session{UserID: 3, Preset: assistant.PresetChat}, uuid.New())
	if reg.MaxResultBytes <= 0 {
		t.Error("the registry has no result cap; one search over full descriptions can fill the context window")
	}
}
