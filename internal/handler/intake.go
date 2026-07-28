package handler

import (
	"context"
	"errors"
	"log"

	"github.com/strelov1/freehire/internal/contribution"
	"github.com/strelov1/freehire/internal/credits"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/linkimport"
)

// intakeService is the one sequence every surface that accepts a job link runs — the website's
// contribute form, the Telegram bot, the browser extension, and the CLI. It lives here, rather
// than in each handler, because the outcome must depend on the link and not on where it was
// pasted: the moment two surfaces carry their own copy of "look, then import, then record",
// they drift.
//
// The order is load-bearing at two points:
//
//   - The catalog lookup runs FIRST. The last-resort resolver writes under source "weblink"
//     keyed by the page URL, so importing a posting we already carry from an aggregator would
//     add a second row under a different identity — a duplicate the ingest dedup passes (which
//     work per company+title) would not collapse.
//   - The board is inspected BEFORE the import, because the import writes a posting under that
//     very board. Asking afterwards reports every freshly imported board as already tracked.
type intakeService struct {
	queries      *db.Queries
	contribution *contribution.Service
	imports      *linkimport.Importer
	credits      *credits.Store
}

// intakeOutcome is what happened to one link, in the vocabulary every surface renders from.
// PublicSlug is empty unless a posting is in the catalog; CompanySlug is set only for
// outcomeTracked, where naming the company we already cover is the point.
type intakeOutcome struct {
	Status      string
	PublicSlug  string
	CompanySlug string
	// Board names the company board the link belongs to, when one was recognised. It is set
	// even for outcomeQueued: failing to READ a page says nothing about whether we recognised
	// the board behind it, and a surface that conflates the two tells a user we ignored a
	// company we in fact just accepted (and paid them for).
	Board string
	// Rewarded reports that this intake earned the submitter AI credits — true only for the
	// first submission of a board we do not yet crawl.
	Rewarded bool
}

// Resolve runs the intake for one link and reports what became of it. An import failure is not
// an error: the link is still worth keeping, so it degrades to the triage queue. A returned
// error is a storage failure the caller should surface.
func (s *intakeService) Resolve(ctx context.Context, userID int64, pageURL, surface string) (intakeOutcome, error) {
	slug, err := catalogSlugForURL(ctx, s.queries, pageURL)
	if err != nil {
		return intakeOutcome{}, err
	}
	if slug != "" {
		return intakeOutcome{Status: outcomeFound, PublicSlug: slug}, nil
	}

	intake, err := s.contribution.Inspect(ctx, pageURL)
	if err != nil {
		return intakeOutcome{}, err
	}

	res, imported, err := s.imports.Import(ctx, pageURL)
	if err != nil {
		// A transient fetch or parse failure, not a verdict on the page.
		log.Printf("intake: import %s: %v", pageURL, err)
	}

	companySlug, rewarded, err := s.record(ctx, userID, pageURL, surface, intake)
	if err != nil {
		return intakeOutcome{}, err
	}

	out := intakeOutcome{Board: intake.Board, CompanySlug: companySlug, Rewarded: rewarded}
	switch {
	case !imported:
		out.Status = outcomeQueued
	case intake.Tracked:
		out.Status, out.PublicSlug = outcomeTracked, res.PublicSlug
	default:
		out.Status, out.PublicSlug = outcomeImported, res.PublicSlug
	}
	return out, nil
}

// record persists the inspected link and, for a board we already crawl, returns the company
// slug to name in the answer.
//
// A tracked board is deliberately NOT recorded: it needs no onboarding. Every other recognised
// board is, even when the vacancy itself imported fine, because reading one posting tells us
// nothing about the other twenty on that board. A board someone already contributed is
// recorded too — each row names its own submitter — but only the first earns credits.
//
// None of the duplicate outcomes is an error the caller can act on: the link is known to us
// either way, so they are swallowed rather than surfaced.
func (s *intakeService) record(ctx context.Context, userID int64, pageURL, surface string, intake contribution.Intake) (companySlug string, rewarded bool, err error) {
	res, err := s.contribution.RecordIntake(ctx, contribution.SubmitInput{
		SubmittedBy: userID,
		URL:         pageURL,
		Surface:     surface,
	}, intake)
	switch {
	case err == nil:
		rewarded := res.Rewardable && res.Contribution.Status == contribution.StatusPending
		if rewarded {
			rewardContribution(ctx, s.credits, userID, res.Contribution.ID)
		}
		return "", rewarded, nil
	case errors.Is(err, contribution.ErrBoardAlreadyTracked):
		_, slug, _ := s.contribution.CompanyForBoard(ctx, intake.Source, intake.Board)
		return slug, false, nil
	case errors.Is(err, contribution.ErrBoardAlreadyContributed),
		errors.Is(err, contribution.ErrUnsupportedATS):
		// Already in the triage queue, or not a link any board can be derived from. Neither
		// changes what the caller is told about the page itself.
		return "", false, nil
	default:
		return "", false, err
	}
}
