//go:build integration

package main

import (
	"context"
	"encoding/xml"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/ingest/pipeline"
	"github.com/strelov1/freehire/internal/ingest/sources"
	sweeppolicy "github.com/strelov1/freehire/internal/ingest/sweep"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

type clinchHydrationPages struct {
	blocked bool
	body    string
}

func (*clinchHydrationPages) GetXML(_ context.Context, _ string, v any) error {
	return xml.Unmarshal([]byte(`<urlset><url>
<loc>https://careers.example.com/jobs/backend-engineer-london-united-kingdom</loc>
<lastmod>2026-11-15</lastmod></url></urlset>`), v)
}
func (p *clinchHydrationPages) GetHTML(_ context.Context, url string) (*html.Node, error) {
	if p.blocked {
		return nil, &sources.ChallengeError{URL: url}
	}
	return html.Parse(strings.NewReader(`<div class="job-description">` + p.body + `</div>`))
}

func TestClinchHydrationPreservesStoredBodyUntilRecovery(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	policy := defaultSeenPolicy()
	policy.bodies = sweeppolicy.BodyRefresh{Cutoff: time.Now().AddDate(0, 0, -30), Slices: 1, Slot: 0}
	store := newDBStore(pool, 1, newCrawledSet(), nil, policy)
	pages := &clinchHydrationPages{body: "<p>Build backend services in Go.</p>"}
	runner := pipeline.Runner{Store: store, Registry: map[string]sources.Source{"clinch": sources.NewClinch(pages, pages)}}
	entries := []sources.CompanyEntry{{Provider: "clinch", Board: "careers.example.com", Company: "Acme"}}
	run := func() pipeline.Stats {
		t.Helper()
		stats, err := runner.Run(ctx, entries)
		if err != nil {
			t.Fatal(err)
		}
		return stats["clinch"]
	}
	run()
	const id = "careers.example.com:backend-engineer-london-united-kingdom"
	var body string
	var hydrated time.Time
	read := func() {
		t.Helper()
		if err := pool.QueryRow(ctx, `SELECT j.description,a.hydrated_at FROM jobs j
 JOIN job_activity a ON a.job_id=j.id
 WHERE j.source='clinch' AND j.external_id=$1`, id).Scan(&body, &hydrated); err != nil {
			t.Fatal(err)
		}
	}
	read()
	original := body
	if _, err := pool.Exec(ctx, `UPDATE job_activity a SET hydrated_at=now()-interval '40 days'
 FROM jobs j WHERE j.id=a.job_id AND j.source='clinch' AND j.external_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	read()
	before := hydrated
	pages.blocked = true
	if st := run(); st.Unreadable != 1 {
		t.Fatalf("challenge not counted: %+v", st)
	}
	read()
	if body != original || !hydrated.Equal(before) {
		t.Fatal("challenge erased body or stamped unread detail")
	}
	pages.blocked = false
	pages.body = "<p>Edited backend services in Go and PostgreSQL.</p>"
	run()
	read()
	if body != pages.body || !hydrated.After(before) {
		t.Fatalf("due body did not recover: %q %v", body, hydrated)
	}
	before = hydrated
	run()
	read()
	if !hydrated.Equal(before) {
		t.Fatal("known body stamped without a reread")
	}
	// A persisted empty fallback from the old adapter is also eligible immediately.
	if _, err := pool.Exec(ctx, `UPDATE jobs SET description='', content_hash=''
 WHERE source='clinch' AND external_id=$1`, id); err != nil {
		t.Fatal(err)
	}
	run()
	read()
	if body != pages.body {
		t.Fatal("legacy empty fallback did not hydrate")
	}
}
