// Command quality-audit measures, by sampling, how accurate what the catalogue shows is — the
// layer above source-quality-metrics, which counts what is MISSING; this one checks what is
// WRONG. Run it weekly. It is a run-once-and-exit worker with two passes:
//
//   - liveness: for every source with at least QUALITY_AUDIT_MIN_OPEN open public postings, probe
//     a random sample of their stored URLs and count how many the source answers gone (404/410)
//     and how many live — counting a verdict only on a host that has first 404'd a posting that
//     cannot exist (see internal/job/qualityaudit).
//   - remote: ask a model, for ~QUALITY_AUDIT_REMOTE_MAX postings tagged remote, what each
//     posting's own text says about where the work is done, and keep the judgement.
//
// It MEASURES and never acts: no posting is closed, reopened or retagged. Results go out as
// gauges through the node_exporter textfile collector, raw counts only — the alert rules own
// every threshold. Without PROM_TEXTFILE_DIR it is a no-op that never opens the pool, unless
// -dry-run, which runs both passes, writes nothing and prints the exposition.
//
// It exits non-zero when a judgement could not be made or stored; an unanswered probe is the
// measurement, not a failure.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"math/rand/v2"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/job/qualityaudit"
	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/llm"
	"github.com/strelov1/freehire/internal/platform/safehttp"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// textfileName is the collector file this worker owns — deliberately NOT <binary>.prom, which
// worker.Main overwrites with the run outcome after run returns (the same reason queue-metrics
// publishes as freehire-pipeline.prom).
const textfileName = "freehire-quality-audit.prom"

const (
	// remoteWindow is how far back the remote gauges reach. One week judges ~10 sources ~20
	// postings each; eight weeks lets a large source, drawn most weeks, accumulate a sample that
	// is a precision rather than an anecdote.
	remoteWindow = 56 * 24 * time.Hour
	// minRemote is the fewest open remote postings a source needs to be drawn at all: below it
	// a sample of 20 is most of the source, and the source is not where visitors meet remote work.
	minRemote = 50
	// oversample sizes the Bernoulli pre-filter of a sample query at a few times the sample, so
	// it almost never comes back short and still sorts only a few dozen rows.
	oversample = 4
	// probeTimeout bounds one request, redirects included.
	probeTimeout = 15 * time.Second
	// hostInterval spaces requests to one host; a longer Crawl-delay wins.
	hostInterval = 2 * time.Second
	// maxCrawlDelay is the longest Crawl-delay waited out; a host asking for more is skipped.
	maxCrawlDelay = 60 * time.Second
	// hostWorkers is how many hosts are probed at once.
	hostWorkers = 16
	// judgeFeature files the judge's spend apart in the gateway.
	judgeFeature = "quality-audit"
)

type knobs struct {
	minOpen, maxRequests int64
	sample, remoteMax    int32
	remotePerSource      int32
}

func loadKnobs() (knobs, error) {
	var k knobs
	var err error
	if k.minOpen, err = worker.EnvInt64("QUALITY_AUDIT_MIN_OPEN", 500); err != nil {
		return k, err
	}
	if k.maxRequests, err = worker.EnvInt64("QUALITY_AUDIT_MAX_REQUESTS", 15000); err != nil {
		return k, err
	}
	if k.sample, err = worker.EnvInt32("QUALITY_AUDIT_LIVENESS_SAMPLE", 30); err != nil {
		return k, err
	}
	if k.remoteMax, err = worker.EnvInt32("QUALITY_AUDIT_REMOTE_MAX", 200); err != nil {
		return k, err
	}
	if k.remotePerSource, err = worker.EnvInt32("QUALITY_AUDIT_REMOTE_PER_SOURCE", 20); err != nil {
		return k, err
	}
	return k, nil
}

func main() { worker.Main(run) }

