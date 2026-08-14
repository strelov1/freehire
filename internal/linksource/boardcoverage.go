package linksource

import (
	"context"
	"net/url"
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/atsboard"
	"github.com/strelov1/freehire/internal/sources"
)

// boardCoverage resolves a vacancy on any multi-tenant ATS the board recogniser knows, by
// reusing the ingest adapter that already crawls that platform: derive (source, board) from
// the URL, fetch that one tenant's board, and pick the posting the link points at.
//
// It exists because the two registries were wildly out of step — eight host-scoped link
// adapters against ~50 recognised ATS hosts and ~166 ingest adapters — so most pasted links
// could not be read even though the platform behind them is one freehire crawls in full.
// Writing a single-page adapter per platform would be ~50 near-identical files; this is one,
// and its coverage grows with the recogniser table rather than with hand-written code.
//
// The cost is honest: answering about one vacancy fetches a whole tenant board. That is
// acceptable here because it sits BEHIND the host-scoped adapters (a platform with a cheap
// per-job API keeps using it), the boards are one company's, and the endpoint is rate limited
// per user.
type boardCoverage struct {
	// reg is the ingest registry, keyed by provider — the same map cmd/ingest crawls with.
	reg map[string]sources.Source
}

// NewBoardCoverage builds the board-coverage adapter over an ingest registry. A nil or empty
// registry is safe: nothing matches, so the caller falls through to its next resolver.
func NewBoardCoverage(reg map[string]sources.Source) Source {
	return boardCoverage{reg: reg}
}

// Source names the adapter, not the platform: unlike every other link source, this one serves
// many platforms, so the identity a resolved job is stored under depends on the link. That is
// what SourceFor answers, and ResolveLinks prefers it. This value exists only so the adapter
// satisfies Source and is identifiable in logs; it is never written to jobs.source.
func (boardCoverage) Source() string { return "boardcoverage" }

// SourceFor reports the platform this particular link belongs to — the value that becomes
// jobs.source, so the imported posting carries the same identity the ingest crawl of that
// board would give it.
func (boardCoverage) SourceFor(u *url.URL) string {
	if u == nil {
		return ""
	}
	source, _, _, ok := atsboard.Recognize(u.String())
	if !ok {
		return ""
	}
	return source
}

// Match accepts a link whose host resolves to a board on a platform we have an ingest adapter
// for. Both halves matter: an unrecognised host has no board to fetch, and a recognised one
// with no adapter cannot be fetched, so in either case a later resolver should get the link.
// It is deliberately network-free — Match must stay a pure predicate.
func (b boardCoverage) Match(u *url.URL) bool {
	if u == nil {
		return false
	}
	source, board, _, ok := atsboard.Recognize(u.String())
	if !ok || board == "" {
		return false
	}
	_, has := b.reg[source]
	return has
}

// Resolve fetches the tenant board the link belongs to and returns the posting it points at.
// ok=false means the board was read but carries no such posting (a closed or delisted
// vacancy) — the caller should record the link rather than retry. A non-nil error is a board
// fetch failure, which is worth retrying and must not be mistaken for "no such vacancy".
func (b boardCoverage) Resolve(ctx context.Context, raw string) (sources.Job, bool, error) {
	source, board, _, ok := atsboard.Recognize(raw)
	if !ok || board == "" {
		return sources.Job{}, false, nil
	}
	return ResolveOnBoard(ctx, b.reg, source, board, raw)
}

// ResolveOnBoard fetches one tenant board through its ingest adapter and returns the posting
// raw points at. It is exported for the caller that already knows the board by other means:
// a vanity careers domain, where the host says nothing and only fetching the page reveals the
// embedded ATS (see internal/boardresolve). That path cannot go through Match — Find picks a
// single adapter and never tries the next — so the fallback is orchestrated a level up, and
// this is the piece it reuses.
//
// ok=false means the board carries no such posting; an error means the board could not be
// fetched, which the caller must not mistake for the posting being absent.
func ResolveOnBoard(ctx context.Context, reg map[string]sources.Source, source, board, raw string) (sources.Job, bool, error) {
	adapter, has := reg[source]
	if !has || board == "" {
		return sources.Job{}, false, nil
	}

	jobs, err := fetchForResolve(ctx, adapter, sources.CompanyEntry{Provider: source, Board: board}, raw)
	if err != nil {
		return sources.Job{}, false, err
	}

	job, found := pickPosting(jobs, raw)
	if !found {
		return sources.Job{}, false, nil
	}
	// Namespace the id by board exactly as the ingest pipeline does, so a later crawl of this
	// board dedups onto the row this import writes instead of adding a second posting.
	job.ExternalID = sources.NamespaceExternalID(board, job.ExternalID)
	return job, true, nil
}

// fetchForResolve fetches a board through the adapter, preferring a HydratingSource's FetchNew
// over Fetch: for such an adapter Fetch hydrates every posting (e.g. workday.Fetch), so resolving
// one pasted link would otherwise trigger a full multi-thousand-posting hydration crawl. FetchNew
// takes a seen predicate; supplying one that reports every posting EXCEPT the one raw points at as
// already seen limits hydration to just that posting — the mirror image of how
// pipeline.Runner.fetchBoard uses the same predicate to skip postings the store already has. The
// target's external id is guessed from raw's path segments, the same heuristic pickPosting's id
// fallback uses to identify a posting once the board is in hand.
func fetchForResolve(ctx context.Context, adapter sources.Source, e sources.CompanyEntry, raw string) ([]sources.Job, error) {
	hs, hydrating := adapter.(sources.HydratingSource)
	if !hydrating {
		return adapter.Fetch(ctx, e)
	}
	want := pathSegmentsFromEnd(raw)
	seen := func(externalID string) bool {
		return !slices.Contains(want, externalID)
	}
	return hs.FetchNew(ctx, e, seen)
}

// pickPosting finds the posting a link refers to among a board's postings. It tries the URL
// first and the posting id second: a board often serves one posting under several URLs (a
// locale prefix, a slug that changed after the title was edited), but the id in the last path
// segment survives all of those.
func pickPosting(jobs []sources.Job, raw string) (sources.Job, bool) {
	want := normalizeURL(raw)
	for _, j := range jobs {
		if normalizeURL(j.URL) == want {
			return j, true
		}
	}
	for _, seg := range pathSegmentsFromEnd(raw) {
		for _, j := range jobs {
			if j.ExternalID != "" && j.ExternalID == seg {
				return j, true
			}
		}
	}
	return sources.Job{}, false
}

// normalizeURL reduces a posting URL to what identifies it: scheme-less, lowercased host with
// any "www." dropped, no query, no fragment, no trailing slash. Two URLs for one posting
// differing only in tracking parameters or scheme compare equal.
func normalizeURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return ""
	}
	host := strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
	return host + strings.TrimSuffix(u.Path, "/")
}

// pathSegmentsFromEnd returns a URL's path segments last-first — the order to look for a
// posting id in. Most ATS URL shapes end in the id, but a storefront over the board appends a
// readable slug after it (…/jobs/<id>/<title>/), so the tail alone is not enough. Matching is
// against one board's own posting ids, a set small enough that scanning inward costs no
// precision.
func pathSegmentsFromEnd(raw string) []string {
	u, err := url.Parse(raw)
	if err != nil {
		return nil
	}
	p := strings.Trim(u.Path, "/")
	if p == "" {
		return nil
	}
	segs := strings.Split(p, "/")
	slices.Reverse(segs)
	return segs
}
