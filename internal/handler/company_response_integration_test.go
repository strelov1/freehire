//go:build integration

// Integration tests for the sample-size gate on the company response rate: below it
// the field is absent from the payload rather than zero or estimated.
// Run with: go test -tags=integration ./internal/handler/
package handler

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/db"
)

func TestCompanyResponseRate_GatedBySampleSize(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()

	seed := func(slug string, applications, answered int32) {
		t.Helper()
		if _, err := pool.Exec(ctx,
			`INSERT INTO insights_company_response (company_slug, applications, answered)
			 VALUES ($1, $2, $3)`, slug, applications, answered); err != nil {
			t.Fatalf("seed %s: %v", slug, err)
		}
	}

	seed("thin", responseSampleGate-1, 1)
	seed("thick", responseSampleGate, 3)

	// A ratio over a handful of applications is noise presented as a fact, and a
	// company that happens to have ignored two people would read as ignoring
	// everybody.
	if got := companyResponseRate(ctx, q, "thin"); got != nil {
		t.Errorf("response = %+v, want nil below the gate", got)
	}

	got := companyResponseRate(ctx, q, "thick")
	if got == nil {
		t.Fatal("response = nil at the gate, want it served")
	}
	if got.Applications != responseSampleGate || got.Answered != 3 {
		t.Errorf("response = %+v, want %d applications and 3 answered", got, responseSampleGate)
	}

	// Absence must stay distinguishable from a zero rate: a company nobody
	// observably applied to has said nothing about how it treats applicants.
	if got := companyResponseRate(ctx, q, "never-heard-of"); got != nil {
		t.Errorf("response = %+v, want nil for a company with no row", got)
	}
}

// The median has its own denominator — answered applications only — and therefore its
// own gate. A company can clear the rate's gate comfortably while its median rests on
// three data points.
func TestCompanyMedianReplyDays_GatedSeparatelyAndCensoringReported(t *testing.T) {
	pool := startPostgres(t)
	q := db.New(pool)
	ctx := context.Background()

	seed := func(slug string, applications, answered int32, median *float32) {
		t.Helper()
		if _, err := pool.Exec(ctx,
			`INSERT INTO insights_company_response (company_slug, applications, answered, median_reply_days)
			 VALUES ($1, $2, $3, $4)`, slug, applications, answered, median); err != nil {
			t.Fatalf("seed %s: %v", slug, err)
		}
	}
	days := func(v float32) *float32 { return &v }

	seed("few-answers", 20, replySampleGate-1, days(6))
	seed("many-answers", 20, replySampleGate, days(6))
	seed("answered-nobody", 11, 0, nil)

	if got := companyResponseRate(ctx, q, "few-answers"); got == nil || got.MedianReplyDays != nil {
		t.Errorf("response = %+v, want the rate served and no median below the reply gate", got)
	}

	got := companyResponseRate(ctx, q, "many-answers")
	if got == nil || got.MedianReplyDays == nil {
		t.Fatalf("response = %+v, want a median at the reply gate", got)
	}
	if *got.MedianReplyDays != 6 {
		t.Errorf("median = %v, want 6", *got.MedianReplyDays)
	}
	// The censoring must travel with the median: 15 of the 20 were never answered.
	if got.Unanswered != 15 {
		t.Errorf("unanswered = %d, want 15 — a median over answered applications alone is only half the fact", got.Unanswered)
	}

	// Nobody answered: a rate of zero is the truth, a median of zero would read as
	// "answers immediately".
	zero := companyResponseRate(ctx, q, "answered-nobody")
	if zero == nil {
		t.Fatal("response = nil above the gate, want a zero rate served")
	}
	if zero.Answered != 0 || zero.Unanswered != 11 {
		t.Errorf("response = %+v, want 0 answered and 11 unanswered", zero)
	}
	if zero.MedianReplyDays != nil {
		t.Errorf("median = %v, want absent for a company that answered nobody", *zero.MedianReplyDays)
	}
}
