package handler

import (
	"context"
	"errors"
	"log"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/contribution"
	"github.com/strelov1/freehire/internal/credits"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/linkimport"
)

// contributionHandlers serves the crowdsourced paste-a-link flow: submit a URL →
// detect ATS, dedup by derived identity, record + award a point; list the caller's
// own. The use cases live in contribution.Service.
type contributionHandlers struct {
	contribution *contribution.Service
	credits      *credits.Store
	// queries backs the catalog lookup /jobs/resolve does before importing anything.
	queries *db.Queries
	// imports turns one job page URL into a catalog posting (the engine cmd/resolve-url
	// runs), backing "add this page" from the browser extension.
	imports *linkimport.Importer
}

func newContributionHandlers(contribution *contribution.Service, credits *credits.Store, queries *db.Queries, imports *linkimport.Importer) *contributionHandlers {
	return &contributionHandlers{contribution: contribution, credits: credits, queries: queries, imports: imports}
}

func (h *contributionHandlers) register(api fiber.Router, mw middleware) {
	// Link contributions: any authenticated user pastes a job URL (cookie or API key);
	// a supported, novel link is recorded and earns a point. No moderation queue — the
	// derived-identity dedup and the supported-ATS gate are the only guards. The caller
	// reads their own contributions; the points balance rides on /auth/me.
	api.Post("/me/contributions", mw.key, mw.outboundFetch, h.CreateContribution)
	api.Get("/me/contributions", mw.key, h.ListMyContributions)

	// The extension's "add this page": resolve the page to a catalog posting, importing it
	// when a link-source adapter can read the page and queueing the link for triage when
	// none can. Same limiter as a contribution — both make the server fetch a caller URL.
	api.Post("/jobs/resolve", mw.key, mw.outboundFetch, h.ResolveJob)
}

// contributionRequest is the submit body: just the pasted job URL.
type contributionRequest struct {
	URL string `json:"url"`
}

// contributionResponse is the public shape of a recorded contribution. submitted_by is
// omitted (ownership, internal); source + board name the company board the user discovered.
type contributionResponse struct {
	ID        int64      `json:"id"`
	URL       string     `json:"url"`
	Source    string     `json:"source"`
	Board     string     `json:"board"`
	Status    string     `json:"status"`
	CreatedAt *time.Time `json:"created_at"`
}

// toContributionResponse maps a domain contribution to its wire shape.
func toContributionResponse(c contribution.Contribution) contributionResponse {
	return contributionResponse{
		ID:        c.ID,
		URL:       c.URL,
		Source:    c.Source,
		Board:     c.Board,
		Status:    c.Status,
		CreatedAt: c.CreatedAt,
	}
}

// contributionError maps the contribution sentinels to HTTP statuses: an unsupported link
// is 422 (well-formed request, unprocessable target); a board we already crawl or already
// recorded is 409. Anything else falls through to RenderError as a 500.
func contributionError(err error) error {
	switch {
	case errors.Is(err, contribution.ErrUnsupportedATS):
		return fiber.NewError(fiber.StatusUnprocessableEntity, "link is not from a supported ATS board")
	case errors.Is(err, contribution.ErrBoardAlreadyTracked):
		return fiber.NewError(fiber.StatusConflict, "this board is already in the catalogue")
	case errors.Is(err, contribution.ErrBoardAlreadyContributed):
		return fiber.NewError(fiber.StatusConflict, "this board has already been contributed")
	default:
		return err
	}
}

// CreateContribution records a user-contributed job link and awards AI credits for a novel one.
// Authenticated by cookie or API key. A non-ATS link is 422, a duplicate is 409; a novel
// link returns 201 with the recorded contribution.
func (h *contributionHandlers) CreateContribution(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	var in contributionRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	rec, source, board, err := h.contribution.Submit(c.Context(), userID, in.URL)
	if err != nil {
		// On "already tracked", enrich the 409 with the company we cover so the UI can link to
		// it (and say the exact role will land on the next crawl).
		if errors.Is(err, contribution.ErrBoardAlreadyTracked) {
			body := fiber.Map{"error": "this board is already in the catalogue"}
			if name, slug, ok := h.contribution.CompanyForBoard(c.Context(), source, board); ok {
				body["company_name"] = name
				body["company_slug"] = slug
			}
			return c.Status(fiber.StatusConflict).JSON(body)
		}
		return contributionError(err)
	}
	// Credit is exclusive to a recognized novel board (status pending). An unrecognized link is
	// recorded for manual review (status review) and earns nothing until a maintainer promotes it.
	if rec.Status == contribution.StatusPending {
		rewardContribution(c.Context(), h.credits, userID, rec.ID)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": toContributionResponse(rec)})
}

// rewardContribution grants the AI-credits contribution reward, idempotent by the contribution
// id, for a novel board recorded via any surface (the HTTP submit or the Telegram webhook).
// Best-effort: the contribution is already recorded, so a reward error (or a zero configured
// reward) is logged, not surfaced.
func rewardContribution(ctx context.Context, credits *credits.Store, userID, contributionID int64) {
	if _, err := credits.Reward(ctx, userID, strconv.FormatInt(contributionID, 10)); err != nil {
		log.Printf("credits: contribution reward user=%d contribution=%d: %v", userID, contributionID, err)
	}
}

// ListMyContributions returns the caller's own contributions, newest first. Scoped to the
// authenticated user, so it never reveals another user's.
func (h *contributionHandlers) ListMyContributions(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	rows, err := h.contribution.ListMine(c.Context(), userID)
	if err != nil {
		return err
	}
	out := make([]contributionResponse, len(rows))
	for i, r := range rows {
		out[i] = toContributionResponse(r)
	}
	return c.JSON(fiber.Map{"data": out})
}
