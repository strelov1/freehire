package main

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func companyList(n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = "co"
		// Distinct enough for the error message; the loop never inspects the values.
		out[i] += string(rune('a' + i%26))
	}
	return out
}

// One dead batch must not starve the rest — that is the whole reason the pass is
// batched with per-batch fault isolation rather than one transaction.
func TestForCompanyBatchesIsolatesOneFailure(t *testing.T) {
	companies := companyList(3 * companyBatchSize)
	var calls int
	total, err := forCompanyBatches(context.Background(), companies, func(_ context.Context, batch []string) (int64, error) {
		calls++
		if calls == 2 {
			return 0, errors.New("statement timeout")
		}
		return int64(len(batch)), nil
	})
	if calls != 3 {
		t.Fatalf("ran %d batches, want all 3 attempted despite the failure", calls)
	}
	if want := int64(2 * companyBatchSize); total != want {
		t.Fatalf("total = %d, want %d — the two healthy batches must still count", total, want)
	}
	if err == nil {
		t.Fatal("a failed batch must surface as an aggregate error")
	}
}

// A cancelled context ends the pass. Every remaining batch would fail instantly
// against the same dead context, so continuing reports one deadline as hundreds of
// separate failures — which is exactly what obscured the 2026-08-16 timeout.
func TestForCompanyBatchesStopsOnCancellation(t *testing.T) {
	companies := companyList(10 * companyBatchSize)
	ctx, cancel := context.WithCancel(context.Background())
	var calls int
	total, err := forCompanyBatches(ctx, companies, func(ctx context.Context, batch []string) (int64, error) {
		calls++
		if calls == 2 {
			cancel()
			return 0, ctx.Err()
		}
		return int64(len(batch)), nil
	})
	if calls != 2 {
		t.Fatalf("ran %d batches, want it to stop at the cancelled one (2)", calls)
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want it to carry context.Canceled so the caller can tell a deadline from a bad batch", err)
	}
	// The count must be the batches that COMPLETED, not the ones that failed: the
	// cancellation branch runs before the failure is counted, so reporting failures
	// there would always say zero and hide how far the pass actually got.
	if !strings.Contains(err.Error(), "after 1 completed batches") {
		t.Fatalf("err = %q, want it to report the 1 batch that completed before the cancellation", err)
	}
	// Work done before the cancellation is still reported: the pass is best-effort
	// and its markers are already written.
	if want := int64(companyBatchSize); total != want {
		t.Fatalf("total = %d, want %d from the batch that completed", total, want)
	}
}
