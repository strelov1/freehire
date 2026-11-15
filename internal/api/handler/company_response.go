package handler

import (
	"context"

	"github.com/strelov1/freehire/internal/application/userjob"
	"github.com/strelov1/freehire/internal/platform/db"
)

// responseSampleGate is how many OBSERVABLE applications a company needs before its response rate is served.
// Below it the field is absent, not zero and not an estimate: a ratio over three applications would label a company
// that ignored two people as ignoring everybody. A judgement, not a measurement: the point where one unlucky
// applicant moves the rate under ten points. An alias for userjob.ObservableSampleGate so the personal-vs-global
// benchmark, which gates the same quantity, cannot drift.
const responseSampleGate = userjob.ObservableSampleGate

// replySampleGate is how many ANSWERED applications a company needs before its median time to first reply is served.
// Separate from responseSampleGate on purpose: the rate's denominator is every observable application, the median's
// only the answered ones, so one gate can be cleared while the other rests on three points. Also a judgement.
const replySampleGate = 5

// companyResponse is the served response-rate fact: a pointer on the company payload, absent below the gate, so a client
// cannot render a rate the data does not support. Unanswered rides along because the median is right-censored: "replies
// in 6 days" without "31 were never answered" shows only the reassuring half.
type companyResponse struct {
	Applications int32 `json:"applications"`
	Answered     int32 `json:"answered"`
	Unanswered   int32 `json:"unanswered"`
	// MedianReplyDays is absent below replySampleGate, and absent, not zero, for a company that answered nobody:
	// zero days would read as "answers immediately".
	MedianReplyDays *float32 `json:"median_reply_days,omitempty"`
}

// companyResponseRate reads a company's observable application counts and returns them only above the sample gate.
// Best-effort: no row or a lookup error yields nil, rendered as nothing. An absent rate is not an unfinished
// implementation; a fabricated one would be a broken promise.
func companyResponseRate(ctx context.Context, q *db.Queries, slug string) *companyResponse {
	row, err := q.GetCompanyResponse(ctx, slug)
	if err != nil {
		return nil
	}
	if row.Applications < responseSampleGate {
		return nil
	}
	out := &companyResponse{
		Applications: row.Applications,
		Answered:     row.Answered,
		Unanswered:   row.Applications - row.Answered,
	}
	if row.MedianReplyDays.Valid && row.Answered >= replySampleGate {
		days := row.MedianReplyDays.Float32
		out.MedianReplyDays = &days
	}
	return out
}
