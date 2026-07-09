// Command import-yc is a run-once host worker that enriches companies from the
// yc-oss directory (yc-oss.github.io/api/companies/all.json — the same source the
// yc collection tag uses, but here we read the full record). Each entry is mapped
// (internal/ycdir) and upserted by normalized-name slug: an existing company has
// its company-info columns plus the curated yc_batch/yc_status facets refreshed,
// and an unmatched entry is inserted as a reference row (is_reference=true) so we
// hold the full YC directory. Idempotent.
//
//	import-yc            # needs DATABASE_URL; YC_DATASET_URL overrides the source
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/worker"
	"github.com/strelov1/freehire/internal/ycdir"
)

const (
	defaultDatasetURL = "https://yc-oss.github.io/api/companies/all.json"
	fetchTimeout      = 60 * time.Second
)

// store is the slice of the data layer the loader needs; *db.Queries satisfies it.
type store interface {
	CompanyExists(ctx context.Context, slug string) (bool, error)
	UpsertYCCompany(ctx context.Context, arg db.UpsertYCCompanyParams) error
}

func main() { worker.Main(run) }

func run() int {
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	entries, err := fetch(ctx, datasetURL())
	if err != nil {
		log.Printf("import-yc: fetch: %v", err)
		return 1
	}

	stats, err := load(ctx, db.New(pool), entries)
	if err != nil {
		log.Printf("import-yc: %v", err)
		return 1
	}
	log.Printf("import-yc done: matched=%d inserted=%d skipped=%d", stats.matched, stats.inserted, stats.skipped)
	return 0
}

func datasetURL() string {
	if u := os.Getenv("YC_DATASET_URL"); u != "" {
		return u
	}
	return defaultDatasetURL
}

// fetch downloads and decodes the yc-oss directory (a JSON array of entries).
func fetch(ctx context.Context, url string) ([]ycdir.Entry, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dataset %s: status %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var entries []ycdir.Entry
	if err := json.Unmarshal(body, &entries); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	return entries, nil
}

type loadStats struct{ matched, inserted, skipped int }

// load maps each entry and upserts it, tallying matched-existing vs inserted-
// reference (via CompanyExists before the blind upsert) and skipped blank names.
func load(ctx context.Context, s store, entries []ycdir.Entry) (loadStats, error) {
	var stats loadStats
	for _, e := range entries {
		rec, ok := ycdir.Map(e)
		if !ok {
			stats.skipped++
			continue
		}
		exists, err := s.CompanyExists(ctx, rec.Slug)
		if err != nil {
			return stats, err
		}
		if err := s.UpsertYCCompany(ctx, recordToParams(rec)); err != nil {
			return stats, err
		}
		if exists {
			stats.matched++
		} else {
			stats.inserted++
		}
	}
	return stats, nil
}

// recordToParams maps a mapped directory record to upsert params: empty scalars
// become SQL NULL, arrays are non-nil (NOT NULL columns), and the extras JSONB
// falls back to "{}".
func recordToParams(r ycdir.Record) db.UpsertYCCompanyParams {
	info := []byte("{}")
	if len(r.Info) > 0 {
		if b, err := json.Marshal(r.Info); err == nil {
			info = b
		}
	}
	return db.UpsertYCCompanyParams{
		Slug:          r.Slug,
		Name:          r.Name,
		Industries:    nonNil(r.Industries),
		YearFounded:   int4(r.YearFounded),
		EmployeeCount: int4(r.EmployeeCount),
		HqCountry:     text(r.HQCountry),
		Tagline:       text(r.Tagline),
		CompanyInfo:   info,
		YcBatch:       single(r.Batch),
		YcStatus:      single(r.Status),
	}
}

// single wraps a non-empty scalar as a one-element array (empty → '{}'), so the
// curated facet columns filter through the same array-overlap machinery.
func single(s string) []string {
	if strings.TrimSpace(s) == "" {
		return []string{}
	}
	return []string{s}
}

func nonNil(a []string) []string {
	if a == nil {
		return []string{}
	}
	return a
}

func text(s string) pgtype.Text {
	if s = strings.TrimSpace(s); s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

func int4(n int) pgtype.Int4 {
	if n <= 0 {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(n), Valid: true}
}
