package main

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/companyname"
	"github.com/strelov1/freehire/internal/db"
)

// stubResolver returns a canned candidate per board, standing in for a real ATS
// fetch so resolveNames can be exercised without network.
type stubResolver struct{ byBoard map[string]string }

func (s stubResolver) Name(_ context.Context, board string) (string, error) {
	return s.byBoard[board], nil
}

func TestResolveNames(t *testing.T) {
	registry := companyname.Registry{
		"pinpoint": stubResolver{byBoard: map[string]string{
			"afcb":       "AFC Bournemouth",          // accepted (substring)
			"kempinski":  "Elena - Meta Recruitment", // rejected (unrelated)
			"lbresearch": "Centellic",                // rejected (rebrand shares nothing)
		}},
	}
	rows := []db.ListSlugLikeCompaniesForBackfillRow{
		{Slug: "afcb", Name: "afcb", Source: "pinpoint", URL: "https://afcb.pinpointhq.com/x"},
		{Slug: "kempinski", Name: "kempinski", Source: "pinpoint", URL: "https://kempinski.pinpointhq.com/x"},
		{Slug: "lbresearch", Name: "lbresearch", Source: "pinpoint", URL: "https://lbresearch.pinpointhq.com/x"},
		{Slug: "acme", Name: "acme", Source: "unknown-ats", URL: "https://acme.example.com/x"},  // no resolver
		{Slug: "bar", Name: "Bar Inc", Source: "pinpoint", URL: "https://bar.pinpointhq.com/x"}, // not slug-like
	}

	renames, stats := resolveNames(context.Background(), rows, registry)

	if len(renames) != 1 || renames[0].oldSlug != "afcb" || renames[0].name != "AFC Bournemouth" {
		t.Fatalf("renames = %+v, want [{afcb AFC Bournemouth}]", renames)
	}
	if stats.noSource != 1 {
		t.Errorf("noSource = %d, want 1", stats.noSource)
	}
	if stats.rejected != 2 {
		t.Errorf("rejected = %d, want 2 (kempinski, lbresearch)", stats.rejected)
	}
}

// TestApplyRenames_CountsFailures guards the worker's exit-code contract: a
// per-company write failure must be counted, not just logged and dropped, so
// the caller can turn it into a non-zero exit via worker.ExitCode and cron
// actually alerts instead of silently succeeding on a partial batch.
func TestApplyRenames_CountsFailures(t *testing.T) {
	renames := []rename{
		{oldSlug: "afcb", name: "AFC Bournemouth"},
		{oldSlug: "kempinski", name: "Kempinski Hotels"},
		{oldSlug: "lbresearch", name: "Centellic"},
	}
	applied, failed := applyRenames(context.Background(), renames, func(_ context.Context, r rename) (bool, error) {
		if r.oldSlug == "kempinski" {
			return false, errors.New("unique constraint violation")
		}
		return true, nil
	})
	if applied != 2 {
		t.Errorf("applied = %d, want 2", applied)
	}
	if failed != 1 {
		t.Errorf("failed = %d, want 1", failed)
	}
}

// TestApplyRenames_ContinuesPastAFailure guards that one company's write
// error doesn't abort the rest of the batch — every rename must still be
// attempted.
func TestApplyRenames_ContinuesPastAFailure(t *testing.T) {
	renames := []rename{
		{oldSlug: "a", name: "A"},
		{oldSlug: "b", name: "B"},
		{oldSlug: "c", name: "C"},
	}
	var attempted []string
	applyRenames(context.Background(), renames, func(_ context.Context, r rename) (bool, error) {
		attempted = append(attempted, r.oldSlug)
		if r.oldSlug == "a" {
			return false, errors.New("boom")
		}
		return true, nil
	})
	if len(attempted) != 3 {
		t.Errorf("attempted %d renames, want 3 (a failing must not stop b, c)", len(attempted))
	}
}
