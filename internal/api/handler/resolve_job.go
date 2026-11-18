package handler

import (
	"context"
	"net/url"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
)

// Intake outcomes. They differ only in what the system already knew about the link.
const (
	// outcomeFound — the catalog already carries this vacancy: the URL is stored (no fetch, no
	// write), or the page is a second copy of a posting held under another source.
	outcomeFound = "found"
	// outcomeTracked — we already crawl the board; the posting was imported now.
	outcomeTracked = "tracked"
	// outcomeImported — imported, and the board behind it queued for onboarding.
	outcomeImported = "imported"
	// outcomeReview — imported, but no crawlable board (e.g. a careers site on the company's own
	// domain), so the link went to manual triage and no crawl follows.
	outcomeReview = "review"
	// outcomeQueued — nothing could read the page; the link went to manual triage.
	outcomeQueued = "queued"
)

// ResolveJob is the HTTP door onto the shared intake sequence (see intakeService), used by the
// website, extension and CLI. Authenticated because it makes the server fetch a user-supplied URL.
//
// Outcomes (see the outcome constants): 200 found, 201 tracked, 201 imported, 201 review,
// 202 queued.
//
// company_slug rides along on every 201 whose employer the catalog already carries, independent
// of the board outcome: "new company" must not be read off the status alone.
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
