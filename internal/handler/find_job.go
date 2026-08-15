package handler

import (
	"context"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/sources"
)

// catalogSlugForURL resolves the URL of a job page to the catalog posting it is, in two
// tiers, exact first. The catalog's own dedup identity — (source, external_id), recovered
// from the URL — is served by a unique index, but only a handful of ATS URL shapes carry
// an identity a parser can read. So a URL that names no identity, or names one we have no
// row for, falls through to matching the page against the detail URL each source stored in
// jobs.url (normalized on both sides, see migration 0042): for aggregators and most boards
// that IS the page the user is standing on.
//
// An empty URL resolves to nothing: it would normalize to "" in the second tier and match
// a posting stored with an empty url.
//
// Neither tier guesses. An earlier version matched the page title against every catalog
// title, which could not use an index at all (the LIKE pattern was built from the column),
// so it degenerated into a sequential scan of millions of rows and timed out in production
// — and it was guesswork besides, on a page title that is not something to guess from (the
// extension was sending "reCAPTCHA").
//
// It is a free function rather than a method because two feature handlers need it: the
// public /jobs/find read and the extension's /jobs/resolve import.
func catalogSlugForURL(ctx context.Context, queries *db.Queries, postings sources.PostingURLResolver, pageURL string) (string, error) {
	if pageURL == "" {
		return "", nil
	}

	if ref, ok := sources.RefFromURL(pageURL); ok {
		job, err := queries.GetJobBySourceExternalID(ctx, db.GetJobBySourceExternalIDParams{
			Source:     ref.Source,
			ExternalID: ref.ExternalID,
		})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return "", err
		}
		if err == nil && job.PublicSlug != "" {
			return job.PublicSlug, nil
		}
	}

	// The apply form is the same posting under a different path, and it is where a
	// candidate stands when they ask about a vacancy. The catalog stores only the
	// detail page, so the form has to be rewritten to it before the match. Most of
	// that rewrite is offline; a SmartRecruiters one-click link identifies its posting
	// by an id the catalogue never kept, so the resolver asks the platform.
	slug, err := queries.FindOpenJobByURL(ctx, postings.CanonicalPostingURL(ctx, pageURL))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	return slug, nil
}

// FindJob resolves the URL of a job posting in the wild to a freehire catalog slug, so the
// browser extension can tell that the page it is on is a job we already carry and switch
// from the ad-hoc text match to the curated card. Public; returns {"data": null} whenever
// the posting cannot be identified.
func (h *jobsHandlers) FindJob(c *fiber.Ctx) error {
	slug, err := catalogSlugForURL(c.Context(), h.queries, h.postings, strings.TrimSpace(c.Query("url")))
	if err != nil {
		return err
	}
	if slug == "" {
		return c.JSON(fiber.Map{"data": nil})
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"public_slug": slug}})
}
