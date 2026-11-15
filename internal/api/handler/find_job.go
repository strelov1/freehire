package handler

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/ingest/atsboard"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
)

// catalogSlugForURL resolves the URL of a job page to the catalog posting it is, in three
// tiers, exact first: the (source, external_id) identity recovered from the URL (unique
// index; only a few ATS URL shapes carry one), then a verified application destination
// matched with its query identity intact, then the normalized match against the detail
// URL each source stored in jobs.url. No tier guesses from the page title.
//
// An empty URL resolves to nothing: it would normalize to "" and match a posting stored
// with an empty url.
//
// It is a free function because both /jobs/find and the extension's /jobs/resolve use it.
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

	// A verified destination can be the form itself; match it before canonicalisation
	// strips a provider's /apply suffix.
	if slug, err := queries.FindOpenJobByApplicationURL(ctx, pageURL); err == nil {
		return slug, nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}

	// The catalog stores only the detail page, so an apply-form URL is rewritten to it
	// first. Mostly offline; a SmartRecruiters one-click link needs the platform asked.
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
// browser extension can switch to the curated card. Public; returns {"data": null} when the
// posting cannot be identified.
//
// A miss also says whether the URL sits on a crawlable ATS board (meta.ats_board), via
// atsboard.Recognize, the one table of ATS hosts. The panel offers "add this job" only
// there or where the page marks itself up as a JobPosting.
func (h *jobsHandlers) FindJob(ctx context.Context, params apigen.FindJobParams) (*apigen.JobFindResult, error) {
	pageURL := strings.TrimSpace(params.URL.Or(""))
	slug, err := catalogSlugForURL(ctx, h.queries, h.postings, pageURL)
	if err != nil {
		return nil, err
	}
	if slug == "" {
		_, _, _, atsBoard := atsboard.Recognize(pageURL)
		return &apigen.JobFindResult{
			Data: apigen.NilJobFoundPosting{Null: true},
			Meta: apigen.NewOptJobFindMeta(apigen.JobFindMeta{AtsBoard: atsBoard}),
		}, nil
	}
	return &apigen.JobFindResult{Data: apigen.NewNilJobFoundPosting(apigen.JobFoundPosting{PublicSlug: slug})}, nil
}
