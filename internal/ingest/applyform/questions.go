package applyform

import (
	"context"
	"encoding/json"
	"log"

	"github.com/strelov1/freehire/internal/platform/db"
)

// RequiredQuestions is how many questions the employer REQUIRES an answer to, beyond the
// standard application every form on the platform asks for — the number a candidate reads
// as "3 questions" before deciding whether to apply.
//
// It counts exactly the required entries of ForDisplay's Questions, so the figure and the
// list the job page shows under it cannot disagree. That projection already decides what
// is not a question, and the reasons carry over unchanged:
//
//   - the standard fields (name, contact details, CV, cover letter) are asked of everyone
//     and cost nothing to answer, so they are the baseline, not the count — and the
//     platforms disagree about which of them they even show, which would make a raw
//     field count compare platforms rather than employers;
//   - hidden controls and text blocks are not answerable at all;
//   - the equal-opportunity survey and consent boxes are the platform's, not the
//     employer's, and identical everywhere.
//
// Optional questions are not counted: a candidate in a hurry may skip them, so they do
// not make an application longer.
func (f Form) RequiredQuestions() int {
	n := 0
	for _, q := range f.ForDisplay().Questions {
		if q.Required {
			n++
		}
	}
	return n
}

// FormLister reads the captured forms of a batch of jobs. *db.Queries satisfies it.
type FormLister interface {
	ListApplyFormsForJobs(ctx context.Context, jobIDs []int64) ([]db.ListApplyFormsForJobsRow, error)
}

// QuestionCounts reads a batch of jobs' captured forms and counts each one's required
// questions (RequiredQuestions), for the search-index writers that put the count on the
// document. One query per batch.
//
// A job with no captured form is ABSENT from the map, never zero: zero says "this
// employer asks nothing beyond the basics", which is a claim, while no form means we do
// not know — and the filter over the count must not mistake one for the other.
//
// A payload that does not decode is left out the same way and logged: it is one posting
// whose cost we cannot state, which is not a reason to fail the batch of documents
// around it.
func QuestionCounts(ctx context.Context, q FormLister, jobIDs []int64) (map[int64]int, error) {
	if q == nil || len(jobIDs) == 0 {
		return nil, nil
	}
	rows, err := q.ListApplyFormsForJobs(ctx, jobIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]int, len(rows))
	for _, r := range rows {
		var form Form
		if err := json.Unmarshal(r.Payload, &form); err != nil {
			log.Printf("applyform: job %d: undecodable form, no question count: %v", r.JobID, err)
			continue
		}
		// The row's provider is authoritative, as in the job page's read: it is what a
		// re-capture rewrites, and it selects the platform's standard-field rule.
		form.Provider = r.Provider
		out[r.JobID] = form.RequiredQuestions()
	}
	return out, nil
}
