//go:build integration

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"testing"
	"time"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/ingest/pipeline"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/jobderive"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/externalid"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

type talentbrewSweepFeed struct{ failSecond bool }

func (f talentbrewSweepFeed) GetJSON(_ context.Context, raw string, out any) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	page := u.Query().Get("CurrentPage")
	if page != "1" && page != "2" {
		return fmt.Errorf("unexpected listing page %q", page)
	}
	if page == "2" && f.failSecond {
		return errors.New("second listing page temporarily unavailable")
	}
	fragment := fmt.Sprintf(`<section id="search-results" data-total-pages="2"><a data-job-id="%s" href="/job/engineer/%s">Data Engineer</a></section>`, page, page)
	body, err := json.Marshal(map[string]any{"results": fragment, "hasJobs": true})
	if err != nil {
		return err
	}
	return json.Unmarshal(body, out)
}

func (talentbrewSweepFeed) GetHTML(context.Context, string) (*html.Node, error) {
	return nil, errors.New("known listing rows must refresh without detail requests")
}

// A positive first page must never authorize a company sweep if pagination failed.
// The complete control exercises the existing company path, not a new board marker.
func TestTalentbrewPaginationControlsCompanySweep(t *testing.T) {
	for _, failSecond := range []bool{true, false} {
		t.Run(fmt.Sprintf("second_page_failure=%v", failSecond), func(t *testing.T) {
			pool := testdb.Pool(t)
			ctx := context.Background()
			q := db.New(pool)
			const provider, board = "talentbrew", "careers.aldi.us"
			seed := func(id, company string, age time.Duration) int64 {
				t.Helper()
				externalID := externalid.Namespace(board, id)
				j, err := job.New(job.Draft{
					Input: jobderive.Input{Source: provider, ExternalID: externalID,
						Title: "Data Engineer " + id, Company: company, Location: "Remote",
						Description: "<p>Build data pipelines using Python.</p>"},
					URL: "https://careers.aldi.us/job/engineer/" + id,
				})
				if err != nil {
					t.Fatal(err)
				}
				if err := newDBStore(pool, 1, nil, nil, defaultSeenPolicy()).Save(ctx, j); err != nil {
					t.Fatal(err)
				}
				row, err := q.GetJobBySourceExternalID(ctx, db.GetJobBySourceExternalIDParams{Source: provider, ExternalID: externalID})
				if err != nil {
					t.Fatal(err)
				}
				ageJobForSweepTest(t, pool, row.ID, age)
				return row.ID
			}
			first := seed("1", "Acme", staleAfter+time.Hour)
			second := seed("2", "Acme", staleAfter+time.Hour)
			omitted := seed("3", "Acme", staleAfter+time.Hour)
			fresh := seed("4", "Acme", staleAfter-time.Hour)
			unrelated := seed("5", "Other Employer", staleAfter+time.Hour)
			crawled := newCrawledSet()
			runner := pipeline.Runner{
				Registry: map[string]sources.Source{provider: sources.NewTalentbrew(talentbrewSweepFeed{failSecond})},
				Store:    newDBStore(pool, 1, crawled, nil, defaultSeenPolicy()),
			}
			started := time.Now()
			stats, err := runner.Run(ctx, []sources.CompanyEntry{{Provider: provider, Board: board, Company: "Acme"}})
			if err != nil {
				t.Fatal(err)
			}
			if failed := sweep(ctx, q, sources.Taxonomy(), stats, crawled, nil, false, false); failed != 0 {
				t.Fatalf("sweep failures: %d", failed)
			}
			for _, id := range []int64{first, second, omitted, fresh, unrelated} {
				row, err := q.GetJob(ctx, id)
				if err != nil {
					t.Fatal(err)
				}
				wantClose := id == omitted && !failSecond
				if row.ClosedAt.Valid != wantClose || wantClose && row.ClosedReason != "unseen" {
					t.Errorf("job %d closed=%v reason=%q, want closure=%v", id, row.ClosedAt.Valid, row.ClosedReason, wantClose)
				}
				wantRefresh := (id == first || id == second) && !failSecond
				if row.LastSeenAt.Time.After(started) != wantRefresh {
					t.Errorf("job %d refreshed=%v, want %v", id, row.LastSeenAt.Time.After(started), wantRefresh)
				}
				var deletes int
				if err := pool.QueryRow(ctx, "SELECT count(*) FROM search_delete_outbox WHERE job_id=$1", id).Scan(&deletes); err != nil {
					t.Fatal(err)
				}
				if (deletes == 1) != wantClose {
					t.Errorf("job %d delete events=%d, want closure=%v", id, deletes, wantClose)
				}
			}
			if failSecond && (stats[provider].Failed != 1 || stats[provider].Ingested+stats[provider].Refreshed != 0) {
				t.Errorf("incomplete board stats = %+v", stats[provider])
			}
			if !failSecond && (stats[provider].Failed != 0 || stats[provider].Ingested+stats[provider].Refreshed != 2) {
				t.Errorf("complete board stats = %+v", stats[provider])
			}
		})
	}
}
