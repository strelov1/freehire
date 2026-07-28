package experience

import (
	"reflect"
	"strings"
	"testing"
)

func TestProvenancePublishable(t *testing.T) {
	tests := []struct {
		provenance Provenance
		want       bool
	}{
		{ProvenanceCVImport, true},
		{ProvenanceStatedInChat, true},
		{ProvenanceManual, true},
		{ProvenanceAgentInferred, false},
		{Provenance("invented"), false},
		{Provenance(""), false},
	}
	for _, tt := range tests {
		if got := tt.provenance.Publishable(); got != tt.want {
			t.Errorf("Provenance(%q).Publishable() = %v, want %v", tt.provenance, got, tt.want)
		}
	}
}

func TestProvenanceValid(t *testing.T) {
	for _, p := range []Provenance{ProvenanceCVImport, ProvenanceStatedInChat, ProvenanceManual, ProvenanceAgentInferred} {
		if !p.Valid() {
			t.Errorf("Provenance(%q).Valid() = false, want true", p)
		}
	}
	for _, p := range []Provenance{"", "user_said_so", "CV_IMPORT"} {
		if p.Valid() {
			t.Errorf("Provenance(%q).Valid() = true, want false", p)
		}
	}
}

func TestAtomSanitizeBoundsStringsAndArrays(t *testing.T) {
	atom := Atom{
		Claim:      strings.Repeat("a", maxClaimRunes+50),
		Context:    strings.Repeat("b", maxContextRunes+50),
		Metrics:    make([]string, maxMetrics+5),
		Skills:     make([]string, maxSkills+5),
		Provenance: ProvenanceManual,
	}
	for i := range atom.Metrics {
		atom.Metrics[i] = "20s -> 1s"
	}
	for i := range atom.Skills {
		atom.Skills[i] = "go"
	}

	atom.Sanitize()

	if got := len([]rune(atom.Claim)); got != maxClaimRunes {
		t.Errorf("claim runes = %d, want %d", got, maxClaimRunes)
	}
	if got := len([]rune(atom.Context)); got != maxContextRunes {
		t.Errorf("context runes = %d, want %d", got, maxContextRunes)
	}
	if got := len(atom.Metrics); got != maxMetrics {
		t.Errorf("metrics = %d, want %d", got, maxMetrics)
	}
	// Skills canonicalize AND deduplicate, so a list of identical slugs collapses to one.
	if !reflect.DeepEqual(atom.Skills, []string{"go"}) {
		t.Errorf("skills = %q, want [go]", atom.Skills)
	}
}

func TestAtomSanitizeCanonicalizesSkillsDictOnly(t *testing.T) {
	atom := Atom{
		Claim:      "Ran the cluster",
		Skills:     []string{"k8s", "blorptech", "Golang", "  "},
		Provenance: ProvenanceManual,
	}
	atom.Sanitize()

	want := []string{"go", "kubernetes"}
	if !reflect.DeepEqual(atom.Skills, want) {
		t.Errorf("skills = %q, want %q — aliases canonicalize, unknowns emit nothing", atom.Skills, want)
	}
}

func TestAtomSanitizeDropsBlankMetrics(t *testing.T) {
	atom := Atom{Claim: "Cut latency", Metrics: []string{"", "  ", "20s -> 1s"}, Provenance: ProvenanceManual}
	atom.Sanitize()

	if !reflect.DeepEqual(atom.Metrics, []string{"20s -> 1s"}) {
		t.Errorf("metrics = %q, want [20s -> 1s]", atom.Metrics)
	}
}

