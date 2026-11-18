package handler

import (
	"context"
	"errors"
	"log"

	"github.com/strelov1/freehire/internal/ingest/contribution"
	"github.com/strelov1/freehire/internal/ingest/linkimport"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/db"
)

// intakeService is the one sequence every surface that accepts a job link runs (contribute
// form, Telegram bot, extension, CLI), so the outcome depends on the link, not where it was pasted.
//
// The order is load-bearing at two points:
//
//   - The catalog lookup runs FIRST. The last-resort resolver writes under source "weblink"
//     keyed by the page URL, so importing a posting we already carry would add a duplicate
//     the per-company+title dedup passes would not collapse.
//   - The board is inspected BEFORE the import, which writes a posting under that board;
//     asking afterwards reports every freshly imported board as already tracked.
type intakeService struct {
	queries      *db.Queries
	contribution *contribution.Service
	imports      *linkimport.Importer
	// postings canonicalises the pasted link before the catalog lookup — see
	// sources.PostingURLResolver. Zero value = the offline rewrite only.
	postings sources.PostingURLResolver
}

// intakeOutcome is what happened to one link, in the vocabulary every surface renders from.
// PublicSlug is empty unless a posting is in the catalog.
//
// Status and CompanySlug answer different questions: Status is the fate of the BOARD
// (crawled, queued for onboarding, or filed for review), CompanySlug names the COMPANY when
// the catalog already carries it through any source.
type intakeOutcome struct {
	Status      string
	PublicSlug  string
	CompanySlug string
	// Board names the company board the link belongs to, when recognised. It is set even
	// for outcomeQueued: failing to READ a page says nothing about recognising its board.
	Board string
	// NovelBoard reports that this intake was the first submission of a board we do not yet
	// crawl. It decides what the submitter is told: "new to us" vs "we already had this".
	NovelBoard bool
}

// Resolve runs the intake for one link and reports what became of it. An import failure is not
// an error: the link is still worth keeping, so it degrades to the triage queue. A returned
// error is a storage failure the caller should surface.
func (s *intakeService) Resolve(ctx context.Context, userID int64, pageURL, surface string) (intakeOutcome, error) {
	slug, err := catalogSlugForURL(ctx, s.queries, s.postings, pageURL)
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

	// The inspected board goes to the import: it is often the only thing that knows a
	// storefront link belongs to a board we already crawl, else the posting is written twice.
	res, imported, err := s.imports.Import(ctx, pageURL, linkimport.Board{
		Source: intake.Source,
		Board:  intake.Board,
	})
	if err != nil {
		// A transient fetch or parse failure, not a verdict on the page.
		log.Printf("intake: import %s: %v", pageURL, err)
	}

	companySlug, novel, err := s.record(ctx, userID, pageURL, surface, intake)
	if err != nil {
		return intakeOutcome{}, err
	}

	out := intakeOutcome{Board: intake.Board, CompanySlug: companySlug, NovelBoard: novel}
	switch {
	case imported && res.Deduped:
		// The catalog already carried this vacancy under a crawled source. Answering here, not
		// at the top of Resolve, keeps the board: a known vacancy says nothing about the
		// board being known.
		out.Status, out.PublicSlug = outcomeFound, res.PublicSlug
	case !imported:
		out.Status = outcomeQueued
	case intake.Tracked:
		out.Status, out.PublicSlug = outcomeTracked, res.PublicSlug
	case intake.Recognized:
		out.Status, out.PublicSlug = outcomeImported, res.PublicSlug
	default:
		// Imported, but no board could be named from the URL (a careers site on the company's
		// own domain). Nothing was queued, so no crawl is promised; the link is in manual triage.
		out.Status, out.PublicSlug = outcomeReview, res.PublicSlug
	}
	if out.CompanySlug == "" {
		out.CompanySlug = s.companyAlreadyCarried(ctx, res)
	}
	return out, nil
}

// companyAlreadyCarried names the employer of a just-imported posting when the catalog holds
// more of it than this one row: the answer to "is this company new to us?", which board
// checks cannot give. Empty when the company is new, nothing was imported, or the lookup
// failed (the posting is saved either way, so it degrades to not claiming we know them).
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
// board is, even when the vacancy imported fine, since one posting says nothing about the
// rest of the board. An already-contributed board is recorded too (each row names its own
// submitter), but only the first counts as novel.
//
// Duplicate outcomes are swallowed: the link is known to us either way.
func (s *intakeService) record(ctx context.Context, userID int64, pageURL, surface string, intake contribution.Intake) (companySlug string, novel bool, err error) {
	res, err := s.contribution.RecordIntake(ctx, contribution.SubmitInput{
		SubmittedBy: userID,
		URL:         pageURL,
		Surface:     surface,
	}, intake)
	switch {
	case err == nil:
		return "", res.Rewardable && res.Contribution.Status == contribution.StatusPending, nil
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

// renderIntakeOutcome puts one intake outcome into words, shared by every chat surface
// that reports an intakeService.Resolve result back to the submitter (Telegram today). A
// readable vacancy comes back as a link to the posting.
//
// emphasize renders the one span of emphasis (a board name) in the calling surface's own
// markup (Telegram sends parse_mode "HTML").
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
	// outcomeQueued. Failing to read the page says nothing about recognising the board, and
	// "couldn't read" alone would hide a contribution we just accepted.
	switch {
	case out.NovelBoard:
		return "🎉 We couldn't open that page, but " + emphasize(out.Board) +
			" is a company we don't crawl yet — added to the queue. Thank you!"
	case out.Board != "":
		return "👍 We couldn't open that page, but that company's board is already known to us — nothing to add."
	default:
		return "🤔 We couldn't read that page. We'll check by hand whether we can pull its jobs."
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
