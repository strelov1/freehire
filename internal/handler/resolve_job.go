package handler

import (
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"
)

// resolveJobRequest is what the endpoint takes: the page the caller is looking at, and
// optionally which surface they are on. The surface is recorded, never enforced — an older
// client that sends none is served exactly the same.
type resolveJobRequest struct {
	URL     string `json:"url"`
	Surface string `json:"surface"`
}

// Intake outcomes. They differ only in what the system already knew about the link, which is
// precisely what the caller needs to be told.
const (
	// outcomeFound — the catalog already carries this vacancy. Reached two ways: the URL
	// itself is stored (no fetch, no write), or the page turned out to be a second copy of a
	// posting we hold under another source, which collapses onto it.
	outcomeFound = "found"
	// outcomeTracked — we already crawl the board; this posting just had not landed yet, so it
	// was imported now and the company we cover is named.
	outcomeTracked = "tracked"
	// outcomeImported — imported, and the board behind it queued for onboarding.
	outcomeImported = "imported"
	// outcomeReview — imported, but the page names no board we can crawl (a careers site on
	// the company's own domain), so the link went to manual triage rather than onboarding.
	// Distinct from outcomeImported precisely because no crawl follows from it.
	outcomeReview = "review"
	// outcomeQueued — nothing could read the page; the link went to manual triage.
	outcomeQueued = "queued"
)

// ResolveJob is the HTTP door onto the shared intake sequence (see intakeService), used by the
// website, the browser extension and the CLI. Authenticated, and rate limited with board
// contributions because both make the server fetch a user-supplied URL.
//
// Four outcomes, one shape:
//
//	200 found    — the catalog already carries the vacancy: the URL is stored (no fetch, no
//	               write), or the imported page collapsed onto a posting we already had
//	201 tracked  — we crawl this board already; the posting had not landed, so it was imported
//	               now, and the company we cover is named
//	201 imported — imported, and the board behind it queued for onboarding
//	201 review   — imported, but no crawlable board could be named, so the link went to triage
//	202 queued   — nothing could read the page, so the link went to manual triage
//
// company_slug rides along on every 201 whose employer the catalog already carries, whatever
// became of the board — the two are independent, and a surface that reads "new company" off
// the status alone gets it wrong for a company we reach through a different ATS.
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

	out, err := h.intake.Resolve(c.Context(), userID, pageURL, in.Surface)
	if err != nil {
		return err
	}
	switch out.Status {
	case outcomeFound:
		body := fiber.Map{"public_slug": out.PublicSlug, "status": out.Status}
		if out.CompanySlug != "" {
			body["company_slug"] = out.CompanySlug
		}
		return c.JSON(fiber.Map{"data": body})
	case outcomeQueued:
		return c.Status(fiber.StatusAccepted).
			JSON(fiber.Map{"data": fiber.Map{"public_slug": nil, "status": out.Status}})
	}
	body := fiber.Map{"public_slug": out.PublicSlug, "status": out.Status}
	if out.CompanySlug != "" {
		body["company_slug"] = out.CompanySlug
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": body})
}

// isHTTPURL reports whether raw is an absolute http(s) URL — the only thing worth
// fetching, and the same gate the contribution flow applies to a pasted link.
func isHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
