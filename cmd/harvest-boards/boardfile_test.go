package main

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// identityKey is the default dedup key: the board id verbatim (case-sensitive).
func identityKey(s string) string { return s }

func TestNewBoards(t *testing.T) {
	existing := map[string]bool{"acme": true, "Globex": true}
	got := newBoards([]string{"acme", "Globex", "initech", "globex", "initech"}, existing, identityKey)
	// case-sensitive ("globex" != "Globex" survives) and de-duplicated within the seed.
	want := []string{"initech", "globex"}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v, want %v", got, want)
		}
	}
}

// Workday board ids are case-insensitive, so a lowercased harvest twin of an existing
// CamelCase board must dedup away (else the same board is crawled twice).
func TestNewBoardsCaseInsensitiveKey(t *testing.T) {
	existing := map[string]bool{"acme.wd1.myworkdayjobs.com/Careers": true}
	key := func(s string) string { return strings.ToLower(s) }
	got := newBoards([]string{"acme.wd1.myworkdayjobs.com/careers", "new.wd1.myworkdayjobs.com/x"}, existing, key)
	if len(got) != 1 || got[0] != "new.wd1.myworkdayjobs.com/x" {
		t.Fatalf("got %v, want [new.wd1.myworkdayjobs.com/x]", got)
	}
}

func TestDedupKeyOf(t *testing.T) {
	if dedupKeyOf(workdayProber{})("A.WD1.myworkdayjobs.com/Site") != "a.wd1.myworkdayjobs.com/site" {
		t.Error("workday key must fold case")
	}
	// Greenhouse's boards API is case-insensitive too (confirmed live:
	// boards-api.greenhouse.io/v1/boards/adyen and .../Adyen return the same company).
	if dedupKeyOf(greenhouseProber{})("Adyen") != "adyen" {
		t.Error("greenhouse key must fold case")
	}
	// Ashby's job-board API is case-insensitive (confirmed live: /posting-api/job-board/abridge
	// and .../Abridge return the same company), so the dedup key folds case like Workday's.
	if dedupKeyOf(ashbyProber{})("Ramp") != "ramp" {
		t.Error("ashby key must fold case")
	}
	// SmartRecruiters' postings API is case-insensitive too (confirmed live twice, #1884/#1888:
	// a name-derived candidate slipped past dedup because it only differed in case from an
	// existing board), so the dedup key folds case like the others.
	if dedupKeyOf(smartRecruitersProber{})("ArchirodonGroup") != "archirodongroup" {
		t.Error("smartrecruiters key must fold case")
	}
}

func TestWorkdayBoardID(t *testing.T) {
	w := workdayProber{}
	if got := w.boardID("aig|wd1|early_careers"); got != "aig.wd1.myworkdayjobs.com/early_careers" {
		t.Errorf("got %q", got)
	}
	// not three parts => unchanged
	if got := w.boardID("already.wd1.myworkdayjobs.com/Site"); got != "already.wd1.myworkdayjobs.com/Site" {
		t.Errorf("passthrough got %q", got)
	}
}

func TestMapSeeds(t *testing.T) {
	// a non-mapping prober leaves the seed unchanged
	if got := mapSeeds(greenhouseProber{}, []string{"a", "b"}); got[0] != "a" || got[1] != "b" {
		t.Errorf("identity got %v", got)
	}
	// workday maps tenant|dc|site -> host/site
	got := mapSeeds(workdayProber{}, []string{"aig|wd1|early_careers"})
	if got[0] != "aig.wd1.myworkdayjobs.com/early_careers" {
		t.Errorf("workday got %v", got)
	}
}

func TestAppendEntries(t *testing.T) {
	existing := "- company: Acme\n  board: acme\n"
	out, err := appendEntries(existing, []entry{
		{Company: "Initech", Board: "initech"},
		{Company: "Findhelp, A PBC", Board: "findhelp"},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Existing preserved verbatim; additions sorted by board (findhelp < initech). A comma
	// in a block scalar is not a YAML flow indicator, so yaml.v3 leaves the name unquoted.
	want := "- company: Acme\n  board: acme\n" +
		"- company: Findhelp, A PBC\n  board: findhelp\n" +
		"- company: Initech\n  board: initech\n"
	if out != want {
		t.Fatalf("got:\n%q\nwant:\n%q", out, want)
	}

	// Round-trip guard: the merged file parses back into the union of all entries.
	var entries []entry
	if err := yaml.Unmarshal([]byte(out), &entries); err != nil {
		t.Fatalf("merged output does not parse: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("round-trip: got %d entries, want 3", len(entries))
	}
}
