package handler

import (
	"slices"
	"strings"
	"testing"

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
	reg := presetAPI().registry(assistant.Session{UserID: 3, Preset: assistant.PresetChat})

	for _, want := range []string{"facets", "search_jobs", "get_job", "get_company", "market_fit",
		"save_job", "unsave_job", "apply_job", "track_job", "my_jobs"} {
		if !slices.Contains(reg.Names(), want) {
			t.Errorf("chat preset is missing the %q tool; registered: %v", want, reg.Names())
		}
	}
}

func TestChatPresetHasNoCVTools(t *testing.T) {
	reg := presetAPI().registry(assistant.Session{UserID: 3, Preset: assistant.PresetChat})

	for _, name := range reg.Names() {
		if strings.HasPrefix(name, "cv_") {
			t.Errorf("chat session offers %q; CV editing belongs to a tailoring session only", name)
		}
	}
}

func TestTailorPresetAddsTheCVTools(t *testing.T) {
	cvID, jobID := int64(5), int64(9)
	reg := presetAPI().registry(assistant.Session{
		UserID: 3, Preset: assistant.PresetTailor, CVID: &cvID, JobID: &jobID,
	})

	for _, want := range []string{"cv_context", "cv_get", "cv_edit", "search_jobs"} {
		if !slices.Contains(reg.Names(), want) {
			t.Errorf("tailor preset is missing the %q tool; registered: %v", want, reg.Names())
		}
	}
}

func TestTailorPresetWithoutABindingHasNoCVTools(t *testing.T) {
	// A tailoring session whose CV was deleted must degrade to a plain chat rather
	// than register CV tools bound to a zero id.
	reg := presetAPI().registry(assistant.Session{UserID: 3, Preset: assistant.PresetTailor})

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
	cvID, jobID := int64(5), int64(9)

	for _, sess := range []assistant.Session{
		{UserID: 3, Preset: assistant.PresetChat},
		{UserID: 3, Preset: assistant.PresetTailor, CVID: &cvID, JobID: &jobID},
	} {
		reg := presetAPI().registry(sess)
		for _, name := range reg.Names() {
			if slices.Contains(forbidden, name) {
				t.Errorf("preset %q registers the moderator tool %q", sess.Preset, name)
			}
		}
	}
}

func TestRegistryCarriesTheResultCap(t *testing.T) {
	reg := presetAPI().registry(assistant.Session{UserID: 3, Preset: assistant.PresetChat})
	if reg.MaxResultBytes <= 0 {
		t.Error("the registry has no result cap; one search over full descriptions can fill the context window")
	}
}
