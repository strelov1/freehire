package handler

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/contribution"
	"github.com/strelov1/freehire/internal/credits"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/linkimport"
	"github.com/strelov1/freehire/internal/sources"
)

// contributionHandlers serves the crowdsourced paste-a-link flow: one intake endpoint every
// surface posts to, plus the caller's own list. The sequence lives in intakeService and the
// use cases in contribution.Service.
type contributionHandlers struct {
	contribution *contribution.Service
	credits      *credits.Store
	// intake is the shared look-import-record sequence /jobs/resolve is the HTTP door onto.
	intake *intakeService
}

func newContributionHandlers(contribution *contribution.Service, credits *credits.Store, queries *db.Queries, imports *linkimport.Importer, postings sources.PostingURLResolver) *contributionHandlers {
	return &contributionHandlers{
		contribution: contribution,
		credits:      credits,
		intake: &intakeService{
			queries:      queries,
			contribution: contribution,
			imports:      imports,
			credits:      credits,
			postings:     postings,
		},
	}
}

func (h *contributionHandlers) register(api fiber.Router, mw middleware) {
	// One intake for every surface: resolve the page to a catalog posting, importing it when
	// anything can read it and recording the board behind it either way. There is deliberately
	// no second "contribute a board" endpoint — a link pasted on the website, in the bot, in
	// the extension or in the CLI must get the same answer, and two endpoints would drift.
	api.Post("/jobs/resolve", mw.key, mw.outboundFetch, h.ResolveJob)

	// The caller reads their own contributions; the credit balance rides on /auth/me.
	api.Get("/me/contributions", mw.key, h.ListMyContributions)
}

// contributionResponse is the public shape of a recorded contribution. submitted_by is
// omitted (ownership, internal); source + board name the company board the user discovered.
type contributionResponse struct {
	ID        int64      `json:"id"`
	URL       string     `json:"url"`
	Source    string     `json:"source"`
	Board     string     `json:"board"`
	Status    string     `json:"status"`
	Surface   string     `json:"surface"`
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
		Surface:   c.Surface,
		CreatedAt: c.CreatedAt,
	}
}

// rewardContribution grants the AI-credits contribution reward, idempotent by the contribution
// id, for a novel board recorded via any surface (the HTTP submit or the Telegram webhook).
// Best-effort: the contribution is already recorded, so a reward error (or a zero configured
// reward) is logged, not surfaced.
func rewardContribution(ctx context.Context, credits *credits.Store, userID, contributionID int64) {
	if credits == nil {
		return // credits unconfigured: the contribution is recorded, there is just nothing to pay
	}
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
