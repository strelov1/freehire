package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/ingest/atsboard"
	"github.com/strelov1/freehire/internal/ingest/boardcatalog"
	"github.com/strelov1/freehire/internal/ingest/ingestsched"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
)

func (d *nativeDiscovery) resolveCompany(ctx context.Context, discovered sources.Job, previous *db.Job) error {
	key := normalize.CompanySlug(discovered.Company)
	if previous != nil {
		key = previous.CompanySlug
	}
	value, _ := d.companies.LoadOrStore(key, &discoveredCompany{})
	cached := value.(*discoveredCompany)
	cached.mu.Lock()
	metadata := discovered.CompanyWebsite + "\x00" + discovered.CompanyProfileURL
	attemptErr, attempted := cached.attempts[metadata]
	if cached.done {
		attemptErr = nil
	} else if !attempted {
		cached.done, attemptErr = d.discoverCompanyBoard(ctx, key, discovered)
		if cached.attempts == nil {
			cached.attempts = map[string]error{}
		}
		cached.attempts[metadata] = attemptErr
	}
	cached.mu.Unlock()
	if attemptErr != nil {
		return attemptErr
	}
	if previous != nil {
		covered, err := newCoverage(d.pool).NonAggregatorCompanies(ctx, []string{previous.CompanySlug}, sources.AggregatorProviders(d.registry), previous.Countries)
		if err != nil {
			return err
		}
		if covered[previous.CompanySlug] {
			return d.retireCoveredDiscovery(ctx, *previous)
		}
	}
	return nil
}

func (d *nativeDiscovery) discoverCompanyBoard(ctx context.Context, slug string, discovered sources.Job) (bool, error) {
	destination, canonical, err := d.importer.LinkedInEmployerBoard(ctx, slug, discovered)
	if err != nil {
		return false, err
	}
	source, board := destination.Source, destination.Board
	found := source != "" && board != ""
	if !found || d.registry[source] == nil || sources.ProviderKind(d.registry, source) != "ats" {
		log.Printf("ingest: no native ATS discovered for %s; not publishing as LinkedIn", discovered.ExternalID)
		return false, nil
	}
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	qtx := d.q.WithTx(tx)
	b, err := boardcatalog.NewInserter(boardcatalog.NewQueriesRepository(qtx), d.registry).Insert(ctx,
		boardcatalog.InsertInput{Provider: source, Board: board, Region: atsboard.Region(canonical), Company: discovered.Company}, boardcatalog.StatusPending)
	if err != nil && !errors.Is(err, boardcatalog.ErrDuplicateBoard) {
		return false, err
	}
	if err == nil && b.Status == boardcatalog.StatusRejected {
		return false, fmt.Errorf("discovered native board rejected by catalogue")
	}
	if err == nil {
		if err := ingestsched.NewQueriesRepository(qtx).AdoptDiscoveredProvider(ctx, source); err != nil {
			return false, err
		}
	}
	cfg, err := qtx.GetIngestSchedule(ctx, source)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	disabled := err == nil && !cfg.Enabled
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	if disabled {
		log.Printf("ingest: native provider %s is disabled; discovered board remains pending", strconv.Quote(source))
		return true, nil
	}
	log.Printf("ingest: native board %s/%s discovered from LinkedIn employer %s; queued for ordinary scheduled ingestion", strconv.Quote(source), strconv.Quote(board), strconv.Quote(slug))
	return true, nil
}
