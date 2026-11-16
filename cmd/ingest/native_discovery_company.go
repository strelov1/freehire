package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"

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
	cached.once.Do(func() {
		cached.err = d.discoverCompanyBoard(ctx, key, discovered)
	})
	if cached.err != nil {
		return cached.err
	}
	if previous != nil {
		covered, err := newCoverage(d.pool).NonAggregatorCompanies(ctx, []string{previous.CompanySlug}, sources.AggregatorProviders(d.registry), previous.Countries)
		if err != nil {
			return err
		}
		if covered[previous.CompanySlug] {
			_, err := d.store.CloseCovered(ctx, "linkedin", []string{previous.ExternalID})
			return err
		}
	}
	return nil
}

func (d *nativeDiscovery) discoverCompanyBoard(ctx context.Context, slug string, discovered sources.Job) error {
	website := discovered.CompanyWebsite
	if website == "" {
		company, err := d.q.GetCompany(ctx, slug)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err == nil {
			var info struct {
				Website string `json:"website"`
			}
			if json.Unmarshal(company.CompanyInfo, &info) == nil {
				website = info.Website
			}
		}
	}
	if website == "" && discovered.CompanyProfileURL != "" {
		if reader, ok := d.registry["linkedin"].(interface {
			CompanyWebsite(context.Context, string) (string, error)
		}); ok {
			var err error
			website, err = reader.CompanyWebsite(ctx, discovered.CompanyProfileURL)
			if err != nil {
				log.Printf("ingest: employer website unavailable for LinkedIn discovery %s: %v", discovered.ExternalID, err)
			}
		}
	}
	if website == "" {
		log.Printf("ingest: LinkedIn discovery %s has no accessible ATS address or employer website; not publishing as LinkedIn", discovered.ExternalID)
		return nil
	}
	source, board, canonical, found := d.boards.ResolveCompany(ctx, website)
	if !found || d.registry[source] == nil || sources.ProviderKind(d.registry, source) != "ats" {
		log.Printf("ingest: no native ATS discovered for %s; not publishing as LinkedIn", discovered.ExternalID)
		return nil
	}
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	qtx := d.q.WithTx(tx)
	b, err := boardcatalog.NewInserter(boardcatalog.NewQueriesRepository(qtx), d.registry).Insert(ctx,
		boardcatalog.InsertInput{Provider: source, Board: board, Region: atsboard.Region(canonical), Company: discovered.Company}, boardcatalog.StatusPending)
	if err != nil && !errors.Is(err, boardcatalog.ErrDuplicateBoard) {
		return err
	}
	if err == nil && b.Status == boardcatalog.StatusRejected {
		return fmt.Errorf("discovered native board rejected by catalogue")
	}
	if err == nil {
		if err := ingestsched.NewQueriesRepository(qtx).AdoptDiscoveredProvider(ctx, source); err != nil {
			return err
		}
	}
	cfg, err := qtx.GetIngestSchedule(ctx, source)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	disabled := err == nil && !cfg.Enabled
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	if disabled {
		log.Printf("ingest: native provider %s is disabled; discovered board remains pending", source)
		return nil
	}
	log.Printf("ingest: native board %s/%s discovered from LinkedIn employer %s; queued for ordinary scheduled ingestion", source, board, slug)
	return nil
}
