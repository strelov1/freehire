package pipeline

import (
	"context"
	"errors"
	"fmt"
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
