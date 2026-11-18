//go:build integration

package worker

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestTryLockExcludesASecondHolderUntilReleased(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	const key int64 = 0x7e57_10c4

	release, ok, err := TryLock(ctx, pool, key)
	if err != nil || !ok {
		t.Fatalf("first TryLock = (%v, %v), want it to win a free lock", ok, err)
	}
	if rel2, ok, err := TryLock(ctx, pool, key); err != nil || ok {
		if ok {
			rel2()
		}
		t.Fatalf("second TryLock = (%v, %v), want it to lose while the first holds the key", ok, err)
	}

	release()
	rel3, ok, err := TryLock(ctx, pool, key)
	if err != nil || !ok {
		t.Fatalf("TryLock after release = (%v, %v), want it to win", ok, err)
	}
	rel3()
}

func TestTryLockReportsAnErrorRatherThanAWonLock(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, ok, err := TryLock(ctx, testdb.Pool(t), 0x7e57_10c5); err == nil || ok {
		t.Fatalf("TryLock on a cancelled context = (%v, %v), want an error and no lock", ok, err)
	}
}
