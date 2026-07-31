package cvedit

import (
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/cv"
)

func TestDescribeNamesTheChangeInProse(t *testing.T) {
	before := sample()

	tests := []struct {
		name string
		ops  []Op
		want string
	}{
		{"the summary", []Op{{Kind: OpSet, Path: "summary"}}, "Rewrote the summary"},
		{"the title", []Op{{Kind: OpSet, Path: "title"}}, "Renamed the CV"},
		{"the template", []Op{{Kind: OpSet, Path: "template_id", Value: "sidebar"}}, "Switched to the sidebar template"},
		{"typography", []Op{{Kind: OpSet, Path: "style.font_size"}}, "Changed the typography"},
		{"margins", []Op{{Kind: OpSet, Path: "margins.left"}}, "Changed the page margins"},
		// An address is named the way the candidate sees it on the page, not as a path.
		{"a bullet", []Op{{Kind: OpSet, Path: "experience[0].bullets[1]"}}, "Rewrote a bullet in Engineer, Acme"},
		{"a new bullet", []Op{{Kind: OpInsert, Path: "experience[1].bullets[0]"}}, "Added a bullet in Junior, Initech"},
		{"a deleted bullet", []Op{{Kind: OpRemove, Path: "experience[0].bullets[0]"}}, "Removed a bullet in Engineer, Acme"},
		{"a moved bullet", []Op{{Kind: OpMove, Path: "experience[0].bullets[1]"}}, "Moved a bullet in Engineer, Acme"},
		{"the technologies", []Op{{Kind: OpSet, Path: "experience[0].stack[0]"}}, "Changed the technologies in Engineer, Acme"},
		{"an entry's description", []Op{{Kind: OpSet, Path: "experience[0].summary"}}, "Rewrote the description of Engineer, Acme"},
		{"a whole entry", []Op{{Kind: OpRemove, Path: "experience[0]"}}, "Removed an experience entry"},
		{"the skills", []Op{{Kind: OpSet, Path: "skills[0].items[1]"}}, "Changed the skills"},
		{"an education entry", []Op{{Kind: OpInsert, Path: "education[0]"}}, "Added an education entry"},
		{"a certification", []Op{{Kind: OpInsert, Path: "certifications[0]"}}, "Added a certification"},
		{"a language", []Op{{Kind: OpRemove, Path: "languages[0]"}}, "Removed a language"},
		{"nothing", nil, "No change"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := Describe(tc.ops, before, before); got != tc.want {
				t.Fatalf("Describe = %q, want %q", got, tc.want)
			}
		})
	}
}

// Several edits in one place read as that place, not as a list of paths.
func TestDescribeFoldsABatchInOneEntry(t *testing.T) {
	before := sample()
	got := Describe([]Op{
		{Kind: OpSet, Path: "experience[0].bullets[0]"},
		{Kind: OpSet, Path: "experience[0].bullets[1]"},
		{Kind: OpSet, Path: "experience[0].stack[0]"},
	}, before, before)

	if got != "Edited 3 things in Engineer, Acme" {
		t.Fatalf("Describe = %q, want the entry named once", got)
	}
}

func TestDescribeFoldsABatchAcrossPlaces(t *testing.T) {
	before := sample()
	got := Describe([]Op{
		{Kind: OpSet, Path: "summary"},
		{Kind: OpSet, Path: "experience[1].bullets[0]"},
	}, before, before)

	if got != "Edited 2 places" {
		t.Fatalf("Describe = %q, want a count of places", got)
	}
}

// A removed entry is named from the state BEFORE the change: after it, there is nothing at
// that address to read a name from.
func TestDescribeNamesAnEntryThatIsNoLongerThere(t *testing.T) {
	before := sample()
	after, _, err := Apply(before, []Op{{Kind: OpRemove, Path: "experience[1].bullets[0]"}})
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}

	got := Describe([]Op{{Kind: OpRemove, Path: "experience[1].bullets[0]"}}, before, after)
	if !strings.Contains(got, "Initech") {
		t.Fatalf("Describe = %q, want the entry it was removed from named", got)
	}
}

// A field the description table has not been taught still reads as a sentence rather than as
// a path, so adding a field to the document cannot produce a blank line in the feed.
func TestDescribeFallsBackToASentence(t *testing.T) {
	got := Describe([]Op{{Kind: OpSet, Path: "certifications[0].issuer"}}, sample(), sample())

	if strings.Contains(got, "[") || strings.Contains(got, "_") {
		t.Fatalf("Describe = %q, want prose rather than a path", got)
	}
	if got == "" {
		t.Fatal("Describe returned nothing")
	}
}

// Every shape the document can present must produce something readable — the feed has no
// fallback of its own, and a blank entry is worse than a clumsy one.
func TestDescribeAnswersForEveryAddressableShape(t *testing.T) {
	state := State{Document: cv.Document{
		Experience:     []cv.ExperienceItem{{Role: "Eng", Company: "Acme", Bullets: []string{"x"}, Stack: []string{"Go"}}},
		Education:      []cv.EducationItem{{Institution: "TU"}},
		Skills:         []cv.SkillGroup{{Group: "Languages", Items: []string{"Go"}}},
		Languages:      []cv.Language{{Name: "English"}},
		Projects:       []cv.Project{{Name: "freehire", Bullets: []string{"x"}}},
		Certifications: []cv.Certification{{Name: "CKA"}},
	}}

	for _, shape := range Paths() {
		concrete := strings.NewReplacer("[i]", "[0]", "[j]", "[0]").Replace(shape)
		got := Describe([]Op{{Kind: OpSet, Path: Path(concrete)}}, state, state)
		if strings.TrimSpace(got) == "" {
			t.Errorf("Describe(%q) is empty", concrete)
		}
	}
}
