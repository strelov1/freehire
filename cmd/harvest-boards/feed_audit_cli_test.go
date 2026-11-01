package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

func TestFeedAuditOptionsRejectWritesAndUnboundedRuns(t *testing.T) {
	for _, opts := range []feedAuditOptions{
		{Limit: 20, Timeout: 0},
		{Limit: 20, Timeout: -time.Second},
		{Apply: true, Limit: 20, Timeout: time.Minute},
		{Limit: 0, Timeout: time.Minute},
		{Limit: -1, Timeout: time.Minute},
		{Limit: 20, Timeout: time.Minute, Pace: -1},
		{Limit: 20, Timeout: time.Minute, Pace: math.NaN()},
		{Limit: 20, Timeout: time.Minute, Pace: math.Inf(1)},
	} {
		if err := opts.validate("scalis"); err == nil {
			t.Fatalf("accepted %+v", opts)
		}
	}
	if err := (feedAuditOptions{Limit: 20, Timeout: time.Minute}).validate("workday"); err == nil {
		t.Fatal("accepted unsupported provider")
	}
}

func TestFeedAuditSeedsKeepKnownBoardsWithoutDatabase(t *testing.T) {
	t.Setenv("DATABASE_URL", "invalid database must not be opened")
	path := filepath.Join(t.TempDir(), "boards.json")
	seed := `[{"board":"tibber","company":"Tibber"},
{"board":"jobs.acme.com"},{"board":"tibber.teamtailor.com"}]`
	if err := os.WriteFile(path, []byte(seed), 0600); err != nil {
		t.Fatal(err)
	}
	entries, err := loadFeedAuditBoards(context.Background(), "teamtailor", path)
	if err != nil {
		t.Fatal(err)
	}
	selected, total, err := selectAuditBoards("teamtailor", entries, 1)
	if err != nil {
		t.Fatal(err)
	}
	if total != 2 || len(selected) != 1 || selected[0].Board != "jobs.acme.com" {
		t.Fatalf("selection = %+v, total %d", selected, total)
	}
	selected, total, err = selectAuditBoards("teamtailor", entries, 20)
	if err != nil || total != 2 || selected[1].Company != "Tibber" {
		t.Fatalf("selection = %+v, total %d, err %v", selected, total, err)
	}
}

type auditStatusClient struct {
	httpClient
	code int
}

func (c auditStatusClient) GetXML(context.Context, string, any) error {
	return &sources.StatusError{Code: c.code}
}

type auditBrokenWriter struct{}

func (auditBrokenWriter) Write([]byte) (int, error) { return 0, errors.New("output closed") }

func TestFeedAuditReportsUnavailableAndRefusedSeparately(t *testing.T) {
	entries := []sources.CompanyEntry{{Provider: "teamtailor", Board: "jobs.acme.com"}}
	for _, tc := range []struct {
		code    int
		status  string
		wantErr bool
	}{
		{code: 404, status: "feed_unavailable"},
		{code: 403, status: "feed_failed", wantErr: true},
		{code: 429, status: "feed_failed", wantErr: true},
	} {
		var buf bytes.Buffer
		err := (feedAuditor{client: auditStatusClient{code: tc.code}, timeout: time.Minute}).write(context.Background(), entries, &buf)
		if (err != nil) != tc.wantErr {
			t.Fatalf("status %d: error %v", tc.code, err)
		}
		var report feedAuditReport
		if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
			t.Fatal(err)
		}
		if report.Status != tc.status || report.Comparison != nil || report.FeedRequests != 1 {
			t.Fatalf("report = %+v", report)
		}
	}
	if err := (feedAuditor{client: auditStatusClient{code: 404}, timeout: time.Minute}).write(context.Background(), entries, auditBrokenWriter{}); err == nil {
		t.Fatal("ignored output error")
	}
}

// Ignoring the configured deadline would hang on an unresponsive feed.
type auditWaitingClient struct{ httpClient }

func (auditWaitingClient) GetXML(ctx context.Context, _ string, _ any) error {
	<-ctx.Done()
	return ctx.Err()
}
func TestFeedAuditHonorsConfiguredTimeout(t *testing.T) {
	var buf bytes.Buffer
	auditor := feedAuditor{client: auditWaitingClient{}, timeout: time.Millisecond}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := auditor.write(ctx, []sources.CompanyEntry{{Provider: "scalis", Board: "acme"}}, &buf)
	if err == nil || ctx.Err() != nil {
		t.Fatalf("configured deadline not respected: %v, parent %v", err, ctx.Err())
	}
	var report feedAuditReport
	if err := json.Unmarshal(buf.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Status != "feed_failed" || report.Comparison != nil {
		t.Fatalf("report = %+v", report)
	}
}
