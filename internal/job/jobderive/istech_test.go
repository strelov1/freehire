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

// TestDerive_IsTech_SourceHint pins the precedence of a structured "confirmed
// technical" source signal (e.g. a source that crawls only a dedicated IT board)
// over the title/category dictionaries — see the tech-classification spec's
// "structured source is_tech signal" requirement.
func TestDerive_IsTech_SourceHint(t *testing.T) {
	tests := []struct {
		name string
		in   Input
		want *bool
	}{
		{
			name: "hint yields true when neither dictionary resolves anything",
			in:   Input{Title: "Windows rendszermérnök", IsTechHint: true},
			want: boolp(true),
		},
		{
			name: "hint overrides an otherwise-confident non-tech title match",
			in:   Input{Title: "Warehouse Janitorial Cleaner", IsTechHint: true},
			want: boolp(true),
		},
		{
			name: "hint absent falls back to the existing dictionary precedence",
			in:   Input{Title: "Sales Manager", IsTechHint: false},
			want: boolp(false),
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

// TestDerive_IsTech_NonTechTitleBeatsTechCategory pins which of the two technical
// signals may be outvoted.
//
// A technical TITLE speaks for the whole role and keeps its outright win — that is what
// stops "Backend Engineer — Teller Systems" from being dragged out of the catalogue by
// its accidental "teller". A technical CATEGORY is resolved from a SUBSTRING, and the
// substring can be about something else: `qa` from a hospital's quality assurance,
// `mobile` from a mobile clinic or a mobile mechanic, `security` from occupational
// safety. In every case below the title dictionary already said non-technical outright
// and was never asked, because the category had already asserted true.
//
// Measured over the 3 000 commonest titles currently flagged technical (197 916 open
// postings), this changes 22 titles and 2 067 postings — every one a correction, and
// none in the other direction, since the positive branch is strictly narrower than
// before.
func TestDerive_IsTech_NonTechTitleBeatsTechCategory(t *testing.T) {
	for _, tt := range []struct {
		name string
		in   Input
		want *bool
	}{
		// The category is technical; the title says otherwise, and now wins.
		{"qa category, nurse title", Input{Title: "Quality Assurance Nurse Manager"}, boolp(false)},
		{"qa category, nurse abbreviation", Input{Title: "QA Nurse LPN/RN"}, boolp(false)},
		{"qa category, lab technician", Input{Title: "QA Lab Technician"}, boolp(false)},
		{"mobile category, nurse title", Input{Title: "Registered Nurse Intensive Mobile Team"}, boolp(false)},
		{"mobile category, mechanic title", Input{Title: "Mobile Diesel Mechanic"}, boolp(false)},
		{"project_management category, HVAC title", Input{Title: "HVAC Project Manager"}, boolp(false)},

		// A technical TITLE still wins outright — the case TechEvidence exists to protect.
		{"tech title survives an accidental non-tech word", Input{Title: "Backend Engineer — Teller Systems"}, boolp(true)},
		{"tech title survives a non-tech domain", Input{Title: "Data Engineer, Nurse Scheduling"}, boolp(true)},
		{"tech title survives a non-tech product", Input{Title: "Full Stack Developer (Janitorial SaaS)"}, boolp(true)},

		// A technical category with no non-tech title is untouched.
		{"qa category, software title", Input{Title: "QA Automation Engineer"}, boolp(true)},
		{"mobile category, software title", Input{Title: "Mobile Developer"}, boolp(true)},

		// The source hint still wins ahead of everything, including a non-tech title:
		// it is a fact the source stated about its own crawl scope.
		{"source hint beats a non-tech title", Input{Title: "QA Nurse LPN/RN", IsTechHint: true}, boolp(true)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := Derive(tt.in).IsTech; showBoolp(got) != showBoolp(tt.want) {
				t.Errorf("IsTech(%q) = %s, want %s", tt.in.Title, showBoolp(got), showBoolp(tt.want))
			}
		})
	}
}