func run() int {
	dryRun := flag.Bool("dry-run", false, "run both passes, store nothing, print the exposition")
	flag.Parse()

	dir := os.Getenv(worker.PromTextfileDirEnv)
	if dir == "" && !*dryRun {
		log.Printf("quality-audit: %s is unset, nothing to publish", worker.PromTextfileDirEnv)
		return 0
	}
	k, err := loadKnobs()
	if err != nil {
		log.Printf("config: %v", err)
		return 1
	}

	// The judge runs on the SERVICE credential: this is ownerless background work, so no user's
	// key is resolved and the paid-AI gate does not apply. It is tagged so its spend reads apart.
	var model qualityaudit.JSONModel
	if lcfg := config.LoadLLM(); lcfg.Require() == nil {
		client, flush, err := llm.NewClient(lcfg.Settings(lcfg.Model), judgeFeature)
		if err != nil {
			log.Printf("llm: %v", err)
			return 1
		}
		defer flush()
		model = client.As("", nil, llm.Feature(judgeFeature))
	} else {
		// The remote gauges keep publishing the stored window; their timestamp going stale is
		// what an alert sees.
		log.Print("quality-audit: LLM_* not configured, the remote check is skipped this run")
	}

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()
	q := db.New(pool)

	snap := snapshot{livenessAt: time.Now()}
	var livenessErr error
	failed := 0
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		snap.liveness, livenessErr = auditLiveness(ctx, q, k)
	}()
	if model != nil {
		failed = auditRemote(ctx, q, model, k, *dryRun)
	}
	wg.Wait()
	if livenessErr != nil {
		log.Printf("quality-audit: liveness: %v", livenessErr)
		return 1
	}

	tally, err := q.TallyRemoteAuditJudgements(ctx, pgtype.Timestamptz{Time: time.Now().Add(-remoteWindow), Valid: true})
	if err != nil {
		log.Printf("quality-audit: tally: %v", err)
		return 1
	}
	snap.remote = tally
	latest, err := q.LatestRemoteAuditJudgement(ctx)
	if err != nil {
		log.Printf("quality-audit: latest judgement: %v", err)
		return 1
	}
	if latest.Valid {
		snap.remoteAt = latest.Time
	}

	body := render(snap)
	if *dryRun {
		fmt.Print(body)
	} else if err := worker.WriteTextfile(dir, textfileName, body); err != nil {
		log.Printf("quality-audit: %v", err)
		return 1
	}
	return worker.ExitCode(failed, 0)
}

// auditLiveness samples every eligible source, probes the sample and tallies it. Every eligible
// source is in the tally, sampled or not, so a source whose sample came back empty still says so.
func auditLiveness(ctx context.Context, q *db.Queries, k knobs) (map[string]qualityaudit.LivenessCounts, error) {
	sources, err := q.ListLivenessAuditSources(ctx, k.minOpen)
	if err != nil {
		return nil, fmt.Errorf("list sources: %w", err)
	}
	var postings []qualityaudit.Posting
	for _, s := range sources {
		rows, err := q.SampleLivenessAuditJobs(ctx, db.SampleLivenessAuditJobsParams{
			Source:   s.Source,
			Fraction: fraction(int64(k.sample), s.OpenJobs),
			MaxRows:  k.sample,
		})
		if err != nil {
			return nil, fmt.Errorf("sample %s: %w", s.Source, err)
		}
		for _, r := range rows {
			postings = append(postings, qualityaudit.Posting{
				ID: r.ID, Source: r.Source, ExternalID: r.ExternalID, URL: r.URL, Slug: r.PublicSlug,
			})
		}
	}
	log.Printf("quality-audit: liveness: probing %d postings of %d sources", len(postings), len(sources))

	auditor := &qualityaudit.Auditor{
		Client:        safehttp.NewClient(probeTimeout),
		MaxRequests:   k.maxRequests,
		HostInterval:  hostInterval,
		MaxCrawlDelay: maxCrawlDelay,
		HostWorkers:   hostWorkers,
	}
	results := auditor.Run(ctx, postings)
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}

	reasons := map[qualityaudit.Reason]int{}
	for _, r := range results {
		switch r.Outcome {
		case qualityaudit.Dead:
			// The drill-down: every posting shown while its source says it is gone.
			log.Printf("quality-audit: dead %s %s %s", r.Posting.Source, r.Posting.Slug, r.Posting.URL)
		case qualityaudit.Unmeasured:
			reasons[r.Reason]++
		}
	}
	tally := qualityaudit.TallyLiveness(results)
	for _, s := range sources {
		if _, ok := tally[s.Source]; !ok {
			tally[s.Source] = qualityaudit.LivenessCounts{}
		}
	}
	var live, dead int64
	for _, c := range tally {
		live += c.Live
		dead += c.Dead
	}
	log.Printf("quality-audit: liveness done: sampled=%d live=%d dead=%d unmeasured=%s",
		len(results), live, dead, formatReasons(reasons))
	return tally, nil
}

