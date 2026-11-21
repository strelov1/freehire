package main

import (
	"context"
	"errors"
	"fmt"
	"slices"
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
	}, func(string) {})
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
	}, func(string) {})
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

// The resume point advances past every batch the walk gets past, failed ones included (a batch
// that always fails must not pin the walk), and never past one a cancellation cut off.
func TestForCompanyBatchesCheckpointsFinishedBatchesOnly(t *testing.T) {
	companies := chunkable([]string{"a", "b", "c", "d", "e"})
	ctx, cancel := context.WithCancel(context.Background())
	var calls int
	var points []string
	_, _ = forCompanyBatches(ctx, companies, func(ctx context.Context, batch []string) (int64, error) {
		calls++
		switch calls {
		case 2:
			return 0, errors.New("statement timeout")
		case 3:
			cancel()
			return 0, ctx.Err()
		}
		return 1, nil
	}, func(point string) { points = append(points, point) })
	if want := lastOfBatches(companies)[:2]; !slices.Equal(points, want) {
		t.Fatalf("checkpoints = %v, want the last company of the two batches before the cancelled one, %v", points, want)
	}
}

// chunkable spreads companies so each lands in its own batch.
func chunkable(companies []string) []string {
	out := make([]string, 0, len(companies)*companyBatchSize)
	for _, c := range companies {
		for i := range companyBatchSize {
			out = append(out, fmt.Sprintf("%s-%04d", c, i))
		}
	}
	return out
}

func lastOfBatches(companies []string) []string {
	var out []string
	for batch := range slices.Chunk(companies, companyBatchSize) {
		out = append(out, batch[len(batch)-1])
	}
	return out
}
