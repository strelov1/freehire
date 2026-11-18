package notify

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

type ctxCheckingStore struct {
	*fakeStore
	liveWithDeadline bool
}

func (s *ctxCheckingStore) ReleaseMatchClaim(ctx context.Context, a db.ReleaseMatchClaimParams) error {
	_, ok := ctx.Deadline()
	s.liveWithDeadline = ctx.Err() == nil && ok
	return s.fakeStore.ReleaseMatchClaim(ctx, a)
}

func TestRelease_SurvivesCancelledRunContext(t *testing.T) {
	store := &ctxCheckingStore{fakeStore: &fakeStore{}}
	r := New(store, &fakeSearcher{}, &fakeNotifier{}, DefaultConfig())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r.release(ctx, 1, []int64{10})
	if !store.liveWithDeadline || len(store.released) != 1 {
		t.Fatalf("release ran on a dead or unbounded context (live+deadline=%v, released=%v)", store.liveWithDeadline, store.released)
	}
}
