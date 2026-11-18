//go:build integration

package main

import (
	"context"
	"encoding/xml"
	"errors"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/ingest/pipeline"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/externalid"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

type personioSweepFeed struct {
	body            string
	interrupt       context.CancelFunc
	peerInterrupted bool
}

func (f personioSweepFeed) GetXML(ctx context.Context, url string, out any) error {
	if f.peerInterrupted && strings.Contains(url, "https://untouched.") {
		return errors.New("rate: Wait(n=1) would exceed context deadline")
	}
	if f.interrupt != nil {
		f.interrupt()
		return ctx.Err()
	}
	return xml.Unmarshal([]byte(f.body), out)
}

func (personioSweepFeed) GetJSON(context.Context, string, any) error {
	return errors.New("unexpected JSON fallback")
}

func (personioSweepFeed) GetHTML(context.Context, string) (*html.Node, error) {
	return nil, errors.New("unexpected detail fallback")
}

// Exercise Runner -> provider dispatch -> the real SQL sweep with no busy second
// board to open the positive-ingest gate that masked single empty-board runs.
func TestSweepDispatchForZeroIngestBoardEvidence(t *testing.T) {
	for _, tc := range []struct {
		name            string
		provider        string
		body            string
		interrupt       bool
		peerInterrupted bool
		ambiguous       bool
		wantEmpty       bool
		wantClose       bool
	}{
		{name: "verified empty", provider: "personio", body: `<workzag-jobs/>`, wantEmpty: true, wantClose: true},
		{name: "finished empty survives interrupted peer", provider: "personio", body: `<workzag-jobs/>`, peerInterrupted: true, wantEmpty: true, wantClose: true},
		{name: "malformed envelope", provider: "personio", body: `<workzag-jobs><error>temporarily unavailable</error></workzag-jobs>`},
		{name: "partial document", provider: "personio", body: `<workzag-jobs><position><id>1</id><name>Engineer</name></position>`},
		{name: "interrupted", provider: "personio", interrupt: true},
		{name: "ambiguous board", provider: "personio", body: `<workzag-jobs/>`, ambiguous: true, wantEmpty: true},
		{name: "unmarked empty", provider: "smartrecruiters", wantEmpty: true},
		{name: "full catalog empty cannot source close", provider: "habr_career", wantEmpty: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := testdb.Pool(t)
			ctx := context.Background()
			q := db.New(pool)
			seed := func(board, id string, age time.Duration) int64 {
				t.Helper()
				j := sweepTestPosting(tc.provider, externalid.Namespace(board, id), "Data Engineer "+id)
				if err := newDBStore(pool, 1, newCrawledSet(), nil, defaultSeenPolicy()).Save(ctx, j); err != nil {
					t.Fatal(err)
				}
				row, err := q.GetJobBySourceExternalID(ctx, db.GetJobBySourceExternalIDParams{
					Source: tc.provider, ExternalID: externalid.Namespace(board, id),
				})
				if err != nil {
					t.Fatal(err)
				}
				ageJobForSweepTest(t, pool, row.ID, age)
				return row.ID
			}
			old := seed("emptied", "2002", staleAfter+time.Hour)
			recent := seed("emptied", "3003", staleAfter-time.Hour)
			otherBoard := seed("untouched", "4004", staleAfter+time.Hour)

			crawlCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			var src sources.Source = emptiedBoardSource{tc.provider}
			if tc.provider == "personio" {
				feed := personioSweepFeed{body: tc.body, peerInterrupted: tc.peerInterrupted}
				if tc.interrupt {
					feed.interrupt = cancel
				}
				src = sources.NewPersonio(feed)
			}
			crawled := newCrawledSet()
			runner := pipeline.Runner{
				Registry: map[string]sources.Source{tc.provider: src},
				Store:    newDBStore(pool, 1, crawled, nil, defaultSeenPolicy()),
			}
			entries := []sources.CompanyEntry{{Company: "Acme", Provider: tc.provider, Board: "emptied"}}
			if tc.peerInterrupted {
				entries = append(entries, sources.CompanyEntry{Company: "Acme", Provider: tc.provider, Board: "untouched"})
			}
			runStats, err := runner.Run(crawlCtx, entries)
			if (err != nil) != (tc.interrupt || tc.peerInterrupted) {
				t.Fatalf("crawl error = %v, interrupted = %v", err, tc.interrupt || tc.peerInterrupted)
			}
			stats := runStats[tc.provider]
			if stats.Ingested != 0 || stats.Refreshed != 0 {
				t.Fatalf("fixture must reach the zero-ingest dispatch: %+v", stats)
			}
			if (len(stats.EmptyBoards) == 1) != tc.wantEmpty {
				t.Fatalf("empty-board evidence = %v, want %v", stats.EmptyBoards, tc.wantEmpty)
			}
			registry := sources.Taxonomy()
			if tc.provider == "habr_career" {
				full := false
				for _, p := range sources.FullCatalogProviders(registry) {
					full = full || p == tc.provider
				}
				if !full {
					t.Fatal("fixture needs a full-catalog provider to guard against whole-source closure")
				}
			}
			ambiguous := map[string]bool{}
			if tc.ambiguous {
				ambiguous["emptied"] = true
			}
			// The sweep gets its independent context even when the crawl was interrupted.
			if failed := sweep(ctx, q, registry, runStats, crawled, ambiguous, tc.interrupt || tc.peerInterrupted, false); failed != 0 {
				t.Fatalf("sweep reported %d failures", failed)
			}
			for _, check := range []struct {
				id   int64
				want bool
			}{{old, tc.wantClose}, {recent, false}, {otherBoard, false}} {
				row, err := q.GetJob(ctx, check.id)
				if err != nil {
					t.Fatal(err)
				}
				if row.ClosedAt.Valid != check.want || check.want && row.ClosedReason != "unseen" {
					t.Errorf("job %d closed = %v (%s), want %v", check.id, row.ClosedAt.Valid, row.ClosedReason, check.want)
				}
				var deletes int
				if err := pool.QueryRow(ctx, "SELECT count(*) FROM search_delete_outbox WHERE job_id=$1", check.id).Scan(&deletes); err != nil {
					t.Fatal(err)
				}
				if (deletes == 1) != check.want {
					t.Errorf("job %d has %d delete events, closure wanted = %v", check.id, deletes, check.want)
				}
			}
		})
	}
}
