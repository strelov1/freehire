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
	for s.Scan() {
		line := s.Text()
		i := strings.LastIndex(line, "|")
		if i < 0 {
			continue
		}
		title := line[:i]
		n, _ := strconv.Atoi(line[i+1:])
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
