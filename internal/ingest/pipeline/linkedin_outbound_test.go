package pipeline

import (
	"context"
	"sync"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

type outboundRecorder struct {
	mu   sync.Mutex
	jobs []sources.Job
}

func (r *outboundRecorder) Resolve(_ context.Context, _ sources.CompanyEntry, j sources.Job) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.jobs = append(r.jobs, j)
	return nil
}

func TestLinkedInPublishesOnlyConfirmedOnsiteApplications(t *testing.T) {
	src := fakeSource{provider: "linkedin", jobs: []sources.Job{
		{ExternalID: "1", Title: "Go Engineer", Company: "Acme", Description: "Build Go services", URL: "https://www.linkedin.com/jobs/view/1/", ApplyMethod: sources.ApplyOnSite},
		{ExternalID: "2", Title: "Go Engineer", Company: "Acme", Description: "Build Go services", URL: "https://www.linkedin.com/jobs/view/2/", ApplyMethod: sources.ApplyExternal, ExternalApplyURL: "https://boards.greenhouse.io/acme/jobs/123"},
		{ExternalID: "3", Title: "Go Engineer", Company: "Acme", Description: "Build Go services", URL: "https://www.linkedin.com/jobs/view/3/"},
	}}
	store, outbound := &fakeStore{}, &outboundRecorder{}
	runner := Runner{Registry: registry(src), Store: store, Outbound: outbound}
	st, err := runner.Run(context.Background(), []sources.CompanyEntry{{Provider: "linkedin", Board: "Go"}})
	if err != nil || len(store.saved) != 1 || store.saved[0].Fields().ExternalID != "1" {
		t.Fatalf("LinkedIn public writes = %+v, %v", store.saved, err)
	}
	if len(outbound.jobs) != 2 || st.Total().Discovered != 2 {
		t.Fatalf("non-native discoveries = %d, stats %+v", len(outbound.jobs), st.Total())
	}
}

func TestDiscoveryDoesNotClaimThatTheSourceListingWasEmpty(t *testing.T) {
	if !boardListedAnyPosting(Stats{Discovered: 1}) {
		t.Fatal("a discovery-only listing is not an empty source feed")
	}
}
