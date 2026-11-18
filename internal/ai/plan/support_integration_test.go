//go:build integration

package plan

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSupportStandingFollowsConsumeSupport(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	cfg := enforcing()
	cfg.SupportMessagesPerDay = 2
	s, pool := newStore(t, cfg, now)
	ctx := context.Background()
	user := insertSubscriber(t, pool, "support-standing@example.test")

	got, err := s.SupportStanding(ctx, user)
	if err != nil || got.Used != 0 || got.Remaining != 2 || got.Limit != 2 {
		t.Fatalf("fresh standing = %+v, %v; want used 0, remaining 2", got, err)
	}
	if !got.ResetsAt.Equal(time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("ResetsAt = %v, want the next UTC midnight", got.ResetsAt)
	}
	for _, ref := range []string{"a", "b"} {
		if _, err := s.ConsumeSupport(ctx, user, ref); err != nil {
			t.Fatalf("ConsumeSupport %s: %v", ref, err)
		}
	}
	if _, err := s.ConsumeSupport(ctx, user, "c"); !errors.Is(err, ErrRefused) {
		t.Fatalf("third message = %v, want ErrRefused", err)
	}
	got, _ = s.SupportStanding(ctx, user)
	if got.Used != 2 || got.Remaining != 0 {
		t.Errorf("after refusal = %+v, want used 2, remaining 0", got)
	}
	if err := s.Release(ctx, user, FeatureSupport, "a"); err != nil {
		t.Fatal(err)
	}
	got, _ = s.SupportStanding(ctx, user)
	if got.Used != 1 || got.Remaining != 1 {
		t.Errorf("after release = %+v, want used 1, remaining 1", got)
	}
}
