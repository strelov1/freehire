package handler

import (
	"errors"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/sources"
)

// FindJob resolves the URL of a job posting in the wild to a freehire catalog
// slug, so the browser extension can tell that the page it is on is a job we
// already carry and switch from the ad-hoc text match to the curated card.
// Public; returns {"data": null} whenever the posting cannot be identified.
//
// The lookup is by the catalog's own dedup identity — (source, external_id),
// recovered from the URL — which is exact and served by a unique index. It
// replaced a company+title match that compared the page title against every
// catalog title: that could not use an index at all (the LIKE pattern was built
// from the column), so it degenerated into a sequential scan of millions of rows
// and timed out in production. It was also guesswork, and a page title is not
// something to guess from — the extension was sending "reCAPTCHA".
func (h *jobsHandlers) FindJob(c *fiber.Ctx) error {
	ref, ok := sources.RefFromURL(c.Query("url"))
	if !ok {
		return c.JSON(fiber.Map{"data": nil})
	}

	job, err := h.queries.GetJobBySourceExternalID(c.Context(), db.GetJobBySourceExternalIDParams{
		Source:     ref.Source,
		ExternalID: ref.ExternalID,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return c.JSON(fiber.Map{"data": nil})
		}
		return err
	}
	if job.PublicSlug == "" {
		return c.JSON(fiber.Map{"data": nil})
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"public_slug": job.PublicSlug}})
}
