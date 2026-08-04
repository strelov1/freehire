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
// PublicSlug is empty unless a posting is in the catalog.
//
// Status and CompanySlug answer two different questions, and conflating them is what told a
// contributor of a second Dropbox ATS that "this company is new to us": Status is the fate of
// the BOARD (crawled already, queued for onboarding, or filed for review), CompanySlug names
// the COMPANY when the catalog already carries it — through any source, not just this one.
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

	// The board Inspect just resolved is handed to the import: it is often the only thing that
	// knows a storefront link belongs to a board we already crawl, and without it the posting
	// would be written a second time under its URL.
	res, imported, err := s.imports.Import(ctx, pageURL, linkimport.Board{
		Source: intake.Source,
		Board:  intake.Board,
	})
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
	case imported && res.Deduped:
		// The catalog already carried this vacancy under a crawled source. Answering found HERE
		// rather than at the top of Resolve is what keeps the board: an unrecognised storefront
		// may front a board nobody has contributed yet, and the vacancy being known says nothing
		// about the board being known.
		out.Status, out.PublicSlug = outcomeFound, res.PublicSlug
	case !imported:
		out.Status = outcomeQueued
	case intake.Tracked:
		out.Status, out.PublicSlug = outcomeTracked, res.PublicSlug
	case intake.Recognized:
		out.Status, out.PublicSlug = outcomeImported, res.PublicSlug
	default:
		// Imported, but no board could be named from the URL — a careers site on the company's
		// own domain. Nothing was queued for onboarding, so promising a crawl here would be a
		// promise nothing keeps; the link is in the manual triage queue instead.
		out.Status, out.PublicSlug = outcomeReview, res.PublicSlug
	}
	if out.CompanySlug == "" {
		out.CompanySlug = s.companyAlreadyCarried(ctx, res)
	}
	return out, nil
}

// companyAlreadyCarried names the employer of a just-imported posting when the catalog holds
// more of it than this one row — the answer to "is this company new to us?", which the board
// checks cannot give: a company reached through an ATS we do not recognise, or through a
// second ATS, is one we already carry. Empty when the company is genuinely new, when nothing
// was imported, or on a lookup failure — the posting is saved either way, so the question
// degrades to "we won't claim we know them".
func (s *intakeService) companyAlreadyCarried(ctx context.Context, res linkimport.Result) string {
	if res.CompanySlug == "" {
		return ""
	}
	carried, err := s.queries.CompanyHasOtherJobs(ctx, db.CompanyHasOtherJobsParams{
		CompanySlug: res.CompanySlug,
		Source:      res.Source,
		ExternalID:  res.ExternalID,
	})
	if err != nil {
		log.Printf("intake: company coverage for %s: %v", res.CompanySlug, err)
		return ""
	}
	if !carried {
		return ""
	}
	return res.CompanySlug
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

// renderIntakeOutcome puts one intake outcome into words. A readable vacancy now comes back as
// a link to the posting — before, a Telegram user was only ever told about the board, even when
// we could have handed them the job. Shared across every surface that calls intakeService.Resolve
// and reports the result back to the submitter (Telegram's chat reply, Discord's deferred
// response), so the wording does not drift between them.
//
// emphasize renders the one span of emphasis this wording needs (a board name), in whatever
// markup the calling surface actually understands: Telegram sends parse_mode "HTML", Discord
// interaction responses are Markdown — hardcoding either here would render as literal markup on
// the other surface.
func renderIntakeOutcome(out intakeOutcome, frontendOrigin string, emphasize func(string) string) string {
	switch out.Status {
	case outcomeFound:
		return "👍 We already have this one:\n" + jobURL(frontendOrigin, out.PublicSlug)
	case outcomeTracked:
		return "✅ Added — and we already track this company, so the rest of its roles will follow on the next crawl.\n" +
			jobURL(frontendOrigin, out.PublicSlug) + companyURL(frontendOrigin, out.CompanySlug)
	case outcomeImported:
		if out.CompanySlug != "" {
			return "✅ Added — we already carry this company, and now we'll crawl this board of theirs too.\n" +
				jobURL(frontendOrigin, out.PublicSlug) + companyURL(frontendOrigin, out.CompanySlug)
		}
		return "🎉 Added, and this company is new to us — we'll start crawling its board.\n" + jobURL(frontendOrigin, out.PublicSlug)
	case outcomeReview:
		// Imported, but the page named no board we know how to crawl, so no crawl is promised.
		if out.CompanySlug != "" {
			return "✅ Added — we already carry this company. Its careers site isn't one we can crawl yet, so we'll look at it by hand.\n" +
				jobURL(frontendOrigin, out.PublicSlug) + companyURL(frontendOrigin, out.CompanySlug)
		}
		return "✅ Added. Its careers site isn't one we can crawl yet — we'll check by hand whether we can pull the rest of its jobs.\n" +
			jobURL(frontendOrigin, out.PublicSlug)
	}
	// outcomeQueued. Failing to read the page says nothing about whether we recognised the
	// board behind it, and answering only "couldn't read" would hide a contribution we just
	// accepted — and paid for.
	switch {
	case out.Rewarded:
		return "🎉 We couldn't open that page, but " + emphasize(out.Board) +
			" is a company we don't crawl yet — added to the queue. +1 AI credit!"
	case out.Board != "":
		return "👍 We couldn't open that page, but that company's board is already known to us — nothing to add."
	default:
		return "🤔 We couldn't read that page. We'll check by hand whether we can pull its jobs — if we can, you'll get a credit. Not credited yet."
	}
}

// jobURL renders a posting link, or an empty string when the frontend origin is unset (the
// slug alone would be meaningless to a reader).
func jobURL(frontendOrigin, slug string) string {
	if slug == "" || frontendOrigin == "" {
		return ""
	}
	return frontendOrigin + "/jobs/" + slug
}

// companyURL renders a company link on its own line, so a reply can append it unconditionally.
// Empty under the same rule as jobURL.
func companyURL(frontendOrigin, slug string) string {
	if slug == "" || frontendOrigin == "" {
		return ""
	}
	return "\n" + frontendOrigin + "/companies/" + slug
}