func TestAtomValidate(t *testing.T) {
	tests := []struct {
		name    string
		atom    Atom
		wantErr error
	}{
		{
			name: "a claim and a known provenance is the whole requirement",
			atom: Atom{Claim: "Cut latency 20s to 1s", Provenance: ProvenanceStatedInChat},
		},
		{
			name:    "an atom with no claim carries no evidence",
			atom:    Atom{Claim: "   ", Context: "some context", Provenance: ProvenanceManual},
			wantErr: ErrEmptyClaim,
		},
		{
			name:    "an unknown provenance is refused, not defaulted",
			atom:    Atom{Claim: "Cut latency", Provenance: "vibes"},
			wantErr: ErrInvalidProvenance,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			atom := tt.atom
			atom.Sanitize()
			if err := atom.Validate(); err != tt.wantErr {
				t.Errorf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// ClaimKey is what import matches on and what the unique index enforces, so two
// spellings of one achievement must produce one key — and two different
// achievements must not collide.
func TestClaimKey(t *testing.T) {
	tests := []struct {
		name string
		a, b string
		same bool
	}{
		{"case differs", "Cut latency 20s to 1s", "cut latency 20s to 1s", true},
		{"whitespace differs", "Cut  latency\n20s to 1s", "Cut latency 20s to 1s", true},
		{"trailing punctuation differs", "Cut latency 20s to 1s.", "Cut latency 20s to 1s", true},
		{"leading/trailing space", "  Cut latency  ", "Cut latency", true},
		{"different achievements", "Cut latency 20s to 1s", "Cut cost by 40%", false},
		{"same words, different numbers", "Cut latency 20s to 1s", "Cut latency 30s to 1s", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ka, kb := ClaimKey(tt.a), ClaimKey(tt.b)
			if ka == "" {
				t.Fatalf("ClaimKey(%q) is empty", tt.a)
			}
			if (ka == kb) != tt.same {
				t.Errorf("ClaimKey(%q)=%q vs ClaimKey(%q)=%q: same=%v, want %v", tt.a, ka, tt.b, kb, ka == kb, tt.same)
			}
		})
	}
}

func TestEmploymentSanitizeAndValidate(t *testing.T) {
	e := Employment{
		Kind:     KindJob,
		Company:  "  RingCentral  ",
		Role:     strings.Repeat("r", maxShortRunes+20),
		Location: "USA, Remote",
		Start:    "2023-09",
		End:      "Present",
		Current:  true,
		Summary:  strings.Repeat("s", maxSummaryRunes+20),
	}
	e.Sanitize()

	if e.Company != "RingCentral" {
		t.Errorf("company = %q, want trimmed", e.Company)
	}
	if got := len([]rune(e.Role)); got != maxShortRunes {
		t.Errorf("role runes = %d, want %d", got, maxShortRunes)
	}
	if got := len([]rune(e.Summary)); got != maxSummaryRunes {
		t.Errorf("summary runes = %d, want %d", got, maxSummaryRunes)
	}
	if err := e.Validate(); err != nil {
		t.Errorf("Validate() = %v, want nil", err)
	}
}

func TestEmploymentValidate(t *testing.T) {
	tests := []struct {
		name    string
		e       Employment
		wantErr error
	}{
		{
			name: "a job needs a company or a role",
			e:    Employment{Kind: KindJob, Company: "RingCentral"},
		},
		{
			name: "a project named only by its role is still a place",
			e:    Employment{Kind: KindProject, Role: "Telegram analytics"},
		},
		{
			name:    "an employment naming nothing is not a place",
			e:       Employment{Kind: KindJob},
			wantErr: ErrEmptyEmployment,
		},
		{
			name:    "an unknown kind is refused, not defaulted",
			e:       Employment{Kind: "hobby", Company: "RingCentral"},
			wantErr: ErrInvalidKind,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := tt.e
			e.Sanitize()
			if err := e.Validate(); err != tt.wantErr {
				t.Errorf("Validate() = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

// The array columns are NOT NULL and pgx sends a nil slice as SQL NULL, so a Sanitize
// that leaves a slice nil produces a row Postgres rejects (SQLSTATE 23502). This is the
// unit-level guard for a failure that otherwise only appears against a real database.
func TestSanitizeNeverLeavesANilSlice(t *testing.T) {
	atom := Atom{Claim: "Did a thing", Provenance: ProvenanceManual}
	atom.Sanitize()
	if atom.Metrics == nil {
		t.Error("metrics is nil after Sanitize — the column is NOT NULL")
	}
	if atom.Skills == nil {
		t.Error("skills is nil after Sanitize — the column is NOT NULL")
	}

	// Every skill failing to resolve is the realistic path to nil, not the empty input.
	atom = Atom{Claim: "Did a thing", Skills: []string{"blorptech"}, Provenance: ProvenanceManual}
	atom.Sanitize()
	if atom.Skills == nil {
		t.Error("skills is nil after every token failed to resolve — the column is NOT NULL")
	}

	e := Employment{Kind: KindJob, Company: "RingCentral", Stack: []string{"blorptech"}}
	e.Sanitize()
	if e.Stack == nil {
		t.Error("stack is nil after Sanitize — the column is NOT NULL")
	}
}
