package handler

import (
	"context"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
)

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
// website, the browser extension and the CLI. Authenticated, alongside board
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
func (h *contributionHandlers) ResolveJob(ctx context.Context, req *apigen.ResolveJobRequest) (apigen.ResolveJobRes, error) {
	pageURL := strings.TrimSpace(req.URL.Value)
	if !isHTTPURL(pageURL) {
		return nil, fiber.NewError(fiber.StatusUnprocessableEntity, "not a job page URL")
	}

	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	out, err := h.intake.Resolve(ctx, userID, pageURL, req.Surface.Value)
	if err != nil {
		return nil, err
	}
	if out.Status == outcomeQueued {
		page := apigen.ResolvedJobPage{Status: apigen.ResolvedJobStatus(out.Status)}
		page.PublicSlug.SetToNull()
		return &apigen.ResolveJobAccepted{Data: page}, nil
	}
	page := apigen.ResolvedJobPage{
		PublicSlug:  apigen.NewNilString(out.PublicSlug),
		Status:      apigen.ResolvedJobStatus(out.Status),
		CompanySlug: optString(out.CompanySlug),
	}
	if out.Status == outcomeFound {
		return &apigen.ResolveJobOK{Data: page}, nil
	}
	return &apigen.ResolveJobCreated{Data: page}, nil
}

// isHTTPURL reports whether raw is an absolute http(s) URL — the only thing worth
// fetching, and the same gate the contribution flow applies to a pasted link.
func isHTTPURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
