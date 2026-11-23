//go:build integration

package main

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// A telegram posting that relays a LinkedIn job is closed by LinkedIn's closed marker; a t.me
// post is never probed and only the age rule closes it.
func TestTelegramLinkedInPostingClosesOnTheClosedMarker(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	q := db.New(pool)
	seed := func(name, url string, ageDays int) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx, `INSERT INTO jobs (source,external_id,url,title,public_slug,posted_at)
			VALUES ('telegram',$1,$2,$1,$1,now()-make_interval(days => $3)) RETURNING id`, name, url, ageDays).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	linkedin := seed("tg-linkedin", "https://www.linkedin.com/jobs/view/123/", 10)
	post := seed("tg-post", "https://t.me/jobs/7", 50)

	rows, err := q.SelectOrphanLivenessCandidates(ctx, classifiedProviderKeys())
	if err != nil {
		t.Fatal(err)
	}
	paced := map[string][]candidate{}
	if orphans, _ := routeUnsignalled(rows, paced); len(orphans) != 0 || len(paced["linkedin"]) != 1 || paced["linkedin"][0].id != linkedin {
		t.Fatalf("routing: orphans=%+v paced=%+v", orphans, paced)
	}
	closed := `<section class="top-card-layout"><h1 class="top-card-layout__title">Engineer</h1><a class="topcard__org-name-link">Acme</a>
<figcaption class="closed-job__flavor--closed">No longer accepting applications</figcaption></section><div class="show-more-less-html__markup">Build APIs.</div>`
	client := &http.Client{Transport: linkedInTransport(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(closed)), Header: make(http.Header), Request: r}, nil
	})}
	pass := linkedInPass(client, false)
	pass.interval = time.Millisecond
	var st stats
	for range closeThreshold { // two consecutive runs
		st.applyPacedPasses(ctx, q, map[string][]candidate{"linkedin": {paced["linkedin"][0]}}, []pacedPass{pass})
	}
	var reason string
	if err := pool.QueryRow(ctx, `SELECT closed_reason FROM jobs WHERE id=$1 AND closed_at IS NOT NULL`, linkedin).Scan(&reason); err != nil || reason != "probe_expired" {
		t.Fatalf("telegram linkedin posting not probe-closed: reason=%q err=%v", reason, err)
	}

	if n, err := q.CloseStaleUnsignalledJobs(ctx, db.CloseStaleUnsignalledJobsParams{Sources: unsignalledSources,
		Cutoff: pgtype.Timestamptz{Time: time.Now().Add(-expiryWindow), Valid: true}}); err != nil || n != 1 {
		t.Fatalf("age rule: count=%d err=%v", n, err)
	}
	if err := pool.QueryRow(ctx, `SELECT closed_reason FROM jobs WHERE id=$1`, post).Scan(&reason); err != nil || reason != "expired" {
		t.Fatalf("t.me posting: reason=%q err=%v", reason, err)
	}
}
