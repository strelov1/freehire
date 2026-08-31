package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/ingest/contribution"
	"github.com/strelov1/freehire/internal/ingest/linkimport"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
)

// contributionHandlers serves the crowdsourced paste-a-link flow: one intake endpoint every
// surface posts to, plus the caller's own list. The sequence lives in intakeService and the
// use cases in contribution.Service.
type contributionHandlers struct {
	contribution *contribution.Service
	// intake is the shared look-import-record sequence /jobs/resolve is the HTTP door onto.
	intake *intakeService
}

func newContributionHandlers(contribution *contribution.Service, queries *db.Queries, imports *linkimport.Importer, postings sources.PostingURLResolver) *contributionHandlers {
	return &contributionHandlers{
		contribution: contribution,
		intake: &intakeService{
			queries:      queries,
			contribution: contribution,
			imports:      imports,
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

// Contributing a board earns nothing at the moment. The reward used to be one AI credit,
// and the currency it was paid in no longer exists — a daily allowance is not something a
// one-off act can top up, and banking a day's worth of anything for later is the shape the
// old balance had and the reason it was withdrawn.
//
// The reward returns in the add-invites change as days of Pro, which a one-off act CAN
// grant. Until then a novel board is still recorded and still attributed; the submitter is
// simply not paid for it. The idempotency rule that protected the old reward — at most one
// per contribution id — carries over unchanged when it does.

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
