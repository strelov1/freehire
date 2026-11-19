package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// RecordCompanyWebsite stores a posting's stated employer website on its company row
// (company_info.feed_website) when the row has none yet, once per company per run. Implements pipeline.CompanyWebsiteRecorder.
func (s *dbStore) RecordCompanyWebsite(ctx context.Context, companySlug, website string) error {
	if _, done := s.websites.LoadOrStore(companySlug, struct{}{}); done {
		return nil
	}
	err := s.q.SetCompanyFeedWebsiteIfAbsent(ctx, db.SetCompanyFeedWebsiteIfAbsentParams{
		Slug:    companySlug,
		Website: website,
	})
	if err != nil {
		s.websites.Delete(companySlug) // the next posting of this company tries again
	}
	return err
}

// CompanyDescriptionDue implements pipeline.CompanyDescriptionFiller: whether the company
// still lacks a description and has had no answered fetch since checkedSince.
func (s *dbStore) CompanyDescriptionDue(ctx context.Context, slug string, checkedSince time.Time) (bool, error) {
	due, err := s.q.CompanyDescriptionDue(ctx, db.CompanyDescriptionDueParams{
		Slug:         slug,
		CheckedSince: pgconv.Instant(checkedSince),
	})
	if err != nil {
		return false, fmt.Errorf("company description due %s: %w", slug, err)
	}
	return due, nil
}

// RecordCompanyDescription implements pipeline.CompanyDescriptionFiller: record an
// answered company-description fetch (see sources.CompanyDescriber). A blank answer only
// stamps the attempt; a non-empty one also fills the company's company_info gap. Stored
// under company_info.summary rather than the short tagline field — this text is raw
// employer-authored prose, not the terse, curated-style values tagline is reserved for
// (the YC directory's one_liner, the Wikipedia backfill's Wikidata description).
func (s *dbStore) RecordCompanyDescription(ctx context.Context, slug, name, description string) error {
	if description == "" {
		if err := s.q.MarkCompanyDescriptionChecked(ctx, db.MarkCompanyDescriptionCheckedParams{Slug: slug, Name: name}); err != nil {
			return fmt.Errorf("mark company description checked %s: %w", slug, err)
		}
		return nil
	}
	info, err := json.Marshal(map[string]string{"summary": description})
	if err != nil {
		return fmt.Errorf("fill company description %s: encode company_info: %w", slug, err)
	}
	if err := s.q.FillCompanyDescriptionFromIngest(ctx, db.FillCompanyDescriptionFromIngestParams{
		Slug:        slug,
		Name:        name,
		Tagline:     pgtype.Text{},
		CompanyInfo: info,
	}); err != nil {
		return fmt.Errorf("fill company description %s: %w", slug, err)
	}
	return nil
}

// CanonicalCompanySlugs implements pipeline.AliasLookup: for a batch of folded company slugs,
// the canonical slug a merge elected for each employer that has one.
//
// Read rather than written here, like ExistingExternalIDs — the store already owns the pool,
// and the pipeline asks for this once per board run, not once per posting.
func (s *dbStore) CanonicalCompanySlugs(ctx context.Context, foldedKeys []string) (map[string]string, error) {
	rows, err := s.q.ResolveCompanySlugAliases(ctx, foldedKeys)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(rows))
	for _, r := range rows {
		// One canonical slug per folded key is the writer's invariant, not the schema's (see
		// the query). Rows arrive ordered, so the first wins deterministically; a second,
		// different one means two waves elected different winners for one employer, which
		// would otherwise show up only as a company that keeps changing url.
		if prev, ok := out[r.FoldedKey]; ok {
			if prev != r.CanonicalSlug {
				log.Printf("ingest: company alias conflict: folded key %q maps to both %q and %q; using %q",
					r.FoldedKey, prev, r.CanonicalSlug, prev)
			}
			continue
		}
		out[r.FoldedKey] = r.CanonicalSlug
	}
	return out, nil
}
