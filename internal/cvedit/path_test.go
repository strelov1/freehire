package cvedit

import (
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/cv"
)

func TestParsePathAcceptsEveryShapeOfAddress(t *testing.T) {
	for _, want := range []string{
		"title",
		"template_id",
		"summary",
		"style.font_family",
		"style.font_size",
		"margins.left",
		"header.email",
		"header.links[0]",
		"experience[3]",
		"experience[3].bullets[1]",
		"experience[0].stack[2]",
		"education[1].institution",
		"skills[0].items[4]",
		"languages[0].level",
		"projects[2].bullets[0]",
		"certifications[0].issuer",
	} {
		t.Run(want, func(t *testing.T) {
			p, err := ParsePath(want)
			if err != nil {
				t.Fatalf("ParsePath(%q): %v", want, err)
			}
			// The canonical form round-trips: what a revision stores is what a caller wrote.
			if string(p) != want {
				t.Fatalf("ParsePath(%q) = %q, want the input back", want, p)
			}
		})
	}
}

func TestParsePathRejectsWithTheAddressInTheMessage(t *testing.T) {
	tests := []struct {
		name string
		in   string
	}{
		{"unknown top-level field", "salary"},
		{"unknown nested field", "style.colour"},
		{"unknown field under an indexed entry", "experience[0].tenure"},
		{"indexing a field that is not a list", "summary[0]"},
		{"indexing a struct", "style[0]"},
		{"reaching into a scalar", "summary.text"},
		{"unclosed bracket", "experience[0"},
		{"non-numeric index", "experience[first].bullets[0]"},
		{"negative index", "experience[-1]"},
		{"empty index", "experience[]"},
		{"empty path", ""},
		{"trailing dot", "experience[0]."},
		{"leading dot", ".summary"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParsePath(tc.in)
			if err == nil {
				t.Fatalf("ParsePath(%q) succeeded, want a refusal", tc.in)
			}
			// The message names the address, because for a model the error is its only
			// route to correcting itself inside the turn.
			if !strings.Contains(err.Error(), tc.in) {
				t.Fatalf("ParsePath(%q) error %q does not name the path", tc.in, err)
			}
		})
	}
}

func TestResolveRejectsAnIndexPastTheEnd(t *testing.T) {
	state := State{
		Title: "Backend Engineer",
		Document: cv.Document{
			Experience: []cv.ExperienceItem{{Role: "Engineer", Bullets: []string{"Shipped it"}}},
		},
	}

	for _, tc := range []struct{ name, path string }{
		{"entry past the end", "experience[1]"},
		{"bullet past the end", "experience[0].bullets[1]"},
		{"entry past the end of an empty list", "education[0].degree"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p, err := ParsePath(tc.path)
			if err != nil {
				t.Fatalf("ParsePath(%q): %v", tc.path, err)
			}
			if _, err := resolve(&state, p); err == nil {
				t.Fatalf("resolve(%q) succeeded, want a refusal", tc.path)
			} else if !strings.Contains(err.Error(), tc.path) {
				t.Fatalf("resolve(%q) error %q does not name the path", tc.path, err)
			}
		})
	}
}

func TestResolveReachesTheAddressedValue(t *testing.T) {
	state := State{
		Title:      "Backend Engineer",
		TemplateID: "classic-ats",
		Document: cv.Document{
			Summary: "Ten years of Go",
			Style:   cv.Style{FontSize: 10.5},
			Experience: []cv.ExperienceItem{
				{Role: "Engineer", Bullets: []string{"Shipped it", "Twice"}},
			},
		},
	}

	for _, tc := range []struct {
		path string
		want any
	}{
		{"title", "Backend Engineer"},
		{"template_id", "classic-ats"},
		{"summary", "Ten years of Go"},
		{"style.font_size", 10.5},
		{"experience[0].role", "Engineer"},
		{"experience[0].bullets[1]", "Twice"},
	} {
		t.Run(tc.path, func(t *testing.T) {
			p, err := ParsePath(tc.path)
			if err != nil {
				t.Fatalf("ParsePath(%q): %v", tc.path, err)
			}
			v, err := resolve(&state, p)
			if err != nil {
				t.Fatalf("resolve(%q): %v", tc.path, err)
			}
			if got := v.Interface(); got != tc.want {
				t.Fatalf("resolve(%q) = %v, want %v", tc.path, got, tc.want)
			}
		})
	}
}

func TestPathsEnumeratesTheAddressableShapesFromTheStructItself(t *testing.T) {
	got := Paths()

	index := make(map[string]bool, len(got))
	for _, p := range got {
		index[p] = true
	}

	// A model reads this list, so it has to carry the shapes it will actually need —
	// including the ones the old named vocabulary never reached.
	for _, want := range []string{
		"title",
		"template_id",
		"summary",
		"style.font_size",
		"margins.left",
		"header.email",
		"experience[i].bullets[j]",
		"experience[i].stack[j]",
		"skills[i].items[j]",
		"education[i].institution",
		"certifications[i].issuer",
		"languages[i].level",
		"projects[i].bullets[j]",
	} {
		if !index[want] {
			t.Errorf("Paths() omits %q", want)
		}
	}

	// Derived from the struct, so a field added to Document appears here without anyone
	// editing a list: every enumerated shape must parse.
	for _, p := range got {
		concrete := strings.NewReplacer("[i]", "[0]", "[j]", "[0]").Replace(p)
		if _, err := ParsePath(concrete); err != nil {
			t.Errorf("Paths() offers %q, which does not parse as %q: %v", p, concrete, err)
		}
	}
}
