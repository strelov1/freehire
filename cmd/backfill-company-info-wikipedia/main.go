// Command backfill-company-info-wikipedia fills companies.tagline/company_info for
// companies no curated company-info source has ever matched — the population that
// keeps growing as ATS crawling discovers new companies (see
// openspec/changes/company-info-wikipedia-backfill). Each eligible company (blank
// tagline, never checked by this backfill) is looked up against Wikidata; a match is
// accepted only when the resolved entity is confidently typed as a business/
// organization (internal/job/wikicompany), never by keyword-scanning its description.
//
// Reports by default; --apply writes. WIKIPEDIA_BACKFILL_MAX_PER_RUN (default 500)
// bounds one run. A company is marked checked whether or not a confident match was
// found, so a later run never re-queries Wikidata for it — see migration 0154.
//
// Needs DATABASE_URL. Outbound HTTPS only, to www.wikidata.org, query.wikidata.org,
// and en.wikipedia.org — no API key, no billing.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/job/wikicompany"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/safehttp"
	"github.com/strelov1/freehire/internal/platform/worker"
)

const defaultMaxPerRun = 500

func main() { worker.Main(run) }

func run() int {
	apply := flag.Bool("apply", false, "actually write matches; without it the run only reports what it would do")
	flag.Parse()

	maxPerRun, err := worker.EnvInt64("WIKIPEDIA_BACKFILL_MAX_PER_RUN", defaultMaxPerRun)
	if err != nil {
		log.Printf("backfill-company-info-wikipedia: %v", err)
		return 1
	}

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	q := db.New(pool)
	client := wikicompany.New(safehttp.NewClient(15 * time.Second))

	onFail := func(slug string, err error) {
		log.Printf("backfill-company-info-wikipedia: %s: %v", slug, err)
	}

	res, err := runBackfill(ctx, dbStore{q: q}, client, *apply, maxPerRun, onFail)
	if err != nil {
		log.Printf("backfill-company-info-wikipedia: %v", err)
		return 1
	}

	// A per-company failure (already logged via onFail) is counted and stepped
	// over, never fatal to the run — the same convention backfill-talent-handle
	// and discord-sync use, so one company whose Wikidata lookup deterministically
	// fails does not turn every future scheduled run red.
	if !*apply {
		log.Printf("backfill-company-info-wikipedia: dry run — would match %d, reject %d, %d failed (of up to %d). Re-run with --apply to write.",
			res.Matched, res.Rejected, res.Failed, maxPerRun)
		return 0
	}
	log.Printf("backfill-company-info-wikipedia: matched %d, rejected %d, %d failed (of up to %d)",
		res.Matched, res.Rejected, res.Failed, maxPerRun)
	return 0
}

// dbStore adapts *db.Queries to the store interface the run loop depends on.
type dbStore struct{ q *db.Queries }

func (s dbStore) ListMissing(ctx context.Context, afterSlug string, limit int32) ([]candidate, error) {
	rows, err := s.q.ListCompaniesMissingWikipediaInfo(ctx, db.ListCompaniesMissingWikipediaInfoParams{
		AfterSlug: afterSlug,
		RowLimit:  limit,
	})
	if err != nil {
		return nil, err
	}
	candidates := make([]candidate, len(rows))
	for i, r := range rows {
		candidates[i] = candidate{Slug: r.Slug, Name: r.Name}
	}
	return candidates, nil
}

func (s dbStore) Fill(ctx context.Context, slug, tagline string, companyInfo json.RawMessage) error {
	return s.q.FillCompanyInfoFromWikipedia(ctx, db.FillCompanyInfoFromWikipediaParams{
		Slug:        slug,
		Tagline:     pgtype.Text{String: tagline, Valid: tagline != ""},
		CompanyInfo: companyInfo,
	})
}

func (s dbStore) MarkChecked(ctx context.Context, slug string) error {
	return s.q.MarkCompanyWikipediaChecked(ctx, slug)
}
