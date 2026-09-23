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
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open corpus: %v", err)
	}
	defer func() { _ = f.Close() }()

	var claimedTitles, claimedPostings, totalPostings int
	var claimed []string
	s := bufio.NewScanner(f)
	s.Buffer(make([]byte, 1<<20), 1<<20)
	for line := 0; s.Scan(); {
		line++
		row := s.Text()
		if strings.TrimSpace(row) == "" {
			continue
		}
		// A malformed row fails the probe rather than being skipped. The whole output
		// is a coverage percentage, and a silently dropped row moves it without
		// saying so — which would make this measurement exactly the kind of quiet
		// wrong number the change exists to remove.
		i := strings.LastIndex(row, "|")
		if i < 0 {
			t.Fatalf("%s:%d: no '|' separator in %q", path, line, row)
		}
		title := row[:i]
		n, err := strconv.Atoi(strings.TrimSpace(row[i+1:]))
		if err != nil {
			t.Fatalf("%s:%d: count %q: %v", path, line, row[i+1:], err)
		}
		totalPostings += n
		hit := IsTech(title)
		if hit {
			claimedTitles++
			claimedPostings += n
		}
		if hit == (os.Getenv("CLASSIFY_CORPUS_SHOW") != "missed") {
			claimed = append(claimed, fmt.Sprintf("%6d  %s", n, title))
		}
	}
	for _, c := range claimed {
		t.Log(c)
	}
	t.Logf("claimed %d titles / %d postings out of %d postings in corpus",
		claimedTitles, claimedPostings, totalPostings)
}
