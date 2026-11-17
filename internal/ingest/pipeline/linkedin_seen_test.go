package pipeline

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// LinkedIn rows are stored under the bare global posting id, so the seen-set must be read
// provider-wide and keyed by that id, not by a board pattern and board:id.
func TestLinkedInSeenSetIsProviderWideAndKeyedByBareID(t *testing.T) {
	src := &fakeHydratingSource{provider: "linkedin", jobs: []sources.Job{
		{ExternalID: "4490679811", Title: "A", Company: "C", URL: "https://www.linkedin.com/jobs/view/4490679811/"},
		{ExternalID: "4490679812", Title: "B", Company: "C", URL: "https://www.linkedin.com/jobs/view/4490679812/"},
	}}
	entry := sources.CompanyEntry{Provider: "linkedin", Board: "software engineer", Region: "UK"}
	store := &fakeStore{seenIDs: map[string]bool{"4490679811": true}}
	r := Runner{Registry: registry(src), Store: store}
	if _, err := r.Run(context.Background(), []sources.CompanyEntry{entry}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.seenAsked) != 1 || store.seenAsked[0] != "" {
		t.Errorf("seen lookups scoped to %q, want one provider-wide lookup", store.seenAsked)
	}
	if !src.seenResults["4490679811"] || src.seenResults["4490679812"] {
		t.Errorf("seen results = %v, want only the stored id seen", src.seenResults)
	}
}

// A refreshed posting carries identity only: it must be touched by its bare id and never reach
// ATS discovery as a non-native posting (it has no ApplyMethod) nor be written.
func TestLinkedInSeenRefreshIsTouchedNotDiscovered(t *testing.T) {
	src := &fakeHydratingSource{provider: "linkedin", jobs: []sources.Job{
		{ExternalID: "4490679811", Title: "A", Company: "C", URL: "https://www.linkedin.com/jobs/view/4490679811/"},
		{ExternalID: "4490679812", Title: "B", Company: "C", URL: "https://www.linkedin.com/jobs/view/4490679812/"},
	}}
	outbound := &outboundRecorder{}
	store := &fakeStore{seenIDs: map[string]bool{"4490679811": true, "4490679812": true}}
	r := Runner{Registry: registry(src), Store: store, Outbound: outbound}
	st, err := r.Run(context.Background(), []sources.CompanyEntry{{Provider: "linkedin", Board: "software engineer", Region: "UK"}})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(outbound.jobs) != 0 || st.Total().Discovered != 0 || len(store.saved) != 0 {
		t.Errorf("seen postings reached discovery/save: outbound=%d discovered=%d saved=%d", len(outbound.jobs), st.Total().Discovered, len(store.saved))
	}
	if len(store.touched) != 2 || store.touched[0] != [2]string{"linkedin", "4490679811"} {
		t.Errorf("touched = %v, want both stored postings touched by bare id", store.touched)
	}
}
