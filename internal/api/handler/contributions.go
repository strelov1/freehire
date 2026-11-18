package handler

import (
	"context"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
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

func (h *contributionHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	// One intake for every surface: resolve the page to a catalog posting, importing it when
	// anything can read it and recording the board behind it either way. There is deliberately
	// no second "contribute a board" endpoint — a link pasted on the website, in the bot, in
	// the extension or in the CLI must get the same answer, and two endpoints would drift.
	api.Post("/jobs/resolve", mw.key, contract)

	// The caller reads their own contributions; the credit balance rides on /auth/me.
	api.Get("/me/contributions", mw.key, contract)
}

// toContributionResponse maps a domain contribution to its wire shape.
func contributionOut(c contribution.Contribution) apigen.Contribution {
	out := apigen.Contribution{
		ID:        c.ID,
		URL:       c.URL,
		Source:    c.Source,
		Board:     c.Board,
		Status:    apigen.ContributionStatus(c.Status),
		Surface:   c.Surface,
		CreatedAt: nilDateTime(c.CreatedAt),
	}
	return out
}

// Contributing a board earns nothing for now: a novel board is still recorded and attributed, but the
// submitter is not paid. Any future reward must stay idempotent per contribution id.

// ListMyContributions returns the caller's own contributions, newest first. Scoped to the
// authenticated user, so it never reveals another user's.
func (h *contributionHandlers) ListMyContributions(ctx context.Context) (*apigen.ListMyContributionsOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := h.contribution.ListMine(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]apigen.Contribution, len(rows))
	for i, r := range rows {
		out[i] = contributionOut(r)
	}
	return &apigen.ListMyContributionsOK{Data: out}, nil
}
