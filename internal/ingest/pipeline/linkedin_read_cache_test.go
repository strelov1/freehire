package pipeline

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// readRecordingStore adds the PostingReadRecorder capability to the shared fake.
type readRecordingStore struct {
	*fakeStore
	rmu  sync.Mutex
	read map[string]string
}

func (s *readRecordingStore) RecordLinkedInRead(_ context.Context, id, method string) error {
	s.rmu.Lock()
	defer s.rmu.Unlock()
	if s.read == nil {
		s.read = map[string]string{}
	}
	s.read[id] = method
	return nil
}

func TestLinkedInNonNativeReadIsRecordedAfterResolution(t *testing.T) {
	src := fakeSource{provider: "linkedin", jobs: []sources.Job{
		{ExternalID: "1", Title: "A", Company: "C", Description: "d", URL: "https://www.linkedin.com/jobs/view/1/", ApplyMethod: sources.ApplyOnSite},
		{ExternalID: "2", Title: "A", Company: "C", Description: "d", URL: "https://www.linkedin.com/jobs/view/2/", ApplyMethod: sources.ApplyExternal},
		{ExternalID: "3", Title: "A", Company: "C", Description: "d", URL: "https://www.linkedin.com/jobs/view/3/"},
	}}
	store := &readRecordingStore{fakeStore: &fakeStore{}}
	r := Runner{Registry: registry(src), Store: store, Outbound: &outboundRecorder{}}
	if _, err := r.Run(context.Background(), []sources.CompanyEntry{{Provider: "linkedin", Board: "Go", Region: "UK"}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.read) != 2 || store.read["2"] != "external" || store.read["3"] != "unknown" {
		t.Errorf("recorded reads = %v, want 2:external and 3:unknown only", store.read)
	}
}

type failingOutbound struct{}

func (failingOutbound) Resolve(context.Context, sources.CompanyEntry, sources.Job) error {
	return errors.New("resolve failed")
}

// A failed resolution must stay retryable: caching the posting would suppress the retry.
func TestLinkedInFailedResolutionIsNotRecorded(t *testing.T) {
	src := fakeSource{provider: "linkedin", jobs: []sources.Job{
		{ExternalID: "2", Title: "A", Company: "C", Description: "d", URL: "https://www.linkedin.com/jobs/view/2/", ApplyMethod: sources.ApplyExternal},
	}}
	store := &readRecordingStore{fakeStore: &fakeStore{}}
	r := Runner{Registry: registry(src), Store: store, Outbound: failingOutbound{}}
	if _, err := r.Run(context.Background(), []sources.CompanyEntry{{Provider: "linkedin", Board: "Go", Region: "UK"}}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(store.read) != 0 {
		t.Errorf("recorded %v after a failed resolution", store.read)
	}
}
