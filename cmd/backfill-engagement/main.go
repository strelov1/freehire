// Command backfill-engagement fills jobs.engagement (migration 0231) for the postings
// stored before internal/dict/engagement read them, and queues every row it changes for
// the search drain.
//
// It does NOT walk the catalogue. A term the dictionary reads is a word the posting
// contains, and Meilisearch already indexes the text, so the candidates are the search
// hits for each term's own phrase (~200k across all of them, measured 2026-11-03,
// against a table of ~12M rows) and only their bodies are read, by primary key. Over-
// fetching is free — the dictionary decides, and a candidate it reads nothing from keeps
// what it has — so the phrases are the bare terms, not a second copy of the rules.
//
// Nor does it need a reindex. engagement is not in content_hash, so an incremental push
// would never carry it; SetJobEngagement queues each changed open row in search_outbox in
// the same statement, and the ordinary drain pushes it. That is what makes extending the
// dictionary cheap to roll out: run this again after the deploy that extends it.
//
// The stored terms are kept and unioned with the new reading (jobderive.DeriveEngagement),
// because some came from a source's picklist the text cannot recover. Idempotent: the
// write is IS DISTINCT FROM-guarded, so a re-run writes nothing and stopping it is free.
// There is no per-run cap: a cap over a search-ranked candidate list would hand every run
// the same first ids, and the whole list is a few minutes of primary-key reads.
//
// Needs DATABASE_URL, MEILI_URL, MEILI_MASTER_KEY.
package main

import (
	"context"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/job/jobderive"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
	"github.com/strelov1/freehire/internal/search/search"
)

// candidatePhrases are the terms as Meilisearch should find them: each sent as one
// phrase, so "W-2" is not every posting with a "2" in it. A term whose dictionary rule
// reads a longer anchor is still gathered by the bare term.
var candidatePhrases = []string{
	"IR35", "IR 35",
	"W2", "W-2", "1099", "C2C", "corp to corp", "corp2corp",
	"contract to hire", "right to hire", "C2H", "temp to hire", "temp to perm", "contract to perm",
	"temp to permanent", "contract to permanent",
	"B2B", "UoP",
	"umowa o pracę", "umowy o pracę", "umowę o pracę",
	"umowa zlecenie", "umowy zlecenia", "umowa zlecenia", "umowę zlecenia", "umowę zlecenie",
	"umowa o dzieło", "umowy o dzieło", "umowę o dzieło",
	"CLT", "PJ", "pessoa jurídica",
	"ТК", "трудовой договор", "трудовому договору", "ГПХ",
	"гражданско-правовой договор", "гражданско-правовому договору",
	"самозанятый", "самозанятость", "самозанятости", "самозанятым", "ИП",
	"freiberuflich", "freiberuflicher", "freiberufliche", "Freiberufler", "Arbeitnehmerüberlassung",
	"ZZP", "zzp'er",
	"auto-entrepreneur", "autoentrepreneur", "micro-entrepreneur", "portage salarial",
	"autónomo", "autónomos", "autónoma", "monotributo", "monotributista",
}

const (
	pageSize            = 1000
	readBatch           = 500
	pauseBetweenBatches = 100 * time.Millisecond
)

func main() { worker.Main(run) }

func run() int {
	ctx, cfg, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	ids, err := candidateIDs(ctx, search.NewClient(cfg.MeiliURL, cfg.MeiliKey))
	if err != nil {
		log.Printf("backfill-engagement: gather candidates: %v", err)
		return 1
	}
	if len(ids) == 0 {
		log.Print("backfill-engagement: no candidates")
		return 0
	}
	log.Printf("backfill-engagement: %d candidates", len(ids))

	q := db.New(pool)
	var read, written, queued int64
	lastLog := time.Now()
	for start := 0; start < len(ids); start += readBatch {
		end := min(start+readBatch, len(ids))
		rows, err := q.JobsForEngagementByIDs(ctx, ids[start:end])
		if err != nil {
			log.Printf("backfill-engagement: read %d..%d after %d written: %v", start, end, written, err)
			return 1
		}
		for _, r := range rows {
			read++
			terms := jobderive.DeriveEngagement(r.Engagement, r.Title, r.Description)
			if len(terms) == 0 {
				continue
			}
			res, err := q.SetJobEngagement(ctx, db.SetJobEngagementParams{JobID: r.ID, Engagement: terms})
			if err != nil {
				log.Printf("backfill-engagement: write id=%d after %d written: %v", r.ID, written, err)
				return 1
			}
			written += res.UpdatedCount
			queued += res.EnqueuedCount
		}
		if time.Since(lastLog) >= time.Minute {
			log.Printf("backfill-engagement: progress read=%d written=%d queued=%d of %d", read, written, queued, len(ids))
			lastLog = time.Now()
		}
		select {
		case <-ctx.Done():
			log.Printf("backfill-engagement: cancelled after %d written, resume by re-running", written)
			return 1
		case <-time.After(pauseBetweenBatches):
		}
	}
	log.Printf("backfill-engagement: done, read=%d written=%d queued=%d", read, written, queued)
	return 0
}

// candidateIDs gathers the distinct ids every phrase finds, in the order first found.
func candidateIDs(ctx context.Context, sc *search.Client) ([]int64, error) {
	seen := make(map[int64]struct{})
	var ids []int64
	for _, phrase := range candidatePhrases {
		for offset := 0; ; offset += pageSize {
			res, err := sc.Search(ctx, search.SearchParams{PhraseQuery: true, Query: phrase, Limit: pageSize, Offset: offset})
			if err != nil {
				return nil, err
			}
			for _, hit := range res.Hits {
				if _, dup := seen[hit.ID]; dup {
					continue
				}
				seen[hit.ID] = struct{}{}
				ids = append(ids, hit.ID)
			}
			if len(res.Hits) < pageSize {
				break
			}
		}
		log.Printf("backfill-engagement: after %q, %d distinct candidates", phrase, len(ids))
	}
	return ids, nil
}
