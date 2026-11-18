package main

import (
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ingest/pipeline"
	sweeppolicy "github.com/strelov1/freehire/internal/ingest/sweep"
)

// HYDRATION_RETRY_DAYS widens the window during which a body-less row is re-offered for
// detail hydration, so an operator can repair a backlog the ordinary two-week window has
// already aged past (freehire#1866). Unset means the default; a value that is not a positive
// number is a config error, not a silent fallback — a typo on a repair run would otherwise
// look like a run that repaired nothing.
func TestHydrationRetryWindowFor(t *testing.T) {
	cases := []struct {
		name    string
		env     string
		want    time.Duration
		wantErr bool
	}{
		{"unset keeps the default", "", pipeline.HydrationRetryWindow, false},
		{"widened for a repair run", "365", 365 * 24 * time.Hour, false},
		{"narrowed", "1", 24 * time.Hour, false},
		{"not a number", "two weeks", 0, true},
		{"zero", "0", 0, true},
		{"negative", "-5", 0, true},
	}
	for _, tc := range cases {
		got, err := hydrationRetryWindowFor(tc.env)
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: hydrationRetryWindowFor(%q) err = %v, wantErr %v", tc.name, tc.env, err, tc.wantErr)
			continue
		}
		if err == nil && got != tc.want {
			t.Errorf("%s: hydrationRetryWindowFor(%q) = %v, want %v", tc.name, tc.env, got, tc.want)
		}
	}
}

// INGEST_REFETCH_ALL empties the seen-set so a crawl re-writes the provider's stored rows —
// the only way an adapter fix reaches postings ingested before it, since a re-listed posting
// otherwise takes the content-less refresh path. Anything but the two accepted spellings is a
// config error rather than a silent false, for the same reason HYDRATION_RETRY_DAYS is.
func TestRefetchAllFor(t *testing.T) {
	cases := []struct {
		name    string
		env     string
		want    bool
		wantErr bool
	}{
		{"unset is an ordinary crawl", "", false, false},
		{"one", "1", true, false},
		{"true", "true", true, false},
		{"zero is not a spelling of off", "0", false, true},
		{"typo", "yes", false, true},
	}
	for _, tc := range cases {
		got, err := refetchAllFor(tc.env)
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: refetchAllFor(%q) err = %v, wantErr %v", tc.name, tc.env, err, tc.wantErr)
			continue
		}
		if err == nil && got != tc.want {
			t.Errorf("%s: refetchAllFor(%q) = %v, want %v", tc.name, tc.env, got, tc.want)
		}
	}
}

// defaultSeenPolicy is the seen-set policy a deployment with no knobs set gets: retry a
// body-less row inside the default window, re-fetch nothing else. It is what the store
// integration tests construct, so a test asserting a WRITE never accidentally asserts a
// re-fetch policy as well.
func defaultSeenPolicy() seenPolicy {
	return seenPolicy{
		hydrationWindow: pipeline.HydrationRetryWindow,
		bodies:          sweeppolicy.BodyRefresh{Slices: 1, Slot: sweeppolicy.BodyRefreshDisabledSlot},
	}
}

// A red ingest unit must mean the run is broken. A provider of thousands of boards always has
// a few failing ones, and those belong to board_health, not to the exit code.
func TestCrawlReachedNothing(t *testing.T) {
	cases := []struct {
		name  string
		total pipeline.Stats
		want  bool
	}{
		{"a few dead boards beside a working crawl", pipeline.Stats{Ingested: 26362, Failed: 5}, false},
		{"every posting covered, so only refreshed", pipeline.Stats{Refreshed: 40, Failed: 1}, false},
		{"every board failed", pipeline.Stats{Failed: 12}, true},
		{"every save failed", pipeline.Stats{Skipped: 4149}, true},
		{"nothing to do: every board quiet", pipeline.Stats{Quiet: 300}, false},
	}
	for _, tc := range cases {
		if got := crawlReachedNothing(tc.total); got != tc.want {
			t.Errorf("%s: crawlReachedNothing(%+v) = %t, want %t", tc.name, tc.total, got, tc.want)
		}
	}
}

func TestRecentWindowFor(t *testing.T) {
	for _, tc := range []struct {
		env     string
		want    time.Duration
		wantErr bool
	}{
		{"", 0, false},
		{"1", 24 * time.Hour, false},
		{"3", 72 * time.Hour, false},
		{"30", 30 * 24 * time.Hour, false},
		{"0", 0, true},
		{"-2", 0, true},
		{"31", 0, true},
		{"abc", 0, true},
		{"1.5", 0, true},
		{" 3", 0, true},
	} {
		got, err := recentWindowFor(tc.env)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("recentWindowFor(%q) = %v, %v; want %v, err=%t", tc.env, got, err, tc.want, tc.wantErr)
		}
	}
}

