package jobderive

import "testing"

func TestDerive_IsTech(t *testing.T) {
	tests := []struct {
		name string
		in   Input
		want *bool // nil = unknown
	}{
		{
			name: "recognized tech category via title → true",
			in:   Input{Title: "Senior Backend Developer"},
			want: boolp(true),
		},
		{
			name: "blacklist non-tech category via title → false",
			in:   Input{Title: "Sales Manager"},
			want: boolp(false),
		},
		{
			name: "detector-only non-tech title → false",
			in:   Input{Title: "Warehouse Janitorial Cleaner"},
			want: boolp(false),
		},
		{
			name: "unresolved title stays unknown → nil",
			in:   Input{Title: "Yard Coordinator"},
			want: nil,
		},
		{
			name: "tech wins over a non-tech noun in the same title",
			in:   Input{Title: "Backend Engineer, Nurse Scheduling Platform"},
			want: boolp(true),
		},
		{
			name: "detector-only tech title (no category) → true",
			in:   Input{Title: "COBOL Programmer"},
			want: boolp(true),
		},
		{
			// The second mining wave anchors the named physical disciplines, so these
			// leave the unclassified mass instead of sitting in it forever.
			name: "named non-software discipline → false",
			in:   Input{Title: "Senior Mechanical Engineer"},
			want: boolp(false),
		},
		{
			// A discipline neither dictionary names still stays unknown rather than
			// being coerced: the tech detector is software-anchored and the non-tech
			// one carries no bare "engineer".
			name: "unnamed non-software engineer stays unknown → nil",
			in:   Input{Title: "Drainage Engineer"},
			want: nil,
		},
		{
			// Engineering draughting is its own non-technical category, so it reads
			// false where it used to inherit true from the `design` category.
			name: "engineering design category → false",
			in:   Input{Title: "Mechanical Design Engineer"},
			want: boolp(false),
		},
		{
			// The product-design side of the split stays technical.
			name: "product design category → true",
			in:   Input{Title: "Senior Product Designer"},
			want: boolp(true),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Derive(tt.in).IsTech
			if !eqBoolp(got, tt.want) {
				t.Errorf("IsTech = %s, want %s", showBoolp(got), showBoolp(tt.want))
			}
		})
	}
}

func boolp(b bool) *bool { return &b }

func eqBoolp(a, b *bool) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func showBoolp(b *bool) string {
	if b == nil {
		return "nil"
	}
	if *b {
		return "true"
	}
	return "false"
}

// TestDerive_IsTech_MarketingAliasesDoNotClaimTech pins the technical titles that
// the marketing title aliases sit next to. A bare discipline noun added to the
// category dictionary ("growth", "content", "geo") would resolve these to
// `marketing` — a NonTechCategories member — flipping is_tech to false and taking
// them off the enrichment and embedding budgets. Every marketing alias is a phrase
// so that cannot happen; this test is the tripwire.
func TestDerive_IsTech_MarketingAliasesDoNotClaimTech(t *testing.T) {
	tests := []struct {
		title string
		want  *bool
	}{
		{"Growth Engineer", nil},
		{"Content Platform Engineer", boolp(true)},
		{"Geo Data Analyst", boolp(true)},
		{"Geospatial Engineer", nil},
		// the marketing titles themselves stay non-tech, as they always were
		{"Growth Marketing Manager", boolp(false)},
		{"Community Manager", boolp(false)},
	}
	for _, tt := range tests {
		got := Derive(Input{Title: tt.title}).IsTech
		if showBoolp(got) != showBoolp(tt.want) {
			t.Errorf("Derive(%q).IsTech = %s, want %s", tt.title, showBoolp(got), showBoolp(tt.want))
		}
	}
}
