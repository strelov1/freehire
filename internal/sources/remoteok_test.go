package sources

import (
	"context"
	"slices"
	"testing"
)

func TestRemoteOKProvider(t *testing.T) {
	if got := NewRemoteOK(nil).Provider(); got != "remoteok" {
		t.Errorf("Provider() = %q, want remoteok", got)
	}
}

func TestRemoteOKIsBoardlessAggregator(t *testing.T) {
	s := NewRemoteOK(nil)
	if _, ok := s.(boardless); !ok {
		t.Error("remoteok should implement the boardless marker")
	}
	if _, ok := s.(aggregator); !ok {
		t.Error("remoteok should implement the aggregator marker")
	}
}

func TestRemoteOKRegisteredAndFilterable(t *testing.T) {
	if _, ok := All(nil)["remoteok"]; !ok {
		t.Error("All() should register provider remoteok")
	}
	if !slices.Contains(FilterableProviders(), "remoteok") {
		t.Error("FilterableProviders() should include remoteok")
	}
}

func TestRemoteOKBoardFileValidates(t *testing.T) {
	cfg, err := LoadConfig("../../sources/remoteok.yml")
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if err := cfg.Validate(All(nil)); err != nil {
		t.Fatalf("sources/remoteok.yml fails validation: %v", err)
	}
}

func TestRemoteOKFetchSkipsLegalNoticeAndMaps(t *testing.T) {
	// The /api feed's first element is a legal notice (no id); it must be dropped.
	feed := `[
{"legal":"API Terms: link back"},
{"id":"1133522","slug":"cs-rep-dario-1133522","company":"Dario","position":"Customer Support Rep","description":"<p>Help users.</p>","location":"India","date":"2026-06-16T17:47:36+00:00","url":"https://remoteOK.com/remote-jobs/cs-rep-dario-1133522"}
]`
	fake := (&routedHTTP{}).route("remoteok.com/api", feed)
	jobs, err := NewRemoteOK(fake).Fetch(context.Background(), CompanyEntry{})
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if len(jobs) != 1 {
		t.Fatalf("got %d jobs, want 1 (legal notice dropped)", len(jobs))
	}
	j := jobs[0]
	if j.ExternalID != "1133522" || j.Company != "Dario" || j.Title != "Customer Support Rep" {
		t.Errorf("bad mapping: %+v", j)
	}
	if j.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote", j.WorkMode)
	}
	if j.PostedAt == nil {
		t.Error("PostedAt nil, want parsed RFC3339")
	}
}
