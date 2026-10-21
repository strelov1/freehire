package search

import "testing"

func docsN(n int) []JobDocument {
	out := make([]JobDocument, n)
	for i := range out {
		out[i].ID = int64(i + 1)
	}
	return out
}

// A hybrid query asks for one hit more than the page. The extra is dropped and only
// tells the caller a next page exists, so the total can be a true lower bound instead
// of the engine's estimate, which under a vector search is the whole index.
func TestHybridPage_FullPageReportsMoreThanItShows(t *testing.T) {
	got := hybridPage(docsN(11), 10, 20)
	if len(got.Hits) != 10 {
		t.Fatalf("hits = %d, want the page of 10 without the probe", len(got.Hits))
	}
	if got.Total != 31 || !got.TotalIsLowerBound {
		t.Errorf("total = %d lowerBound=%v, want 31 (20 skipped + 10 shown + at least 1 more) and a lower bound",
			got.Total, got.TotalIsLowerBound)
	}
}

func TestHybridPage_ShortPageIsTheEnd(t *testing.T) {
	got := hybridPage(docsN(4), 10, 0)
	if len(got.Hits) != 4 || got.Total != 4 || !got.TotalIsLowerBound {
		t.Errorf("hits=%d total=%d lowerBound=%v, want 4, 4, lower bound", len(got.Hits), got.Total, got.TotalIsLowerBound)
	}
}

func TestHybridPage_EmptyPage(t *testing.T) {
	got := hybridPage(nil, 10, 30)
	if len(got.Hits) != 0 || got.Total != 30 {
		t.Errorf("hits=%d total=%d, want 0 hits and the 30 already skipped", len(got.Hits), got.Total)
	}
}
