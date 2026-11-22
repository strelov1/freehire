package pipeline

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

func TestUnreadablePostingRefreshesOpenRowsWithoutDoubleCounting(t *testing.T) {
	for _, readable := range []int{18, 19} {
		for _, fail := range []bool{false, true} {
			t.Run(fmt.Sprintf("readable=%d/fail=%t", readable, fail), func(t *testing.T) {
				store := &fakeStore{}
				if fail {
					store.refreshErr = errors.New("refresh failed")
				}
				r := Runner{Store: store}
				e := sources.CompanyEntry{Provider: "lever", Board: "acme", Company: "Acme"}
				raw := []sources.Job{{ExternalID: "unreadable", Company: "Acme", Unreadable: true}}
				for i := 0; i < readable; i++ {
					raw = append(raw, sources.Job{ExternalID: fmt.Sprint(i), Company: "Acme", Title: "Backend Engineer", SeenRefresh: true})
				}
				st := r.ingestFetched(context.Background(), e, fetchedBoard{jobs: raw})
				if st.Unreadable != 1 || st.Skipped != 0 || st.Ingested != readable || len(store.saved) != 0 {
					t.Fatalf("posting counted twice or content overwritten: %+v, saved=%d", st, len(store.saved))
				}
				if len(store.touched) != readable {
					t.Fatalf("unreadable posting used the reopening Touch path: %v", store.touched)
				}
				if got, want := boardReadWhatItListed(st), readable == 19; got != want {
					t.Fatalf("unreadable share qualifies=%t,want %t: %+v", got, want, st)
				}
				if fail {
					if st.RefreshFailed != 1 || boardQualifies(e, st) {
						t.Fatalf("failed refresh allowed sweep: %+v", st)
					}
				} else {
					ids := store.refreshed["lever"]
					if st.Refreshed != 1 || len(ids) != 1 || ids[0] != "acme:unreadable" {
						t.Fatalf("open-only refresh lost identity: %+v,ids=%v", st, ids)
					}
				}
			})
		}
	}
}

func TestUnreadableStreamingPostingRefreshesWithoutSaving(t *testing.T) {
	src := fakeStreamingSource{provider: "eightfold", failAfter: -1, jobs: []sources.Job{
		{ExternalID: "1", Title: "Data Engineer", Company: "Acme", URL: "https://example.com/jobs/1"},
		{ExternalID: "2", Title: "Unusable replacement", Unreadable: true},
	}}
	store := &fakeStore{refreshCompanies: []string{"stored-employer"}}
	r := Runner{Registry: registry(src), Store: store}
	e := sources.CompanyEntry{Provider: "eightfold", Board: "host.example/dom", Company: "Acme"}
	stats, err := r.Run(context.Background(), []sources.CompanyEntry{e})
	if err != nil {
		t.Fatal(err)
	}
	st := stats.Total()
	if st.Unreadable != 1 || st.Refreshed != 1 || st.Ingested != 1 || st.Skipped != 0 || st.Failed != 0 {
		t.Fatalf("stats = %+v, want one saved posting and one unreadable identity refresh", st)
	}
	if len(store.saved) != 1 || store.saved[0].Fields().ExternalID != "host.example/dom:1" || len(store.touched) != 0 {
		t.Fatalf("unexpected content or reopening writes: saved=%+v touched=%+v", store.saved, store.touched)
	}
	if got := store.refreshed["eightfold"]; !slices.Equal(got, []string{"host.example/dom:2"}) {
		t.Errorf("open-only refresh got %v", got)
	}
	if !slices.Contains(st.UnprovenCompanies, "stored-employer") {
		t.Errorf("unproven companies = %v, want the unreadable posting's stored employer", st.UnprovenCompanies)
	}
}

func TestUnreadableBoardWithholdsStoredFranchiseCompany(t *testing.T) {
	for _, unknown := range []bool{false, true} {
		t.Run(fmt.Sprintf("seenUnknown=%t", unknown), func(t *testing.T) {
			store := &fakeStore{refreshCompanies: []string{"franchise"}}
			r := Runner{Store: store}
			e := sources.CompanyEntry{Provider: "careerplug", Board: "corporate", Company: "Configured Employer"}
			st := r.ingestFetched(context.Background(), e, fetchedBoard{
				jobs:        []sources.Job{{ExternalID: "1", Company: "Configured Employer", Unreadable: true}},
				seenUnknown: unknown,
			})
			for _, slug := range []string{"configured-employer", "franchise"} {
				if !slices.Contains(st.UnprovenCompanies, slug) {
					t.Fatalf("unproven companies=%v, want configured and actual stored employers", st.UnprovenCompanies)
				}
			}
		})
	}
}
