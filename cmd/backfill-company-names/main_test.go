package main

import (
	"context"
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
