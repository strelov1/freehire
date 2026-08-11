package handler

import (
	"testing"

	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/cvedit"
)

func TestApplySeedContentPreservesPresentation(t *testing.T) {
	keep := cvedit.State{
		Title:      "Tailored for Acme",
		TemplateID: "compact",
		Document: cv.Document{
			Margins: cv.Margins{Top: 0.75, Right: 0.6, Bottom: 0.75, Left: 0.6},
			Style:   cv.Style{FontFamily: "tinos", FontSize: 11, LineHeight: 0.55},
			Header:  cv.Header{FullName: "Old Name"},
			Summary: "old summary",
		},
	}
	seeded := cv.Document{
		Header:  cv.Header{FullName: "Ada Lovelace", Email: "ada@example.com"},
		Summary: "new summary from résumé",
		Skills:  []cv.SkillGroup{{Items: []string{"Go"}}},
	}

	got := applySeedContent(keep, seeded)

	if got.Title != "Tailored for Acme" || got.TemplateID != "compact" {
		t.Fatalf("title/template = %q/%q, want preserved", got.Title, got.TemplateID)
	}
	if got.Margins != keep.Margins || got.Style != keep.Style {
		t.Fatalf("presentation not preserved: margins=%+v style=%+v", got.Margins, got.Style)
	}
	if got.Header.FullName != "Ada Lovelace" || got.Summary != "new summary from résumé" {
		t.Fatalf("content not applied: header=%+v summary=%q", got.Header, got.Summary)
	}
	if len(got.Skills) != 1 || len(got.Skills[0].Items) != 1 || got.Skills[0].Items[0] != "Go" {
		t.Fatalf("skills = %+v, want seeded Go", got.Skills)
	}
}

func TestApplySeedContentEqualYieldsNoDiff(t *testing.T) {
	seeded := cv.Document{
		Header:  cv.Header{FullName: "Ada"},
		Summary: "same",
	}
	keep := cvedit.State{
		Title:      "My CV",
		TemplateID: cv.DefaultTemplateID,
		Document: cv.Document{
			Margins: cv.DefaultMargins(),
			Header:  seeded.Header,
			Summary: seeded.Summary,
		},
	}
	next := applySeedContent(keep, seeded)
	// Sanitizer runs inside CommitDocument; Diff here checks the structural equality the
	// helper is responsible for before that step.
	if ops := cvedit.Diff(keep, next); len(ops) != 0 {
		t.Fatalf("Diff = %+v, want empty when only seed content already matches", ops)
	}
}

func TestApplySeedContentPreservesEmptySeedContacts(t *testing.T) {
	keep := cvedit.State{
		Title:      "My CV",
		TemplateID: cv.DefaultTemplateID,
		Document: cv.Document{
			Header: cv.Header{
				FullName: "Ada Lovelace",
				Email:    "ada@example.com",
				Phone:    "+351 900 000 000",
				Location: "Lisbon, PT",
				Links:    []string{"github.com/ada"},
			},
			Summary: "old",
			Experience: []cv.ExperienceItem{{
				Role: "Old Role", Company: "OldCo",
			}},
		},
	}
	seeded := cv.Document{
		Header:  cv.Header{}, // empty contacts — must not blank the keep header
		Summary: "new summary",
		Experience: []cv.ExperienceItem{{
			Role: "SWE", Company: "RingCentral", Bullets: []string{"Shipped X"},
		}},
	}

	got := applySeedContent(keep, seeded)
	if got.Header.FullName != keep.Header.FullName || got.Header.Email != keep.Header.Email ||
		got.Header.Phone != keep.Header.Phone || got.Header.Location != keep.Header.Location ||
		len(got.Header.Links) != 1 || got.Header.Links[0] != keep.Header.Links[0] {
		t.Fatalf("header = %+v, want keep's contacts preserved", got.Header)
	}
	if got.Summary != "new summary" {
		t.Fatalf("summary = %q, want seeded body", got.Summary)
	}
	if len(got.Experience) != 1 || got.Experience[0].Company != "RingCentral" {
		t.Fatalf("experience = %+v, want seeded body", got.Experience)
	}
}

func TestApplySeedContentNonEmptySeedPhoneReplaces(t *testing.T) {
	keep := cvedit.State{
		Document: cv.Document{
			Header: cv.Header{FullName: "Ada", Phone: "+351 111 111 111", Email: "ada@example.com"},
		},
	}
	seeded := cv.Document{
		Header: cv.Header{FullName: "Ada Lovelace", Phone: "+351 900 000 000"},
	}

	got := applySeedContent(keep, seeded)
	if got.Header.Phone != "+351 900 000 000" {
		t.Fatalf("phone = %q, want seed's phone", got.Header.Phone)
	}
	if got.Header.FullName != "Ada Lovelace" {
		t.Fatalf("full_name = %q, want seed's name", got.Header.FullName)
	}
	if got.Header.Email != "ada@example.com" {
		t.Fatalf("email = %q, want keep's email (seed left it empty)", got.Header.Email)
	}
}

// A blank keep header must take every contact the seed carries — reset/bootstrap onto an
// empty or newly created CV is how first-time contacts land, same as skills.
func TestApplySeedContentFillsEmptyHeaderFromSeed(t *testing.T) {
	keep := cvedit.State{
		Title:      "My CV",
		TemplateID: cv.DefaultTemplateID,
		Document:   cv.Document{Header: cv.Header{}},
	}
	seeded := cv.Document{
		Header: cv.Header{
			FullName: "Ada Lovelace",
			Email:    "ada@example.com",
			Phone:    "+351 900 000 000",
			Location: "Lisbon, PT",
			Links:    []string{"github.com/ada"},
		},
		Skills: []cv.SkillGroup{{Items: []string{"Go"}}},
	}

	got := applySeedContent(keep, seeded)
	if got.Header.FullName != "Ada Lovelace" || got.Header.Email != "ada@example.com" ||
		got.Header.Phone != "+351 900 000 000" || got.Header.Location != "Lisbon, PT" ||
		len(got.Header.Links) != 1 || got.Header.Links[0] != "github.com/ada" {
		t.Fatalf("header = %+v, want full contacts from seed", got.Header)
	}
	if len(got.Skills) != 1 || len(got.Skills[0].Items) != 1 || got.Skills[0].Items[0] != "Go" {
		t.Fatalf("skills = %+v, want seeded Go", got.Skills)
	}
}