// A recent run must not reach the database at all: nil queries would panic if any close ran.
func TestSweepDoesNothingInRecentMode(t *testing.T) {
	stats := pipeline.RunStats{"adzuna": {Ingested: 5, QualifyingBoards: []pipeline.BoardRef{{Board: "it-jobs", Region: "gb"}}}}
	if failed := sweep(t.Context(), nil, nil, stats, newCrawledSet(), nil, false, true); failed != 0 {
		t.Fatalf("failed = %d, want 0", failed)
	}
}

func noEnv(string) string { return "" }

func TestParseArgs_RejectsWhatTodayRunsAsAFullCrawl(t *testing.T) {
	for _, args := range [][]string{
		{"greenhouse", "--boad=acme"},
		{"greenhouse", "-board=acme"},
		{"greenhouse", "--shardd=2/6"},
		{"linkedin", "--reconcile-linkedn"},
		{"greenhouse", "lever"},
		{"--board=x"},                 // no provider
		{"--boad=acme", "greenhouse"}, // flag before the provider
	} {
		if _, err := parseArgs(args, noEnv); err == nil {
			t.Errorf("parseArgs(%q) accepted, want an error", args)
		}
	}
}

func TestParseArgs_AcceptsEveryExistingForm(t *testing.T) {
	cases := []struct {
		args []string
		env  map[string]string
		want cliArgs
	}{
		{[]string{"greenhouse"}, nil, cliArgs{provider: "greenhouse"}},
		{[]string{"greenhouse", "--shard=2/6"}, nil, cliArgs{provider: "greenhouse", shardSpec: "2/6"}},
		{[]string{"x", "--board=X", "--region=IT"}, nil, cliArgs{provider: "x", boardFilter: "X", regionFilter: "IT"}},
		{[]string{"linkedin", "--reconcile-linkedin", "--reconcile-after=5"}, nil, cliArgs{provider: "linkedin", reconcileLinkedIn: true, reconcileAfter: 5}},
		{nil, map[string]string{"INGEST_PROVIDER": "lever", "SHARD": "1/3"}, cliArgs{provider: "lever", shardSpec: "1/3"}},
	}
	for _, tc := range cases {
		got, err := parseArgs(tc.args, func(k string) string { return tc.env[k] })
		if err != nil || got != tc.want {
			t.Errorf("parseArgs(%q) = %+v, %v; want %+v", tc.args, got, err, tc.want)
		}
	}
}

// Rule (a): a reconcile flag without its partner, or reconcile with a filter, is exit 2.
func TestParseArgs_ReconcileCombinationsExitTwo(t *testing.T) {
	for _, args := range [][]string{
		{"linkedin", "--reconcile-after=5"},
		{"linkedin", "--reconcile-linkedin", "--board=x"},
		{"linkedin", "--reconcile-linkedin", "--region=IT"},
		{"linkedin", "--reconcile-linkedin", "--shard=1/2"},
		{"greenhouse", "--reconcile-linkedin"},
		{"linkedin", "--reconcile-linkedin", "--reconcile-after=-1"},
	} {
		_, err := parseArgs(args, noEnv)
		if err == nil || argsExitCode(err) != 2 {
			t.Errorf("parseArgs(%q) err=%v code=%d, want an error with exit 2", args, err, argsExitCode(err))
		}
	}
	if _, err := parseArgs([]string{"greenhouse", "--boad=x"}, noEnv); argsExitCode(err) != 1 {
		t.Errorf("unknown flag exit = %d, want 1", argsExitCode(err))
	}
}

// Rules (b) and (c): an unfinished reconciliation batch and a deadline-cut crawl are failures.
func TestExitCodes(t *testing.T) {
	clean := pipeline.Stats{Ingested: 10}
	for _, tc := range []struct {
		name        string
		total       pipeline.Stats
		truncated   bool
		sweepFailed int
		want        int
	}{
		{"clean", clean, false, 0, 0},
		{"truncated but every board clean", clean, true, 0, 1},
		{"sweep failure", clean, false, 1, 1},
		{"reached nothing", pipeline.Stats{Failed: 3}, false, 0, 1},
	} {
		if got := runExitCode(tc.total, tc.truncated, tc.sweepFailed); got != tc.want {
			t.Errorf("%s: runExitCode = %d, want %d", tc.name, got, tc.want)
		}
	}
	if got := reconcileExitCode(ownershipReport{More: true}, nil); got != 1 {
		t.Errorf("More batch exit = %d, want 1", got)
	}
	if got := reconcileExitCode(ownershipReport{}, errors.New("x")); got != 1 {
		t.Errorf("error exit = %d, want 1", got)
	}
	if got := reconcileExitCode(ownershipReport{}, nil); got != 0 {
		t.Errorf("done exit = %d, want 0", got)
	}
}
