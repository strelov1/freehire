package handler

import (
	"errors"
	"log"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/contribution"
)

// resolveJobRequest is the one field the endpoint takes: the page the caller is looking at.
type resolveJobRequest struct {
	URL string `json:"url"`
}

// ResolveJob answers "is this page a vacancy freehire carries, and if not, take it" — the
// endpoint behind the browser extension's "add this page". Authenticated, and rate limited
// with board contributions because both make the server fetch a user-supplied URL.
//
// Three outcomes, one shape:
//
//	200 found    — the catalog already carries the posting (no fetch, no write)
//	201 imported — a link-source adapter parsed the page and it is now in the catalog
//	202 queued   — nothing could parse it, so the link went to manual triage
//
// The catalog lookup runs FIRST, and not only to save a fetch: the last-resort resolver
// writes under source "weblink" keyed by the page URL, so importing a posting we already
// carry from an aggregator would add a second row under a different identity — a duplicate
// the ingest dedup passes (which work per company+title) would not collapse. Checking
// first is what makes this safe to point at any page.
func (h *contributionHandlers) ResolveJob(c *fiber.Ctx) error {
	var in resolveJobRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	pageURL := strings.TrimSpace(in.URL)
	if !isHTTPURL(pageURL) {
		return fiber.NewError(fiber.StatusUnprocessableEntity, "not a job page URL")
	}

	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	slug, err := catalogSlugForURL(c.Context(), h.queries, pageURL)
	if err != nil {
		return err
	}
	if slug != "" {
		return c.JSON(fiber.Map{"data": fiber.Map{"public_slug": slug, "status": "found"}})
	}

	res, imported, err := h.imports.Import(c.Context(), pageURL)
	if err != nil {
		// A transient fetch or parse failure, not a verdict on the page: the link is worth
		// keeping, so it falls through to triage rather than surfacing as a 5xx.
		log.Printf("resolve-job: import %s: %v", pageURL, err)
	}
	if imported {
		return c.Status(fiber.StatusCreated).
			JSON(fiber.Map{"data": fiber.Map{"public_slug": res.PublicSlug, "status": "imported"}})
	}

	// Nothing read the page. Record the link so a maintainer can see what we are missing —
	// the contribution service triages a recognised novel board as pending (and the
	// submitter is rewarded for it) and anything else as review.
	if err := h.queueForTriage(c, userID, pageURL); err != nil {
		return err
	}
	return c.Status(fiber.StatusAccepted).
		JSON(fiber.Map{"data": fiber.Map{"public_slug": nil, "status": "queued"}})
}

// queueForTriage hands an unimportable link to the contribution service. A board we
// already crawl, or one someone already contributed, is not an error the caller can act
// on — the link is known to us either way — so both are treated as queued.
func (h *contributionHandlers) queueForTriage(c *fiber.Ctx, userID int64, pageURL string) error {
	res, err := h.contribution.Submit(c.Context(), contribution.SubmitInput{
		SubmittedBy: userID,
		URL:         pageURL,
		Surface:     contribution.SurfaceExtension,
	})
	switch {
	case err == nil:
		if res.Rewardable && res.Contribution.Status == contribution.StatusPending {
			rewardContribution(c.Context(), h.credits, userID, res.Contribution.ID)
		}
		return nil
	case errors.Is(err, contribution.ErrBoardAlreadyTracked),
		errors.Is(err, contribution.ErrBoardAlreadyContributed):
		return nil
	default:
		return err
	}
}

// isHTTPURL reports whether raw is an absolute http(s) URL — the only thing worth
// fetching, and the same gate the contribution flow applies to a pasted link.
func isHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
