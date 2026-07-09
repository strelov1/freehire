package main

import (
	"context"
	"encoding/json"
	"reflect"
	"testing"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/ycdir"
)

type fakeStore struct {
	exists map[string]bool
	calls  []db.UpsertYCCompanyParams
}

func (f *fakeStore) CompanyExists(_ context.Context, slug string) (bool, error) {
	return f.exists[slug], nil
}

func (f *fakeStore) UpsertYCCompany(_ context.Context, p db.UpsertYCCompanyParams) error {
	f.calls = append(f.calls, p)
	return nil
}

func TestLoad(t *testing.T) {
	entries := []ycdir.Entry{
		{Name: "Stripe", OneLiner: "Payments", Industry: "Fintech", TeamSize: 8000, Batch: "Summer 2009", Status: "Public"},
		{Name: "New Co", Batch: "Winter 2024", Status: "Active"},
		{Name: "   "}, // blank → skipped
	}
	fs := &fakeStore{exists: map[string]bool{"stripe": true, "new-co": false}}

	stats, err := load(context.Background(), fs, entries)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if stats.matched != 1 || stats.inserted != 1 || stats.skipped != 1 {
		t.Errorf("stats = %+v, want matched1 inserted1 skipped1", stats)
	}
	if len(fs.calls) != 2 {
		t.Fatalf("UpsertYCCompany called %d times, want 2", len(fs.calls))
	}

	stripe := fs.calls[0]
	if stripe.Slug != "stripe" || stripe.Tagline.String != "Payments" {
		t.Errorf("stripe slug/tagline = %q/%q", stripe.Slug, stripe.Tagline.String)
	}
	if !reflect.DeepEqual(stripe.YcBatch, []string{"Summer 2009"}) || !reflect.DeepEqual(stripe.YcStatus, []string{"Public"}) {
		t.Errorf("stripe yc facets = %v/%v", stripe.YcBatch, stripe.YcStatus)
	}
	if stripe.Industries == nil {
		t.Error("industries is nil, want non-nil for the NOT NULL column")
	}
	var info map[string]any
	if err := json.Unmarshal(stripe.CompanyInfo, &info); err != nil {
		t.Fatalf("company_info not JSON: %v", err)
	}

	// New Co: no batch text beyond the single value, non-nil arrays.
	newco := fs.calls[1]
	if newco.Slug != "new-co" || !reflect.DeepEqual(newco.YcStatus, []string{"Active"}) {
		t.Errorf("newco slug/status = %q/%v", newco.Slug, newco.YcStatus)
	}
	if newco.YcBatch == nil || newco.Industries == nil {
		t.Error("newco arrays must be non-nil (NOT NULL columns)")
	}
}
