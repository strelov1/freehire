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

func TestSanitizeMargins(t *testing.T) {
	doc := Document{Margins: Margins{Top: 0, Right: 5.0, Bottom: 0.75, Left: -1.0}}
	doc.Sanitize()

	if doc.Margins.Top != 0.5 {
		t.Errorf("unset Top: got %v, want default 0.5", doc.Margins.Top)
	}
	if doc.Margins.Right != 1.5 {
		t.Errorf("over-max Right: got %v, want clamped 1.5", doc.Margins.Right)
	}
	if doc.Margins.Bottom != 0.75 {
		t.Errorf("in-range Bottom: got %v, want unchanged 0.75", doc.Margins.Bottom)
	}
	if doc.Margins.Left != 0.25 {
		t.Errorf("negative Left: got %v, want clamped 0.25", doc.Margins.Left)
	}

	below := Document{Margins: Margins{Top: 0.1, Right: 0.1, Bottom: 0.1, Left: 0.1}}
	below.Sanitize()
	if below.Margins.Top != 0.25 {
		t.Errorf("below-min Top: got %v, want clamped 0.25", below.Margins.Top)
	}
}

// A zero style value means "inherit from the template", so — unlike a margin — it must
// survive Sanitize untouched. Clamping it up to the lower bound would rewrite every CV in
// the database to the minimum type size on its next save, which is why this is its own test
// rather than a case inside TestSanitizeStyle.
func TestSanitizeStyleLeavesUnsetValuesUnset(t *testing.T) {
	doc := Document{Style: Style{FontSize: 0, LineHeight: 0}}
	doc.Sanitize()

	if doc.Style.FontSize != 0 {
		t.Errorf("unset FontSize: got %v, want 0 (inherit from the template)", doc.Style.FontSize)
	}
	if doc.Style.LineHeight != 0 {
		t.Errorf("unset LineHeight: got %v, want 0 (inherit from the template)", doc.Style.LineHeight)
	}
}

func TestSanitizeStyle(t *testing.T) {
	tests := []struct {
		name                  string
		in                    Style
		wantSize, wantLeading float64
	}{
		{"over-max font size", Style{FontSize: 30.0}, 12.0, 0},
		{"below-min font size", Style{FontSize: 4.0}, 8.5, 0},
		{"font size rounds to the nearest half point", Style{FontSize: 10.3}, 10.5, 0},
		{"in-range font size is left alone", Style{FontSize: 11.0}, 11.0, 0},
		{"negative font size clamps to the minimum", Style{FontSize: -2.0}, 8.5, 0},
		{"over-max line height", Style{LineHeight: 2.0}, 0, 0.9},
		{"below-min line height", Style{LineHeight: 0.05}, 0, 0.3},
		{"in-range line height is left alone", Style{LineHeight: 0.55}, 0, 0.55},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := Document{Style: tt.in}
			doc.Sanitize()
			if doc.Style.FontSize != tt.wantSize {
				t.Errorf("FontSize: got %v, want %v", doc.Style.FontSize, tt.wantSize)
			}
			if doc.Style.LineHeight != tt.wantLeading {
				t.Errorf("LineHeight: got %v, want %v", doc.Style.LineHeight, tt.wantLeading)
			}
		})
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
