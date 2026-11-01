package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/strelov1/freehire/internal/ingest/boardcatalog"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
)

type feedAuditOptions struct {
	Apply    bool
	Limit    int
	Pace     float64
	Rotating bool
	Timeout  time.Duration
}

func (o feedAuditOptions) validate(provider string) error {
	if o.Apply {
		return fmt.Errorf("-audit-feeds cannot be combined with -apply")
	}
	if o.Limit < 1 {
		return fmt.Errorf("-audit-limit must be positive")
	}
	if o.Timeout <= 0 {
		return fmt.Errorf("-audit-timeout must be positive")
	}
	if o.Pace < 0 || math.IsNaN(o.Pace) || math.IsInf(o.Pace, 0) {
		return fmt.Errorf("-pace must be finite and non-negative")
	}
	if provider != "scalis" && provider != "teamtailor" {
		return fmt.Errorf("feed audit is not supported for %q (supported: scalis, teamtailor)", provider)
	}
	return nil
}

func runFeedAuditCommand(provider, seedPath string, opts feedAuditOptions) int {
	if err := opts.validate(provider); err != nil {
		log.Printf("feed audit: %v", err)
		return 2
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	entries, err := loadFeedAuditBoards(ctx, provider, seedPath)
	if err != nil {
		log.Printf("feed audit: %v", err)
		return 1
	}
	selected, total, err := selectAuditBoards(provider, entries, opts.Limit)
	if err != nil {
		log.Printf("feed audit: %v", err)
		return 1
	}
	log.Printf("feed audit: provider=%s selected=%d auditing=%d limit=%d", provider, total, len(selected), opts.Limit)
	if len(selected) == 0 {
		return 0
	}
	client, err := egressFor(provider, opts.Rotating)
	if err != nil {
		log.Printf("feed audit: %v", err)
		return 1
	}
	pace := opts.Pace
	if pace == 0 {
		pace = 1
	}
	auditor := feedAuditor{client: paced(client, pace), timeout: opts.Timeout}
	if err := auditor.write(ctx, selected, os.Stdout); err != nil {
		log.Printf("feed audit: %v", err)
		return 1
	}
	return 0
}

func loadFeedAuditBoards(ctx context.Context, provider, seedPath string) ([]sources.CompanyEntry, error) {
	if seedPath != "" {
		seeds, err := loadSeedItems(seedPath)
		if err != nil {
			return nil, err
		}
		entries := make([]sources.CompanyEntry, 0, len(seeds))
		for _, seed := range seeds {
			entries = append(entries, sources.CompanyEntry{Provider: provider, Board: seed.Board, Company: seed.Company})
		}
		return entries, nil
	}
	ctx, _, pool, cleanup, err := worker.Bootstrap(ctx)
	if err != nil {
		return nil, fmt.Errorf("load catalog: %w", err)
	}
	defer cleanup()
	return boardcatalog.LoadForProvider(ctx, boardcatalog.NewQueriesRepository(db.New(pool)), provider)
}

// Unlike newBoards this deliberately keeps catalog entries. Seed mode and catalog mode
// share normalization, deterministic ordering and limits; no probe writes or filters them.
func selectAuditBoards(provider string, entries []sources.CompanyEntry, limit int) ([]sources.CompanyEntry, int, error) {
	if limit < 1 {
		return nil, 0, fmt.Errorf("audit limit must be positive")
	}
	byBoard := map[string]sources.CompanyEntry{}
	for _, e := range entries {
		e.Provider = provider
		e.Board = strings.ToLower(strings.TrimSpace(e.Board))
		if provider == "teamtailor" {
			e.Board = (teamtailorProber{}).boardID(e.Board)
		}
		if _, err := auditFeedURL(provider, e.Board); err != nil {
			return nil, 0, err
		}
		previous, exists := byBoard[e.Board]
		if exists && previous.Company != "" {
			continue
		}
		byBoard[e.Board] = e
	}
	selected := make([]sources.CompanyEntry, 0, len(byBoard))
	for _, e := range byBoard {
		if e.Company == "" {
			e.Company = e.Board
		}
		selected = append(selected, e)
	}
	slices.SortFunc(selected, func(a, b sources.CompanyEntry) int { return strings.Compare(a.Board, b.Board) })
	total := len(selected)
	if len(selected) > limit {
		selected = selected[:limit]
	}
	return selected, total, nil
}

type feedAuditor struct {
	client  httpClient
	timeout time.Duration
}

func (a feedAuditor) write(ctx context.Context, entries []sources.CompanyEntry, w io.Writer) error {
	encoder := json.NewEncoder(w)
	failures := 0
	for _, e := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		// Baseline detail requests share this deadline and the run's rate limiter.
		boardCtx, cancel := context.WithTimeout(ctx, a.timeout)
		report := auditFeedBoard(boardCtx, a.client, e)
		cancel()
		if err := encoder.Encode(report); err != nil {
			return fmt.Errorf("write audit: %w", err)
		}
		log.Printf("feed audit: %s/%s status=%s feed_jobs=%d feed_requests=%d adapter_requests=%d",
			e.Provider, e.Board, report.Status, report.Feed.Jobs, report.FeedRequests, report.AdapterRequests)
		if report.Status == "feed_failed" || report.Status == "baseline_failed" {
			failures++
		}
	}
	if failures > 0 {
		return fmt.Errorf("%d of %d board audits failed; partial evidence is in the report", failures, len(entries))
	}
	return ctx.Err()
}
