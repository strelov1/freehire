package maildaily

import (
	"context"
	"errors"
	"testing"
)

func TestClaimer_FirstTakerWinsTheDay(t *testing.T) {
	mem := &Memory{}
	first, second := New(mem, SourceOnboarding), New(mem, SourceCVLifecycle)
	ctx := context.Background()

	claim, ok, err := first.Take(ctx, 7)
	if err != nil || !ok || claim.UserID != 7 {
		t.Fatalf("first Take = %+v, %v, %v", claim, ok, err)
	}
	if _, ok, err := second.Take(ctx, 7); err != nil || ok {
		t.Fatalf("second Take = %v, %v; want a lost claim with no error", ok, err)
	}
	if _, ok, _ := second.Take(ctx, 8); !ok {
		t.Fatal("another account's day was not free")
	}
	if got := mem.Holder(7); got != SourceOnboarding {
		t.Errorf("holder = %q", got)
	}
}

func TestClaimer_OnlyTheTakerReleases(t *testing.T) {
	mem := &Memory{}
	first, second := New(mem, SourceOnboarding), New(mem, SourceBroadcast)
	ctx := context.Background()
	claim, _, _ := first.Take(ctx, 7)

	if err := second.Release(ctx, claim); err != nil {
		t.Fatal(err)
	}
	if mem.Holder(7) != SourceOnboarding {
		t.Fatal("another sender released the claim")
	}
	if err := first.Release(ctx, claim); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := second.Take(ctx, 7); !ok {
		t.Fatal("the day was not free after the release")
	}
}

func TestClaimer_StoreErrorsAreReported(t *testing.T) {
	boom := errors.New("db down")
	mem := &Memory{ClaimErr: boom}
	c := New(mem, SourceLimitNudge)
	if _, ok, err := c.Take(context.Background(), 7); ok || !errors.Is(err, boom) {
		t.Fatalf("Take = %v, %v; want the store error and no claim", ok, err)
	}

	mem = &Memory{ReleaseErr: boom}
	c = New(mem, SourceLimitNudge)
	claim, _, _ := c.Take(context.Background(), 7)
	if err := c.Release(context.Background(), claim); !errors.Is(err, boom) {
		t.Fatalf("Release err = %v", err)
	}
	if mem.Releases != 1 {
		t.Errorf("Releases = %d", mem.Releases)
	}
}
