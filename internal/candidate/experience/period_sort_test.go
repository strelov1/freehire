package experience

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestParsePeriodLabel(t *testing.T) {
	cases := []struct {
		in   string
		want periodSortKey
		ok   bool
	}{
		{"2024", periodSortKey(202401), true},
		{"2023-09", periodSortKey(202309), true},
		{"2023/09", periodSortKey(202309), true},
		{"Jan 2018", periodSortKey(201801), true},
		{"January 2018", periodSortKey(201801), true},
		{"Oct 2018", periodSortKey(201810), true},
		{"October 2018", periodSortKey(201810), true},
		{"May 2018", periodSortKey(201805), true},
		{"Jun 2017", periodSortKey(201706), true},
		{"Present", 0, false},
		{"", 0, false},
		{"sometime", 0, false},
	}
	for _, tc := range cases {
		got, ok := parsePeriodLabel(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("parsePeriodLabel(%q) = %d,%v want %d,%v", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestSortKeyPrefersEndThenStart(t *testing.T) {
	if got := sortKeyForEmployment("2018-01", "2024", false); got != periodSortKey(202401) {
		t.Errorf("end wins: got %d", got)
	}
	if got := sortKeyForEmployment("October 2018", "", false); got != periodSortKey(201810) {
		t.Errorf("start fallback: got %d", got)
	}
	if got := sortKeyForEmployment("2010", "Present", false); got != periodKeyCurrent {
		t.Errorf("Present end: got %d", got)
	}
	if got := sortKeyForEmployment("2010", "2020", true); got != periodKeyCurrent {
		t.Errorf("current flag: got %d", got)
	}
	if got := sortKeyForEmployment("", "", false); got != periodKeyUnknown {
		t.Errorf("unknown: got %d", got)
	}
}

// Fixture matching a career scramble: lexicographic period_start DESC puts
// Fabrikam ("2024") under month-named 2017–2018 roles. Chronological sort must not.
func TestSortEmploymentsChronological_FabrikamBeforeNorthwind(t *testing.T) {
	contoso := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	fabrikam := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	northwind := uuid.MustParse("33333333-3333-3333-3333-333333333333")
	litware := uuid.MustParse("44444444-4444-4444-4444-444444444444")
	woodgrove := uuid.MustParse("55555555-5555-5555-5555-555555555555")
	adventure := uuid.MustParse("66666666-6666-6666-6666-666666666666")
	unknown := uuid.MustParse("77777777-7777-7777-7777-777777777777")

	// Deliberately inserted in the wrong (lexicographic-ish) order.
	list := []Employment{
		{ID: northwind, Company: "Northwind", Start: "October 2018", End: "2024"},
		{ID: litware, Company: "Litware", Start: "May 2018", End: "Aug 2018"},
		{ID: adventure, Company: "Adventure Works", Start: "Jun 2017", End: "Dec 2017"},
		{ID: woodgrove, Company: "Woodgrove", Start: "Jan 2018", End: "Apr 2018"},
		{ID: fabrikam, Company: "Fabrikam", Start: "2024", End: "2025"},
		{ID: contoso, Company: "Contoso", Start: "2025", End: "Present", Current: true},
		{ID: unknown, Company: "Mystery", Start: "", End: ""},
	}
	sortEmploymentsChronological(list)

	want := []string{"Contoso", "Fabrikam", "Northwind", "Litware", "Woodgrove", "Adventure Works", "Mystery"}
	got := make([]string, len(list))
	for i, e := range list {
		got[i] = e.Company
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
}

func TestListEmployments_SortsChronologically(t *testing.T) {
	s, _ := newStore()
	ctx := context.Background()
	for _, e := range []Employment{
		{Kind: KindJob, Company: "Northwind", Role: "Staff", Start: "October 2018", End: "2024"},
		{Kind: KindJob, Company: "Adventure Works", Role: "BE", Start: "Jun 2017", End: "Dec 2017"},
		{Kind: KindJob, Company: "Fabrikam", Role: "Staff", Start: "2024", End: "2025"},
		{Kind: KindJob, Company: "Contoso", Role: "Staff", Start: "2025", End: "Present", Current: true},
	} {
		if _, err := s.CreateEmployment(ctx, owner, e); err != nil {
			t.Fatalf("CreateEmployment: %v", err)
		}
	}
	got, err := s.ListEmployments(ctx, owner)
	if err != nil {
		t.Fatalf("ListEmployments: %v", err)
	}
	want := []string{"Contoso", "Fabrikam", "Northwind", "Adventure Works"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Company != want[i] {
			t.Fatalf("order companies = %v, want %v", employmentCompanies(got), want)
		}
	}

	hist, err := s.WorkHistory(ctx, owner)
	if err != nil {
		t.Fatalf("WorkHistory: %v", err)
	}
	for i := range want {
		if hist[i].Company != want[i] {
			comps := make([]string, len(hist))
			for j, h := range hist {
				comps[j] = h.Company
			}
			t.Fatalf("WorkHistory order = %v, want %v", comps, want)
		}
	}
}

// Placeless publishable atoms trail dated roles — they must not sort into the middle of
// the career list as a fake titled job (US1b scenario 4).
func TestWorkHistory_PlacelessTrailsChronologicalRoles(t *testing.T) {
	s, _ := newStore()
	ctx := context.Background()
	for _, e := range []Employment{
		{Kind: KindJob, Company: "Northwind", Role: "Staff", Start: "October 2018", End: "2024"},
		{Kind: KindJob, Company: "Contoso", Role: "Staff", Start: "2025", End: "Present", Current: true},
	} {
		if _, err := s.CreateEmployment(ctx, owner, e); err != nil {
			t.Fatalf("CreateEmployment: %v", err)
		}
	}
	if _, err := s.AddAtom(ctx, owner, Atom{
		Claim: "Certified cloud architect", Provenance: ProvenanceStatedInChat,
	},
		AuthorQuoted,
	); err != nil {
		t.Fatalf("AddAtom placeless: %v", err)
	}
	hist, err := s.WorkHistory(ctx, owner)
	if err != nil {
		t.Fatalf("WorkHistory: %v", err)
	}
	if len(hist) != 3 {
		t.Fatalf("len = %d, want 3 (2 roles + placeless)", len(hist))
	}
	if hist[0].Company != "Contoso" || hist[1].Company != "Northwind" {
		t.Fatalf("roles = %q, %q, want Contoso then Northwind", hist[0].Company, hist[1].Company)
	}
	last := hist[2]
	if last.Company != "" || last.Title != "" {
		t.Fatalf("placeless entry = %+v, want empty company/title", last)
	}
	if len(last.Highlights) != 1 || last.Highlights[0] != "Certified cloud architect" {
		t.Fatalf("placeless highlights = %q", last.Highlights)
	}
}

func employmentCompanies(list []Employment) []string {
	out := make([]string, len(list))
	for i, e := range list {
		out[i] = e.Company
	}
	return out
}
