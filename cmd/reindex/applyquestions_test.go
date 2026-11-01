package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ingest/applyform"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

type fakeFormLister []db.ListApplyFormsForJobsRow

func (f fakeFormLister) ListApplyFormsForJobs(context.Context, []int64) ([]db.ListApplyFormsForJobsRow, error) {
	return f, nil
}

// The rebuild is what reaches every posting that already has a captured form. A job with
// one carries its count — zero included, a real answer — and a job without carries no key
// at all, because "we do not know" must not be filtered as a short form.
func TestStreamOpenDocs_CarriesTheApplyQuestionCount(t *testing.T) {
	body := "<p>Build things.</p>"
	asks := db.Job{ID: 1, Title: "A", PublicSlug: "a", Category: "backend", Description: body}
	asksNothing := db.Job{ID: 2, Title: "B", PublicSlug: "b", Category: "backend", Description: body}
	unknown := db.Job{ID: 3, Title: "C", PublicSlug: "c", Category: "backend", Description: body}
	reader := &fakePageReader{pages: map[int64][]db.Job{0: {asks, asksNothing, unknown}}}

	two, _ := json.Marshal(applyform.Form{Fields: []applyform.Field{
		{ID: "question_1", Label: "Why us?", Type: applyform.TypeTextarea, Required: true},
		{ID: "question_2", Label: "Visa?", Type: applyform.TypeBoolean, Required: true},
	}})
	none, _ := json.Marshal(applyform.Form{})
	forms := fakeFormLister{
		{JobID: 1, Provider: "greenhouse", Payload: two},
		{JobID: 2, Provider: "greenhouse", Payload: none},
	}

	var got []search.JobDocument
	_, _, err := streamOpenDocs(context.Background(), reader, nil, nil, nil, forms, time.Now(),
		func(_ context.Context, docs []search.JobDocument) error {
			got = append(got, docs...)
			return nil
		})
	if err != nil {
		t.Fatalf("streamOpenDocs: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("docs = %d, want 3", len(got))
	}
	if q := got[0].ApplyQuestions; q == nil || *q != 2 {
		t.Errorf("job 1 apply_questions = %v, want 2", q)
	}
	if q := got[1].ApplyQuestions; q == nil || *q != 0 {
		t.Errorf("job 2 apply_questions = %v, want a present 0", q)
	}
	if got[2].ApplyQuestions != nil {
		t.Errorf("job 3 apply_questions = %d, want absent (no form captured)", *got[2].ApplyQuestions)
	}
}
