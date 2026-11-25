// Command report-suspicious-boards is a hand-run, read-only report of first-party boards whose
// postings probably belong to a different employer than the company they land on: the name
// collisions the slug rule cannot see (docs/agents/company-identity.md). It writes a CSV to
// stdout and nothing to the database. A person reads it and repairs a row with
// `cmd/add-board --rename` or a moderator's `same` merge.
//
// It scans every open job once, so point DATABASE_URL at the replica.
//
// Heuristics, each evidence and never a decision:
//   - website_mismatch: the company states a website and none of its boards' ids or names
//     resemble it (motive at gomotive.com fed by greenhouse/motive and polymer/motive).
//   - name_not_board_id: a board's company name does not match its board id
//     (greenhouse/xebialatam named "LATAM Airlines").
//   - same_provider_dissimilar: a slug fed by several boards on one provider, one of which
//     resembles neither its siblings nor the company.
//   - token_names_other_company: a board id is another company's slug or website label
//     (ashby/exa feeding exa-labs while exa exists).
//
// Boards sharing one name on one provider and slug (a chain: Chick-fil-A's workstream boards)
// skip the per-board name checks, and opaque ids (uuids, numeric keys) are never compared.
package main

import (
	"context"
	"encoding/csv"
	"io"
	"log"
	"os"
	"strconv"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

func main() { worker.Main(run) }

func run() int {
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	q := db.New(pool)
	rows, err := q.ListOpenJobsByCompanyBoard(ctx, sources.TaxonomyAggregatorProviders())
	if err != nil {
		log.Printf("report-suspicious-boards: boards: %v", err)
		return 1
	}
	sites, err := q.ListHiringCompanySites(ctx)
	if err != nil {
		log.Printf("report-suspicious-boards: companies: %v", err)
		return 1
	}
	companies := make([]company, len(sites))
	for i, s := range sites {
		companies[i] = company{Slug: s.Slug, Name: s.Name, Website: s.Website}
	}
	found := suspicious(rows, companies)
	if err := writeCSV(os.Stdout, found); err != nil {
		log.Printf("report-suspicious-boards: write: %v", err)
		return 1
	}
	log.Printf("report-suspicious-boards: %d findings over %d company boards", len(found), len(rows))
	return 0
}

func writeCSV(w io.Writer, found []finding) error {
	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"company_slug", "company_name", "company_website", "provider", "board_token",
		"board_company_name", "open_jobs", "reason", "suggested_action", "confidence"})
	for _, f := range found {
		_ = cw.Write([]string{f.CompanySlug, f.CompanyName, f.CompanyWebsite, f.Provider, f.Board,
			f.BoardCompany, strconv.Itoa(int(f.OpenJobs)), f.Reason, f.Action, f.Confidence})
	}
	cw.Flush()
	return cw.Error()
}
