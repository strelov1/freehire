package handler

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/screeninganswers"
)

// fakeScreeningAnswersReader is a screeningAnswersReader returning a canned record/error.
type fakeScreeningAnswersReader struct {
	ret screeninganswers.Answers
	err error
}

func (f fakeScreeningAnswersReader) Get(context.Context, int64) (screeninganswers.Answers, error) {
	return f.ret, f.err
}

func TestAutofillProfile_IncludesScreeningAnswersWhenStated(t *testing.T) {
	days := 14
	h := &autofillHandlers{
		cvs:              fakeBaseCV{},
		resumes:          fakeStructuredResume{},
		accounts:         fakeAccount{email: "account@example.com"},
		screeningAnswers: fakeScreeningAnswersReader{ret: screeninganswers.Answers{NoticePeriodDays: &days}},
	}

	got, err := h.autofillProfile(context.Background(), 7)
	if err != nil {
		t.Fatalf("autofillProfile: %v", err)
	}
	if got.NoticePeriod != "14 days" {
		t.Errorf("NoticePeriod = %q, want %q", got.NoticePeriod, "14 days")
	}
}

func TestAutofillProfile_NoScreeningReaderYieldsEmptyScreeningFields(t *testing.T) {
	h := &autofillHandlers{
		cvs:      fakeBaseCV{},
		resumes:  fakeStructuredResume{},
		accounts: fakeAccount{email: "account@example.com"},
	}

	got, err := h.autofillProfile(context.Background(), 7)
	if err != nil {
		t.Fatalf("autofillProfile: %v", err)
	}
	if got.NoticePeriod != "" || got.DesiredSalary != "" {
		t.Errorf("screening fields = %+v, want all empty with no reader configured", got)
	}
}

func TestAutofillProfile_ScreeningAnswersNotFoundYieldsEmptyFields(t *testing.T) {
	h := &autofillHandlers{
		cvs:              fakeBaseCV{},
		resumes:          fakeStructuredResume{},
		accounts:         fakeAccount{email: "account@example.com"},
		screeningAnswers: fakeScreeningAnswersReader{err: screeninganswers.ErrNotFound},
	}

	got, err := h.autofillProfile(context.Background(), 7)
	if err != nil {
		t.Fatalf("autofillProfile: %v", err)
	}
	if got.WillingToRelocate != "" {
		t.Errorf("WillingToRelocate = %q, want empty when the caller has stated no screening answers", got.WillingToRelocate)
	}
}
