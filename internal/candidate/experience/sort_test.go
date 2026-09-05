package experience

import (
	"testing"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/candidate/perioddate"
)

func date(year, month int) *perioddate.PeriodDate {
	return &perioddate.PeriodDate{Year: year, Month: month}
}

// Fixture matching a career scramble: the free-text era's lexicographic period_start
// DESC put Fabrikam ("2024") under month-named 2017-2018 roles. Chronological sort must
// not — same fixture as the deleted period_sort_test.go's, dates now structured from
// the start instead of parsed from strings.
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
		{ID: northwind, Company: "Northwind", Start: date(2018, 10), End: date(2024, 0)},
		{ID: litware, Company: "Litware", Start: date(2018, 5), End: date(2018, 8)},
		{ID: adventure, Company: "Adventure Works", Start: date(2017, 6), End: date(2017, 12)},
		{ID: woodgrove, Company: "Woodgrove", Start: date(2018, 1), End: date(2018, 4)},
		{ID: fabrikam, Company: "Fabrikam", Start: date(2024, 0), End: date(2025, 0)},
		{ID: contoso, Company: "Contoso", Start: date(2025, 0), End: nil, Current: true},
		{ID: unknown, Company: "Mystery", Start: nil, End: nil},
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

func TestEmploymentSortKey_PrefersEndThenStart(t *testing.T) {
	if got := employmentSortKey(Employment{Start: date(2018, 1), End: date(2024, 0)}); got != 202401 {
		t.Errorf("end wins: got %d", got)
	}
	if got := employmentSortKey(Employment{Start: date(2018, 10)}); got != 201810 {
		t.Errorf("start fallback: got %d", got)
	}
	if got := employmentSortKey(Employment{Start: date(2010, 0), End: nil, Current: true}); got != 999999 {
		t.Errorf("current flag: got %d", got)
	}
	if got := employmentSortKey(Employment{}); got != 0 {
		t.Errorf("unknown: got %d", got)
	}
}
