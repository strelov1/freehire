package pipeline

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

func TestUnreadablePostingRefreshesLivenessWithoutSavingContent(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "refresh", true: "failure"}[fail], func(t *testing.T) {
			store := &fakeStore{}
			if fail {
				store.err = errors.New("touch failed")
			}
			r := Runner{Store: store}
			var tally boardTally
			r.ingestOne(context.Background(), sources.CompanyEntry{Provider: "lever", Board: "acme"},
				sources.Job{ExternalID: "1", Unreadable: true}, nil, nil, nil, &tally)
			if tally.st.Unreadable != 1 || tally.st.Ingested != 0 || len(store.saved) != 0 {
				t.Fatalf("unreadable posting was not preserved: %+v, saved=%d", tally.st, len(store.saved))
			}
			if fail {
				if tally.st.Skipped != 1 || !errors.Is(tally.firstErr, store.err) {
					t.Fatalf("touch error lost: %+v", tally)
				}
			} else if len(store.touched) != 1 {
				t.Fatalf("listing did not refresh liveness: %v", store.touched)
			}
		})
	}
}
