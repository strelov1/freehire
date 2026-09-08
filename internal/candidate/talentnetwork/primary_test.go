package talentnetwork

import (
	"testing"

	"github.com/strelov1/freehire/internal/candidate/perioddate"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
)

func date(year, month int) *perioddate.PeriodDate {
	return &perioddate.PeriodDate{Year: year, Month: month}
}

func TestPrimaryTitle_PrefersTheCurrentRole(t *testing.T) {
	// Deliberately NOT in newest-first order. Structured.Experience's ordering is
	// nowhere documented or enforced by the extraction prompt, which is the same
	// reason resumeextract.Anonymous masks by content rather than by position.
	s := resumeextract.Structured{Experience: []resumeextract.Experience{
		{Title: "Backend Engineer", Start: date(2026, 1), Current: true},
		{Title: "QA Engineer", Start: date(2024, 3), End: date(2025, 12)},
	}}
	if got := PrimaryTitle(s); got != "Backend Engineer" {
		t.Errorf("PrimaryTitle = %q, want the current role", got)
	}
}

// A nil End means ongoing too: the extraction contract defines an unstated end that
// way, and a model that complies with end:null but slips on current:true must not
// silently demote the role.
func TestPrimaryTitle_TreatsAnUnsetEndAsCurrent(t *testing.T) {
	s := resumeextract.Structured{Experience: []resumeextract.Experience{
		{Title: "QA Engineer", Start: date(2024, 3), End: date(2025, 12)},
		{Title: "Backend Engineer", Start: date(2026, 1)},
	}}
	if got := PrimaryTitle(s); got != "Backend Engineer" {
		t.Errorf("PrimaryTitle = %q, want the role with no end", got)
	}
}

func TestPrimaryTitle_FallsBackToTheLatestEndedRole(t *testing.T) {
	s := resumeextract.Structured{Experience: []resumeextract.Experience{
		{Title: "Junior Developer", Start: date(2019, 1), End: date(2021, 6)},
		{Title: "Data Engineer", Start: date(2021, 7), End: date(2025, 4)},
	}}
	if got := PrimaryTitle(s); got != "Data Engineer" {
		t.Errorf("PrimaryTitle = %q, want the most recently ended role", got)
	}
}

func TestPrimaryTitle_TwoConcurrentCurrentRolesPickTheLaterStart(t *testing.T) {
	s := resumeextract.Structured{Experience: []resumeextract.Experience{
		{Title: "Advisor", Start: date(2023, 1), Current: true},
		{Title: "Backend Engineer", Start: date(2025, 9), Current: true},
	}}
	if got := PrimaryTitle(s); got != "Backend Engineer" {
		t.Errorf("PrimaryTitle = %q, want the more recently started current role", got)
	}
}

func TestPrimaryTitle_EmptyWhenThereIsNothingToRead(t *testing.T) {
	cases := map[string]resumeextract.Structured{
		"no experience":  {},
		"untitled roles": {Experience: []resumeextract.Experience{{Company: "X", Current: true}}},
	}
	for name, s := range cases {
		t.Run(name, func(t *testing.T) {
			if got := PrimaryTitle(s); got != "" {
				t.Errorf("PrimaryTitle = %q, want empty", got)
			}
		})
	}
}

// The handle falls back to the neutral base whenever no title can be read, rather than
// failing the join. Being unable to name somebody's discipline is not a reason to leave
// them without a URL.
func TestHandleBase_OverAnUnreadableCV(t *testing.T) {
	if got := HandleBase(PrimaryTitle(resumeextract.Structured{})); got != neutralBase {
		t.Errorf("HandleBase over an empty CV = %q, want %q", got, neutralBase)
	}
}
