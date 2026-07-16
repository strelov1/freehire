package cv

import (
	"strings"
	"testing"
)

func TestSanitizeBoundsStrings(t *testing.T) {
	doc := Document{
		Header: Header{
			FullName: strings.Repeat("a", maxNameRunes+50),
			Email:    strings.Repeat("c", maxEmailRunes+50),
			Phone:    strings.Repeat("d", maxPhoneRunes+50),
			Location: strings.Repeat("e", maxLocationRunes+50),
		},
		Summary: strings.Repeat("f", maxSummaryRunes+50),
	}
	doc.Sanitize()

	if got := len([]rune(doc.Header.FullName)); got > maxNameRunes {
		t.Errorf("FullName not bounded: %d runes", got)
	}
	if got := len([]rune(doc.Header.Email)); got > maxEmailRunes {
		t.Errorf("Email not bounded: %d runes", got)
	}
	if got := len([]rune(doc.Header.Phone)); got > maxPhoneRunes {
		t.Errorf("Phone not bounded: %d runes", got)
	}
	if got := len([]rune(doc.Header.Location)); got > maxLocationRunes {
		t.Errorf("Location not bounded: %d runes", got)
	}
	if got := len([]rune(doc.Summary)); got > maxSummaryRunes {
		t.Errorf("Summary not bounded: %d runes", got)
	}
}

func TestSanitizeCapsArrays(t *testing.T) {
	doc := Document{}
	for i := 0; i < maxExperience+10; i++ {
		doc.Experience = append(doc.Experience, ExperienceItem{Role: "eng"})
	}
	for i := 0; i < maxSkillGroups+10; i++ {
		doc.Skills = append(doc.Skills, SkillGroup{Group: "g", Items: []string{"go"}})
	}
	bullets := make([]string, maxBullets+10)
	for i := range bullets {
		bullets[i] = "did a thing"
	}
	doc.Experience = append(doc.Experience, ExperienceItem{Role: "eng", Bullets: bullets})

	doc.Sanitize()

	if len(doc.Experience) > maxExperience {
		t.Errorf("Experience not capped: %d", len(doc.Experience))
	}
	if len(doc.Skills) > maxSkillGroups {
		t.Errorf("Skills not capped: %d", len(doc.Skills))
	}
	for _, e := range doc.Experience {
		if len(e.Bullets) > maxBullets {
			t.Errorf("Bullets not capped: %d", len(e.Bullets))
		}
	}
}

func TestSanitizeDropsEmptyEntries(t *testing.T) {
	doc := Document{
		Experience: []ExperienceItem{
			{Role: "eng", Company: "acme"},
			{}, // wholly empty → dropped
		},
		Skills: []SkillGroup{
			{Group: "lang", Items: []string{"go", "", "  "}},
			{}, // empty → dropped
		},
		Languages: []Language{
			{Name: "English", Level: "C1"},
			{}, // empty → dropped
		},
	}
	doc.Sanitize()

	if len(doc.Experience) != 1 {
		t.Errorf("empty experience not dropped: %d", len(doc.Experience))
	}
	if len(doc.Skills) != 1 {
		t.Errorf("empty skill group not dropped: %d", len(doc.Skills))
	}
	if items := doc.Skills[0].Items; len(items) != 1 {
		t.Errorf("blank skill items not dropped: %v", items)
	}
	if len(doc.Languages) != 1 {
		t.Errorf("empty language not dropped: %d", len(doc.Languages))
	}
}

func TestEmptyDocumentIsSanitizeStable(t *testing.T) {
	doc := EmptyDocument()
	before := doc
	doc.Sanitize()
	if !equalDocument(before, doc) {
		t.Errorf("EmptyDocument mutated by Sanitize:\nbefore=%+v\nafter=%+v", before, doc)
	}
}

// equalDocument is a shallow structural compare good enough for the empty case
// (all slices nil, all strings empty).
func equalDocument(a, b Document) bool {
	return a.Header.FullName == b.Header.FullName &&
		a.Header.Email == b.Header.Email &&
		a.Header.Phone == b.Header.Phone &&
		a.Header.Location == b.Header.Location &&
		len(a.Header.Links) == len(b.Header.Links) &&
		a.Summary == b.Summary &&
		len(a.Experience) == len(b.Experience) &&
		len(a.Education) == len(b.Education) &&
		len(a.Skills) == len(b.Skills) &&
		len(a.Languages) == len(b.Languages) &&
		len(a.Projects) == len(b.Projects) &&
		len(a.Certifications) == len(b.Certifications)
}
