package cvlifecycle

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

type fakeReportStore struct {
	events  []db.CVLetterEngagementRow
	pay     map[int16]db.CVLetterPaymentsRow
	control map[int32]db.CVLetterHoldoutAtStepRow
	groups  []db.CVLetterGroupsFromEnrollmentRow
	err     error
}

func (f *fakeReportStore) CVLetterEngagement(context.Context) ([]db.CVLetterEngagementRow, error) {
	return f.events, f.err
}

func (f *fakeReportStore) CVLetterPayments(_ context.Context, a db.CVLetterPaymentsParams) (db.CVLetterPaymentsRow, error) {
	return f.pay[a.Step], f.err
}

func (f *fakeReportStore) CVLetterHoldoutAtStep(_ context.Context, a db.CVLetterHoldoutAtStepParams) (db.CVLetterHoldoutAtStepRow, error) {
	return f.control[a.AfterDays], f.err
}

func (f *fakeReportStore) CVLetterGroupsFromEnrollment(context.Context, int32) ([]db.CVLetterGroupsFromEnrollmentRow, error) {
	return f.groups, f.err
}

func TestBuildReport_EachLetterReadsItsOwnCampaignAndItsOwnControl(t *testing.T) {
	store := &fakeReportStore{
		events: []db.CVLetterEngagementRow{
			{Campaign: "cv-letter-1", Type: "delivery", Events: 90},
			{Campaign: "cv-letter-1", Type: "click", Events: 12},
			{Campaign: "cv-letter-3", Type: "click", Events: 5},
			{Campaign: "cv-letter-3", Type: "complaint", Events: 1},
		},
		pay: map[int16]db.CVLetterPaymentsRow{
			1: {Sent: 100, Matured: 80, Paid: 4},
			3: {Sent: 50, Matured: 40, Paid: 6},
		},
		control: map[int32]db.CVLetterHoldoutAtStepRow{
			1:  {Accounts: 10, Paid: 1},
			13: {Accounts: 8, Paid: 0},
		},
		groups: []db.CVLetterGroupsFromEnrollmentRow{
			{Holdout: false, Accounts: 200, Paid: 10},
			{Holdout: true, Accounts: 20, Paid: 2},
		},
	}
	r, err := BuildReport(context.Background(), store, DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Letters) != 3 {
		t.Fatalf("letters = %d, want 3", len(r.Letters))
	}
	one, two, three := r.Letters[0], r.Letters[1], r.Letters[2]
	if one.Delivered != 90 || one.Clicked != 12 || one.Sent != 100 {
		t.Errorf("letter 1 = %+v", one)
	}
	if one.Recipients != (Cohort{Accounts: 80, Paid: 4}) || one.Control != (Cohort{Accounts: 10, Paid: 1}) {
		t.Errorf("letter 1 cohorts = %+v / %+v", one.Recipients, one.Control)
	}
	if two.Clicked != 0 || two.Sent != 0 {
		t.Errorf("letter 2 took another letter's figures: %+v", two)
	}
	if three.Clicked != 5 || three.Complained != 1 || three.Control != (Cohort{Accounts: 8}) {
		t.Errorf("letter 3 = %+v", three)
	}
	if r.Holdout != (Cohort{Accounts: 20, Paid: 2}) || r.Mailed != (Cohort{Accounts: 200, Paid: 10}) {
		t.Errorf("groups: holdout %+v, mailed %+v", r.Holdout, r.Mailed)
	}
}

func TestBuildReport_StoreErrorFailsTheReport(t *testing.T) {
	if _, err := BuildReport(context.Background(), &fakeReportStore{err: errors.New("down")}, DefaultConfig()); err == nil {
		t.Fatal("a store error was swallowed")
	}
}

func TestCohortRate_NobodyIsNoRateNotZero(t *testing.T) {
	if _, ok := (Cohort{}).Rate(); ok {
		t.Error("an empty cohort has a rate")
	}
	if rate, ok := (Cohort{Accounts: 4, Paid: 1}).Rate(); !ok || rate != 0.25 {
		t.Errorf("rate = %v, %v", rate, ok)
	}
}

func TestReportWrite_PrintsRatesAndNeverInventsOpens(t *testing.T) {
	r := Report{
		WindowDays: 14,
		Letters: []LetterReport{
			{Step: StepATS, Sent: 100, Clicked: 12, Recipients: Cohort{Accounts: 80, Paid: 4}, Control: Cohort{Accounts: 10, Paid: 1}},
			{Step: StepPotential},
		},
		Holdout: Cohort{Accounts: 20, Paid: 2},
	}
	var out bytes.Buffer
	if err := r.Write(&out); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"4/80 (5.0%)", "1/10 (10.0%)", "2 (10.0%)", "n/a"} {
		if !strings.Contains(got, want) {
			t.Errorf("report lacks %q:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "0/0 (-)") {
		t.Errorf("an empty cohort should print no rate:\n%s", got)
	}
}
