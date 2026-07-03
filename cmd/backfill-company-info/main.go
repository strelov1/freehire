// Command backfill-company-info is a one-time host worker that loads company-info
// records from a local JSONL dataset into the companies table. Each record is matched
// to a company by its normalized-name slug: an existing company (job-backed or a prior
// reference) has only its company-info columns refreshed, and an unmatched record is
// inserted as a reference row (is_reference = true) with no jobs, which a later job for
// the same slug adopts. The loader is source-agnostic — the dataset's origin is named
// nowhere — and idempotent: re-running the same file rewrites the same values.
//
//	backfill-company-info <path/to/records.jsonl>   # needs DATABASE_URL
//
// Company-info columns are independent of the job-derived facet columns; this worker
// never touches job_count, collections, or those facets. Unknown source values are
// stored as NULL (or omitted from the company_info JSONB) so "unknown" stays distinct
// from a real value.
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"os"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/normalize"
	"github.com/strelov1/freehire/internal/worker"
)

// record is one line of the dataset. Only these fields are consumed; unknown fields
// are ignored. Numeric fields that are absent/null decode to their zero value, which
// the mapping treats as "unknown".
type record struct {
	Name             string   `json:"name"`
	HomepageURI      string   `json:"website"`
	HQCountry        string   `json:"country"`
	ParentCompany    string   `json:"parent"`
	Subsidiaries     []string `json:"subsidiaries"`
	Industries       []string `json:"industries"`
	Activities       []string `json:"activities"`
	NbEmployees      int      `json:"employees"`
	YearFounded      int      `json:"founded"`
	Tagline          string   `json:"tagline"`
	OrganizationType string   `json:"org_type"`
	FundingInvestors []string `json:"funding_investors"`
	FundingType      string   `json:"funding_type"`
	FundingYear      int      `json:"funding_year"`
	FundingAmount    int64    `json:"funding_amount"`
	StockExchange    string   `json:"stock_exchange"`
	StockSymbol      string   `json:"stock_symbol"`
}

// store is the slice of the data layer the loader needs; *db.Queries satisfies it and
// tests use a fake.
type store interface {
	CompanyExists(ctx context.Context, slug string) (bool, error)
	UpsertCompanyInfo(ctx context.Context, arg db.UpsertCompanyInfoParams) error
}

func main() { worker.Main(run) }

func run() int {
	if len(os.Args) < 2 {
		log.Printf("usage: backfill-company-info <path/to/records.jsonl>")
		return 2
	}
	path := os.Args[1]

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	f, err := os.Open(path)
	if err != nil {
		log.Printf("open %s: %v", path, err)
		return 1
	}
	defer f.Close()

	stats, err := load(ctx, db.New(pool), f)
	if err != nil {
		log.Printf("backfill-company-info: %v", err)
		return 1
	}
	log.Printf("backfill-company-info done: applied=%d matched-existing=%d inserted-reference=%d skipped=%d",
		stats.applied, stats.matched, stats.inserted, stats.skipped)
	return 0
}

type loadStats struct{ applied, matched, inserted, skipped int }

// load streams the JSONL dataset, upserting each valid record's company-info by slug and
// tallying matched-existing vs inserted-reference for the report. A blank-name/blank-slug
// or unparseable line is skipped, not fatal.
func load(ctx context.Context, s store, r io.Reader) (loadStats, error) {
	var stats loadStats
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 8*1024*1024) // records run a few KB
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var rec record
		if err := json.Unmarshal(line, &rec); err != nil {
			stats.skipped++
			continue
		}
		params, ok := recordToParams(rec)
		if !ok {
			stats.skipped++
			continue
		}
		exists, err := s.CompanyExists(ctx, params.Slug)
		if err != nil {
			return stats, err
		}
		if err := s.UpsertCompanyInfo(ctx, params); err != nil {
			return stats, err
		}
		stats.applied++
		if exists {
			stats.matched++
		} else {
			stats.inserted++
		}
		if stats.applied%5000 == 0 {
			log.Printf("backfill-company-info: applied=%d matched=%d inserted=%d",
				stats.applied, stats.matched, stats.inserted)
		}
	}
	return stats, sc.Err()
}

// recordToParams maps a dataset record to upsert params, returning ok=false for a
// record with no usable name (empty slug). Empty/zero source values become NULL, and
// the low-coverage extras are assembled into the company_info JSONB.
func recordToParams(r record) (db.UpsertCompanyInfoParams, bool) {
	name := strings.TrimSpace(r.Name)
	slug := normalize.Slug(name)
	if slug == "" {
		return db.UpsertCompanyInfoParams{}, false
	}
	industries := r.Industries
	if industries == nil {
		industries = []string{} // NOT NULL column: send '{}', not NULL
	}
	return db.UpsertCompanyInfoParams{
		Slug:             slug,
		Name:             name,
		Industries:       industries,
		YearFounded:      int4(r.YearFounded),
		EmployeeCount:    int4(r.NbEmployees),
		HqCountry:        text(r.HQCountry),
		OrganizationType: text(r.OrganizationType),
		Tagline:          text(r.Tagline),
		CompanyInfo:      companyInfoJSON(r),
	}, true
}

// companyInfoJSON assembles the low-coverage extras into the company_info JSONB,
// omitting empty values so absent facts stay absent rather than empty sentinels. An
// all-empty record yields "{}".
func companyInfoJSON(r record) json.RawMessage {
	m := map[string]any{}
	if v := strings.TrimSpace(r.HomepageURI); v != "" {
		m["homepage"] = v
	}
	if v := strings.TrimSpace(r.ParentCompany); v != "" {
		m["parent_company"] = v
	}
	if len(r.Subsidiaries) > 0 {
		m["subsidiaries"] = r.Subsidiaries
	}
	if len(r.Activities) > 0 {
		m["activities"] = r.Activities
	}
	funding := map[string]any{}
	if v := strings.TrimSpace(r.FundingType); v != "" {
		funding["type"] = v
	}
	if r.FundingAmount > 0 {
		funding["amount"] = r.FundingAmount
	}
	if r.FundingYear > 0 {
		funding["year"] = r.FundingYear
	}
	if len(r.FundingInvestors) > 0 {
		funding["investors"] = r.FundingInvestors
	}
	if len(funding) > 0 {
		m["funding"] = funding
	}
	if v := strings.TrimSpace(r.StockSymbol); v != "" {
		stock := map[string]any{"symbol": v}
		if e := strings.TrimSpace(r.StockExchange); e != "" {
			stock["exchange"] = e
		}
		m["stock"] = stock
	}
	if len(m) == 0 {
		return json.RawMessage("{}")
	}
	b, err := json.Marshal(m)
	if err != nil {
		return json.RawMessage("{}")
	}
	return b
}

// text maps an optional string to a nullable pgtype.Text; blank becomes SQL NULL.
func text(s string) pgtype.Text {
	s = strings.TrimSpace(s)
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

// int4 maps an optional positive int to a nullable pgtype.Int4; a non-positive value
// (absent headcount / founding year) becomes SQL NULL.
func int4(n int) pgtype.Int4 {
	if n <= 0 {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(n), Valid: true}
}
