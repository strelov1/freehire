package classify

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
)

// TestCorpusProbe is a throwaway probe, not a gate: it reads a prod corpus of titles
// the detector currently reads as unknown and prints which ones the dictionary now
// claims. Run it by hand while curating terms.
//
//	CLASSIFY_CORPUS=/tmp/corpus_all.txt go test ./internal/dict/classify/ -run TestCorpusProbe -v
func TestCorpusProbe(t *testing.T) {
	path := os.Getenv("CLASSIFY_CORPUS")
	if path == "" {
		t.Skip("set CLASSIFY_CORPUS to a 'title|count' file")
	}

	var claimedTitles, claimedPostings, totalPostings int
	var claimed []string
	scanCorpus(t, path, func(title string, n int) {
		totalPostings += n
		hit := IsTech(title)
		if hit {
			claimedTitles++
			claimedPostings += n
		}
		if hit == (os.Getenv("CLASSIFY_CORPUS_SHOW") != "missed") {
			claimed = append(claimed, fmt.Sprintf("%6d  %s", n, title))
		}
	})
	for _, c := range claimed {
		t.Log(c)
	}
	t.Logf("claimed %d titles / %d postings out of %d postings in corpus",
		claimedTitles, claimedPostings, totalPostings)
}

// TestCategoryCorpusProbe is the same idea aimed at one category rather than at
// `is_tech`: it reads a prod corpus of live titles and prints how many postings the
// dictionary now places in CLASSIFY_CATEGORY, and which titles it still misses. A
// throwaway probe, not a gate.
//
// It exists because a test written from the list that produced a dictionary can only
// ever confirm that list. Coverage is a question about the titles employers actually
// write, so it is measured against them.
//
//	CLASSIFY_CATEGORY_CORPUS=/tmp/hse_titles.txt CLASSIFY_CATEGORY=occupational_safety \
//	  go test ./internal/dict/classify/ -run TestCategoryCorpusProbe -v
func TestCategoryCorpusProbe(t *testing.T) {
	path := os.Getenv("CLASSIFY_CATEGORY_CORPUS")
	want := os.Getenv("CLASSIFY_CATEGORY")
	if path == "" || want == "" {
		t.Skip("set CLASSIFY_CATEGORY_CORPUS to a 'title|count' file and CLASSIFY_CATEGORY to the category")
	}
	var claimedPostings, unresolvedPostings, otherPostings, totalPostings int
	var missed []string
	scanCorpus(t, path, func(title string, n int) {
		totalPostings += n
		switch got := Parse(title).Category; got {
		case want:
			claimedPostings += n
		case "":
			unresolvedPostings += n
			missed = append(missed, fmt.Sprintf("%6d  %s", n, title))
		default:
			otherPostings += n
		}
	})

	t.Logf("corpus %d postings: %d (%.1f%%) → %s, %d (%.1f%%) unresolved, %d (%.1f%%) another category",
		totalPostings,
		claimedPostings, pct(claimedPostings, totalPostings), want,
		unresolvedPostings, pct(unresolvedPostings, totalPostings),
		otherPostings, pct(otherPostings, totalPostings))
	for _, m := range missed[:min(len(missed), 60)] {
		t.Log(m)
	}
}

func pct(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return 100 * float64(n) / float64(total)
}

// scanCorpus reads a 'title|count' corpus file and calls visit for each row. Shared by
// both probes: they ask different questions of the same file, and the parsing was
// duplicated between them.
//
// A malformed row fails the probe rather than being skipped. Every probe output is a
// coverage percentage, and a silently dropped row moves it without saying so — which
// would make the measurement exactly the kind of quiet wrong number these probes exist
// to remove.
func scanCorpus(t *testing.T, path string, visit func(title string, n int)) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open corpus: %v", err)
	}
	defer func() { _ = f.Close() }()

	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 1<<20), 1<<20)
	for line := 0; s.Scan(); {
		line++
		row := s.Text()
		if strings.TrimSpace(row) == "" {
			continue
		}
		i := strings.LastIndex(row, "|")
		if i < 0 {
			t.Fatalf("%s:%d: no '|' separator in %q", path, line, row)
		}
		n, err := strconv.Atoi(strings.TrimSpace(row[i+1:]))
		if err != nil {
			t.Fatalf("%s:%d: count %q: %v", path, line, row[i+1:], err)
		}
		visit(row[:i], n)
	}
	if err := s.Err(); err != nil {
		t.Fatalf("read corpus: %v", err)
	}
}
