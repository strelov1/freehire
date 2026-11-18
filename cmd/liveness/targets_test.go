package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/strelov1/freehire/internal/platform/db"
)

type targetQueries struct {
	jobs       []db.Job
	loaded     []int64
	broadReads int
}

func (q *targetQueries) GetJobsByIDs(_ context.Context, ids []int64) ([]db.Job, error) {
	q.loaded = ids
	return q.jobs, nil
}
func (q *targetQueries) SelectOrphanLivenessCandidates(context.Context, []string) ([]db.SelectOrphanLivenessCandidatesRow, error) {
	q.broadReads++
	return nil, nil
}
func (q *targetQueries) SelectStaleRegisteredCandidates(context.Context, db.SelectStaleRegisteredCandidatesParams) ([]db.SelectStaleRegisteredCandidatesRow, error) {
	q.broadReads++
	return nil, nil
}

func TestParseProbeJobIDs(t *testing.T) {
	for _, raw := range []string{" ", ",", "1,", "0", "-1", "abc", "9223372036854775808"} {
		if _, err := parseProbeJobIDs(raw); err == nil {
			t.Errorf("parseProbeJobIDs(%q) accepted an invalid scope", raw)
		}
	}
	ids, err := parseProbeJobIDs("2, 1,2")
	if err != nil || !reflect.DeepEqual(ids, []int64{2, 1}) {
		t.Fatalf("ids=%v err=%v", ids, err)
	}
	ids, err = parseProbeJobIDs("")
	if err != nil || len(ids) != 0 {
		t.Fatalf("default ids=%v err=%v", ids, err)
	}
}

func TestTargetedProbeSelectionPreservesEligibilityAndAvoidsCatalogueReads(t *testing.T) {
	cutoff := time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
	old := pgtype.Timestamptz{Time: cutoff.Add(-time.Hour), Valid: true}
	fresh := pgtype.Timestamptz{Time: cutoff.Add(time.Hour), Valid: true}
	job := func(id int64, source string) db.Job {
		return db.Job{ID: id, Source: source, URL: "https://example.com/job", PublicSlug: "job", LastSeenAt: old}
	}
	q := &targetQueries{jobs: []db.Job{job(1, "manual"), job(2, "instaffo"), job(3, "instaffo"), job(4, "instaffo"), job(5, "instaffo"), job(6, "greenhouse"), job(7, "telegram"), job(8, "manual"), job(9, "instaffo"), job(10, "instaffo"), job(11, "instaffo")}}
	q.jobs[2].LastSeenAt = fresh
	q.jobs[3].ClosedAt = old
	q.jobs[4].IsPrivate = true
	q.jobs[7].URL = ""
	q.jobs[9].LastSeenAt = pgtype.Timestamptz{}
	q.jobs[10].LastSeenAt = pgtype.Timestamptz{Time: cutoff, Valid: true}
	ids := []int64{1, 2, 3, 4, 5, 6, 7, 8, 10, 11, 99}
	orphans, stale, err := selectProbeCandidates(context.Background(), q, probeScope{jobIDs: ids}, []string{"instaffo", "greenhouse", "telegram"}, []string{"instaffo"}, cutoff)
	if err != nil {
		t.Fatal(err)
	}
	if q.broadReads != 0 || !reflect.DeepEqual(q.loaded, ids) {
		t.Fatalf("selection read outside the requested scope: broad=%d ids=%v", q.broadReads, q.loaded)
	}
	if len(orphans) != 1 || orphans[0].ID != 1 || len(stale) != 1 || stale[0].ID != 2 {
		t.Fatalf("orphans=%v stale=%v, want only manual1 and stale public instaffo2", orphans, stale)
	}
	orphans, stale, err = selectProbeCandidates(context.Background(), q, probeScope{source: "instaffo", jobIDs: ids}, []string{"instaffo", "greenhouse", "telegram"}, []string{"instaffo"}, cutoff)
	if err != nil || len(orphans) != 0 || len(stale) != 1 || stale[0].ID != 2 {
		t.Fatalf("source filter widened scope: orphans=%v stale=%v err=%v", orphans, stale, err)
	}
}

func TestExplicitEmptyProbeScopeFailsBeforeDatabaseBootstrap(t *testing.T) {
	oldFlags, oldArgs := flag.CommandLine, os.Args
	t.Cleanup(func() { flag.CommandLine, os.Args = oldFlags, oldArgs })
	flag.CommandLine = flag.NewFlagSet("liveness", flag.ContinueOnError)
	os.Args = []string{"liveness", "-job-ids="}
	t.Setenv("DATABASE_URL", "invalid")
	if code := run(); code != 1 {
		t.Fatalf("exit=%d, want invalid scope refusal", code)
	}
}

func TestProbeScopeRejectsMoreThan500DistinctIDs(t *testing.T) {
	var values []string
	for i := 1; i <= 501; i++ {
		values = append(values, fmt.Sprint(i))
	}
	if _, err := parseProbeJobIDs(strings.Join(values, ",")); err == nil {
		t.Fatal("accepted an oversized scope")
	}
}
