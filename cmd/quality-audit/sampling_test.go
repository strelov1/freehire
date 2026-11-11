package main

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/llm"
)

func TestRemoteStrataReserveGlobalCoverage(t *testing.T) {
	strata := remoteStrata([]db.CountOpenRemoteJobsBySourceRow{{Source: "rare-global", RemoteJobs: 10000, GlobalJobs: 1}, {Source: "large", RemoteJobs: 100000, GlobalJobs: 0}}, 200)
	if len(strata) != 2 || !strata[0].global || strata[0].budget != 100 || strata[1].budget != 100 {
		t.Fatalf("strata = %+v", strata)
	}
	if len(strata[0].weights) != 1 || strata[0].weights[0].Source != "rare-global" || strata[0].weights[0].Weight != 1 {
		t.Fatalf("global weights = %+v", strata[0].weights)
	}
}

func TestRemoteStrataEmptyGlobalReusesBudget(t *testing.T) {
	strata := remoteStrata([]db.CountOpenRemoteJobsBySourceRow{{Source: "remote", RemoteJobs: 1000}}, 200)
	if len(strata) != 1 || strata[0].global || strata[0].budget != 200 {
		t.Fatalf("strata = %+v", strata)
	}
}

func TestAuditRemoteKeepsEligibleNonGlobalRows(t *testing.T) {
	q := newSampleCatalogue([]db.CountOpenRemoteJobsBySourceRow{{Source: "mixed", RemoteJobs: 50, GlobalJobs: 1}})
	model := &countingJudge{}
	if failed := auditRemote(context.Background(), q, model, knobs{remoteMax: 200, remotePerSource: 20}, false); failed != 0 {
		t.Fatalf("failures=%d", failed)
	}
	if model.calls != 20 || q.globalJudged != 1 || len(q.stored) != 20 {
		t.Fatalf("calls=%d global=%d stored=%d, want20/1/20", model.calls, q.globalJudged, len(q.stored))
	}
}

func TestAuditRemoteSharesPerSourceCapAcrossStrata(t *testing.T) {
	q := newSampleCatalogue([]db.CountOpenRemoteJobsBySourceRow{{Source: "mixed", RemoteJobs: 1000, GlobalJobs: 500}})
	model := &countingJudge{}
	auditRemote(context.Background(), q, model, knobs{remoteMax: 200, remotePerSource: 20}, false)
	if model.calls != 20 {
		t.Fatalf("calls=%d, per-source limit20", model.calls)
	}
}

func TestAuditRemoteFillsGlobalBudgetFromSmallSources(t *testing.T) {
	var counts []db.CountOpenRemoteJobsBySourceRow
	for i := range 100 {
		counts = append(counts, db.CountOpenRemoteJobsBySourceRow{Source: fmt.Sprintf("source-%d", i), RemoteJobs: 51, GlobalJobs: 1})
	}
	q := newSampleCatalogue(counts)
	model := &countingJudge{}
	auditRemote(context.Background(), q, model, knobs{remoteMax: 200, remotePerSource: 20}, false)
	if model.calls != 200 || q.globalJudged != 100 {
		t.Fatalf("calls=%d global=%d, want200/100", model.calls, q.globalJudged)
	}
	bySource := map[string]int{}
	for _, row := range q.stored {
		bySource[row.Source]++
		if bySource[row.Source] > 20 {
			t.Fatalf("source %s exceeded cap", row.Source)
		}
	}
}

func TestAuditRemoteFailedCallsConsumeSourceBudget(t *testing.T) {
	q := newSampleCatalogue([]db.CountOpenRemoteJobsBySourceRow{{Source: "mixed", RemoteJobs: 1000, GlobalJobs: 500}})
	model := &countingJudge{err: errors.New("gateway failed")}
	failed := auditRemote(context.Background(), q, model, knobs{remoteMax: 200, remotePerSource: 20}, false)
	if model.calls != 20 || failed != 20 {
		t.Fatalf("calls=%d failures=%d, want20/20", model.calls, failed)
	}
}

type sampleCatalogue struct {
	counts       []db.CountOpenRemoteJobsBySourceRow
	rows         map[string][]db.SampleRemoteAuditJobsRow
	stored       map[int64]db.InsertRemoteAuditJudgementParams
	globalJudged int
}

func newSampleCatalogue(counts []db.CountOpenRemoteJobsBySourceRow) *sampleCatalogue {
	q := &sampleCatalogue{counts: counts, rows: map[string][]db.SampleRemoteAuditJobsRow{}, stored: map[int64]db.InsertRemoteAuditJudgementParams{}}
	var id int64
	for _, c := range counts {
		for i := int64(0); i < c.RemoteJobs; i++ {
			id++
			row := db.SampleRemoteAuditJobsRow{ID: id, Source: c.Source, TaggedGlobal: i < c.GlobalJobs}
			key := fmt.Sprintf("%s:%t", c.Source, row.TaggedGlobal)
			q.rows[key] = append(q.rows[key], row)
		}
	}
	return q
}
func (q *sampleCatalogue) CountOpenRemoteJobsBySource(context.Context, int64) ([]db.CountOpenRemoteJobsBySourceRow, error) {
	return q.counts, nil
}
func (q *sampleCatalogue) SampleRemoteAuditJobs(_ context.Context, p db.SampleRemoteAuditJobsParams) ([]db.SampleRemoteAuditJobsRow, error) {
	rows := q.rows[fmt.Sprintf("%s:%t", p.Source, p.OnlyGlobal)]
	return rows[:min(int(p.MaxRows), len(rows))], nil
}
func (q *sampleCatalogue) InsertRemoteAuditJudgement(_ context.Context, p db.InsertRemoteAuditJudgementParams) error {
	if _, ok := q.stored[p.JobID]; ok {
		return errors.New("duplicate judgement")
	}
	q.stored[p.JobID] = p
	if p.TaggedGlobal {
		q.globalJudged++
	}
	return nil
}

type countingJudge struct {
	calls int
	err   error
}

func (m *countingJudge) GenerateJSON(context.Context, string, string, ...llm.GenOption) (string, error) {
	m.calls++
	return `{"work_mode":"unclear","worldwide":"unclear"}`, m.err
}

func TestAuditRemoteTransfersUnusedGlobalBudget(t *testing.T) {
	counts := []db.CountOpenRemoteJobsBySourceRow{{Source: "rare-global", RemoteJobs: 1, GlobalJobs: 1}}
	for i := range 10 {
		counts = append(counts, db.CountOpenRemoteJobsBySourceRow{Source: fmt.Sprintf("remote-%d", i), RemoteJobs: 50})
	}
	q := newSampleCatalogue(counts)
	model := &countingJudge{}
	failed := auditRemote(context.Background(), q, model, knobs{remoteMax: 200, remotePerSource: 20}, false)
	if failed != 0 || model.calls != 200 || q.globalJudged != 1 || len(q.stored) != 200 {
		t.Fatalf("failures=%d calls=%d global=%d stored=%d, want0/200/1/200", failed, model.calls, q.globalJudged, len(q.stored))
	}
	bySource := map[string]int{}
	for _, row := range q.stored {
		bySource[row.Source]++
		if bySource[row.Source] > 20 {
			t.Fatalf("source %s exceeded cap", row.Source)
		}
	}
}
