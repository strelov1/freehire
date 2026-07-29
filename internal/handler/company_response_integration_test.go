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
