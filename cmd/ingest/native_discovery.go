package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/ingest/boardresolve"
	"github.com/strelov1/freehire/internal/ingest/linkimport"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/externalid"
)

type companyBoardResolver interface {
	linkimport.BoardResolver
	ResolveCompany(context.Context, string) (string, string, string, bool)
}

type nativeDiscovery struct {
	pool      *pgxpool.Pool
	q         *db.Queries
	registry  map[string]sources.Source
	store     *dbStore
	importer  *linkimport.Importer
	boards    companyBoardResolver
	companies sync.Map
}

type discoveredCompany struct {
	mu       sync.Mutex
	done     bool
	attempts map[string]error
}

func newNativeDiscovery(pool *pgxpool.Pool, registry map[string]sources.Source, store *dbStore) *nativeDiscovery {
	q, boards := db.New(pool), boardresolve.New()
	return &nativeDiscovery{pool: pool, q: q, registry: registry, store: store, boards: boards,
		importer: linkimport.New(pool, q, nil, sources.NewClient(), registry, boards)}
}

func (d *nativeDiscovery) Resolve(ctx context.Context, entry sources.CompanyEntry, discovered sources.Job) error {
	origin, err := d.q.GetJobBySourceExternalID(ctx, db.GetJobBySourceExternalIDParams{Source: "linkedin", ExternalID: externalid.Namespace(entry.Board, discovered.ExternalID)})
	if errors.Is(err, pgx.ErrNoRows) {
		origin, err = d.q.GetJobBySourceExternalID(ctx, db.GetJobBySourceExternalIDParams{Source: "linkedin", ExternalID: discovered.ExternalID})
	}
	if errors.Is(err, pgx.ErrNoRows) {
		origin, err = d.q.FindLinkedInJobByURL(ctx, discovered.URL)
	}
	var previous *db.Job
	if err == nil {
		if origin.IsPrivate {
			return fmt.Errorf("native discovery refuses a private origin")
		}
		if (!origin.ClosedAt.Valid || origin.ClosedReason == linkimport.NativeSourcePendingReason) && !origin.DuplicateOf.Valid {
			previous = &origin
		}
	} else if errors.Is(err, pgx.ErrNoRows) {
		origin = db.Job{Source: "linkedin", Title: discovered.Title, Company: discovered.Company, URL: discovered.URL}
	} else {
		return err
	}
	if previous != nil && discovered.ApplyMethod != sources.ApplyOnSite {
		if _, err := d.q.WithholdLinkedInDiscovery(ctx, previous.ID); err != nil {
			return err
		}
	}
	raw := discovered.ExternalApplyURL
	if raw == "" {
		raw = origin.ApplicationUrl
	}
	if raw == "" {
		return d.resolveCompany(ctx, discovered, previous)
	}
	native, err := d.importer.ResolveNative(ctx, origin, raw, discovered.CompanyWebsite)
	if err != nil {
		return err
	}
	// A native posting already in the catalogue remains canonical. Keep any old
	// LinkedIn history on its own row and retire that discovery copy through the
	// same coverage policy as ordinary aggregator ingestion.
	if previous != nil {
		_, err := d.q.GetJobBySourceExternalID(ctx, db.GetJobBySourceExternalIDParams{Source: native.Entry.Provider, ExternalID: native.Job.ExternalID})
		if err == nil {
			if _, err := d.importer.WriteNative(ctx, nil, native); err != nil {
				return err
			}
			return d.retireCoveredDiscovery(ctx, *previous)
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
	}
	result, err := d.importer.WriteNative(ctx, previous, native)
	if err == nil {
		log.Printf("ingest: LinkedIn discovery %s now owned by %s/%s (%s)", discovered.ExternalID, result.Source, result.ExternalID, result.PublicSlug)
	}
	return err
}
