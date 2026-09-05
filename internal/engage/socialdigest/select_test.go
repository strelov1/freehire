package socialdigest

import "testing"

// posting builds a candidate. Views default high enough to clear the floor so a test
// that is not about the floor does not have to think about it.
func posting(id int64, companySlug string, views int) Posting {
	return Posting{
		JobID:       id,
		Slug:        "job-" + companySlug,
		Title:       "Engineer",
		Company:     companySlug,
		CompanySlug: companySlug,
		PageUniques: views,
	}
}

// ids is what almost every assertion below compares — the identity and the order of
// the published list, which is the whole output of these rules.
func ids(list []Posting) []int64 {
	out := make([]int64, 0, len(list))
	for _, p := range list {
		out = append(out, p.JobID)
	}
	return out
}

func equalIDs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func assertIDs(t *testing.T, got []Posting, want []int64) {
	t.Helper()
	if g := ids(got); !equalIDs(g, want) {
		t.Errorf("got %v, want %v", g, want)
	}
}

func TestSelect(t *testing.T) {
	t.Run("keeps the ranking order it was given", func(t *testing.T) {
		got := Select([]Posting{
			posting(1, "alpha", 90),
			posting(2, "beta", 50),
			posting(3, "gamma", 20),
		}, nil)
		assertIDs(t, got, []int64{1, 2, 3})
	})

	t.Run("drops a posting below the view floor", func(t *testing.T) {
		got := Select([]Posting{
			posting(1, "alpha", MinPageUniques),
			posting(2, "beta", MinPageUniques-1),
		}, nil)
		assertIDs(t, got, []int64{1})
	})

	t.Run("a day where nothing clears the floor yields nothing", func(t *testing.T) {
		got := Select([]Posting{
			posting(1, "alpha", 9),
			posting(2, "beta", 3),
		}, nil)
		if len(got) != 0 {
			t.Errorf("got %v, want empty", ids(got))
		}
	})

	t.Run("drops a quarantined posting", func(t *testing.T) {
		got := Select([]Posting{
			posting(1, "alpha", 90),
			posting(2, "beta", 50),
		}, map[int64]bool{1: true})
		assertIDs(t, got, []int64{2})
	})

	t.Run("caps a company at MaxPerCompany", func(t *testing.T) {
		got := Select([]Posting{
			posting(1, "alpha", 90),
			posting(2, "alpha", 80),
			posting(3, "alpha", 70),
			posting(4, "beta", 60),
		}, nil)
		assertIDs(t, got, []int64{1, 2, 4})
	})

	t.Run("the cap keeps a company's highest-ranked postings", func(t *testing.T) {
		got := Select([]Posting{
			posting(1, "alpha", 90),
			posting(2, "beta", 85),
			posting(3, "alpha", 80),
			posting(4, "alpha", 75),
		}, nil)
		assertIDs(t, got, []int64{1, 2, 3})
	})

	t.Run("takes at most Size postings", func(t *testing.T) {
		var candidates []Posting
		for i := 1; i <= Size+5; i++ {
			candidates = append(candidates, posting(int64(i), "company-"+string(rune('a'+i)), 100-i))
		}
		got := Select(candidates, nil)
		if len(got) != Size {
			t.Fatalf("got %d postings, want %d", len(got), Size)
		}
		assertIDs(t, got, []int64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10})
	})

	// This test encodes the open decision documented on Select: the cap runs BEFORE
	// the list is truncated, so a capped posting is replaced by the next eligible one
	// and a normal day yields a full list. Flip this test to change the rule.
	t.Run("a capped posting is replaced, not left as a hole", func(t *testing.T) {
		var candidates []Posting
		// The three highest-viewed postings all belong to one company; only two survive.
		candidates = append(candidates,
			posting(1, "alpha", 100),
			posting(2, "alpha", 99),
			posting(3, "alpha", 98),
		)
		for i := 4; i <= Size+3; i++ {
			candidates = append(candidates, posting(int64(i), "company-"+string(rune('a'+i)), 100-i))
		}
		got := Select(candidates, nil)
		if len(got) != Size {
			t.Fatalf("got %d postings, want a full list of %d", len(got), Size)
		}
		for _, p := range got {
			if p.JobID == 3 {
				t.Error("the third alpha posting should have been capped out")
			}
		}
	})

	t.Run("no candidates at all yields nothing", func(t *testing.T) {
		if got := Select(nil, nil); len(got) != 0 {
			t.Errorf("got %v, want empty", ids(got))
		}
	})

	// Quarantine and cap must compose: a quarantined posting is gone before the cap
	// counts it, so its company still gets MaxPerCompany slots from what is left.
	t.Run("quarantine does not consume a company's cap", func(t *testing.T) {
		got := Select([]Posting{
			posting(1, "alpha", 90),
			posting(2, "alpha", 80),
			posting(3, "alpha", 70),
		}, map[int64]bool{1: true})
		assertIDs(t, got, []int64{2, 3})
	})
}