// auditRemote judges the week's sample and stores each judgement, returning how many postings
// could not be judged or stored.
func auditRemote(ctx context.Context, q *db.Queries, model qualityaudit.JSONModel, k knobs, dryRun bool) int {
	counts, err := q.CountOpenRemoteJobsBySource(ctx, minRemote)
	if err != nil {
		log.Printf("quality-audit: remote: count: %v", err)
		return 1
	}
	weights := make([]qualityaudit.SourceWeight, 0, len(counts))
	for _, c := range counts {
		weights = append(weights, qualityaudit.SourceWeight{Source: c.Source, Weight: c.RemoteJobs})
	}
	picks := int(max(k.remoteMax/k.remotePerSource, 1))
	chosen := qualityaudit.ChooseSources(weights, picks, rand.New(rand.NewPCG(rand.Uint64(), rand.Uint64())))
	log.Printf("quality-audit: remote: judging up to %d postings each of %d sources", k.remotePerSource, len(chosen))

	failed, judged := 0, 0
	for _, pick := range chosen {
		src := pick.Source
		rows, err := q.SampleRemoteAuditJobs(ctx, db.SampleRemoteAuditJobsParams{
			Source:   src,
			Fraction: fraction(int64(k.remotePerSource), pick.Weight),
			MaxRows:  k.remotePerSource,
		})
		if err != nil {
			log.Printf("quality-audit: remote: sample %s: %v", src, err)
			failed++
			continue
		}
		verdicts := map[string]int{}
		for _, r := range rows {
			if ctx.Err() != nil || judged >= int(k.remoteMax) {
				break
			}
			judged++
			j, err := qualityaudit.JudgeRemote(ctx, model, qualityaudit.RemotePosting{
				ID: r.ID, Title: r.Title, Location: r.Location, Description: r.Description,
			})
			if err != nil {
				log.Printf("quality-audit: remote: %s %s: %v", src, r.PublicSlug, err)
				failed++
				continue
			}
			verdicts[j.WorkMode]++
			if j.WorkMode == qualityaudit.ModeHybrid || j.WorkMode == qualityaudit.ModeOnsite ||
				(r.TaggedGlobal && j.Worldwide == qualityaudit.WorldwideNo) {
				log.Printf("quality-audit: remote: %s %s reads %s, worldwide %s", src, r.PublicSlug, j.WorkMode, j.Worldwide)
			}
			if dryRun {
				continue
			}
			if err := q.InsertRemoteAuditJudgement(ctx, db.InsertRemoteAuditJudgementParams{
				JobID: r.ID, Source: src, WorkMode: j.WorkMode, Worldwide: j.Worldwide, TaggedGlobal: r.TaggedGlobal,
			}); err != nil {
				log.Printf("quality-audit: remote: store %s: %v", r.PublicSlug, err)
				failed++
			}
		}
		log.Printf("quality-audit: remote: %s %v", src, verdicts)
	}
	return failed
}

// fraction sizes a sample query's Bernoulli pre-filter: oversample times the sample's share of
// the population, capped at 1.
func fraction(sample, population int64) float64 {
	if population <= 0 {
		return 1
	}
	return min(1, float64(oversample*sample)/float64(population))
}

func formatReasons(m map[qualityaudit.Reason]int) string {
	parts := make([]string, 0, len(m))
	for r, n := range m {
		parts = append(parts, fmt.Sprintf("%s:%d", r, n))
	}
	sort.Strings(parts)
	return "{" + strings.Join(parts, " ") + "}"
}
