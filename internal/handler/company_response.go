package handler

import (
	"context"

	"github.com/strelov1/freehire/internal/db"
)

// responseSampleGate is how many OBSERVABLE applications a company needs before its
// response rate is served at all.
//
// Below it the field is absent, not zero and not an estimate. A ratio over three
// applications is noise presented as a fact, and a company that happens to have
// ignored two people would be labelled as ignoring everybody — the worst outcome for
// a signal whose entire purpose is to be trusted.
//
// Ten is a judgement, not a measurement, and it should be revisited once the sample
// exists. It is set where a rate first survives one unlucky applicant changing it by
// less than ten points.
const responseSampleGate = 10

// replySampleGate is how many ANSWERED applications a company needs before its median
// time to first reply is served.
//
// Separate from responseSampleGate on purpose, and deliberately not shared with it: the
// rate's denominator is every observable application, the median's is only the answered
// ones, so a company can clear the first comfortably while the second rests on three
// data points. They are two numbers with two denominators and they need two gates.
//
// Like the gate beside it this is a judgement, not a measurement, and should be revisited
// once the sample exists.
const replySampleGate = 5

// companyResponse is the served response-rate fact. It is a pointer field on the
// company payload and absent below the gate, so a client cannot render a rate the
// data does not support: there is nothing to render.
//
// Unanswered rides along because the median is right-censored: it covers the answered
// applications only, and a client shown "replies in 6 days" without "and 31 were never
// answered" would read the reassuring half of a two-part fact.
type companyResponse struct {
	Applications int32 `json:"applications"`
	Answered     int32 `json:"answered"`
	Unanswered   int32 `json:"unanswered"`
	// MedianReplyDays is absent below replySampleGate, and absent — not zero — for a
	// company that answered nobody. A median of zero days would read as "answers
	// immediately", the exact inversion of what happened.
	MedianReplyDays *float32 `json:"median_reply_days,omitempty"`
}

// companyResponseRate reads a company's observable application counts and returns
// them only above the sample gate.
//
// Best-effort: no row, or a lookup error, yields nil — which the interface renders as
// nothing at all. That is the correct answer for nearly every company today, and it
// will stay the correct answer until applications with observable outcomes
// accumulate. An absent rate is not an unfinished implementation; a fabricated one
// would be a broken promise.
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
