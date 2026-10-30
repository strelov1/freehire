package linksource

import (
	"context"
	"maps"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/atsboard"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/externalid"
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
	// catalog supplies the entry a crawl of the board is handed; see BoardCatalog.
	catalog BoardCatalog
}

// BoardCatalog answers the board catalogue's entry for (source, board) — the CompanyEntry
// cmd/ingest hands the adapter when it crawls that board. Resolving a link must hand the
// adapter the same entry: most adapters copy CompanyEntry.Company onto every posting, so a
// bare {Provider, Board} builds a posting with no employer, and writing it over the row the
// crawl stored blanks that row's company and company_slug (freehire#3913).
//
// This package reads no database, so the lookup is the caller's; boardcatalog's
// QueriesRepository satisfies it. found=false means the catalogue has no such board — a
// board we do not crawl, so no stored row carries a name to lose.
type BoardCatalog interface {
	Entry(ctx context.Context, source, board string) (e sources.CompanyEntry, found bool, err error)
}

// NewBoardCoverage builds the board-coverage adapter over an ingest registry. A nil or empty
// registry is safe: nothing matches, so the caller falls through to its next resolver. A nil
// catalog fetches every board by its bare (source, board) — only right where nothing is
// written over a crawled row.
func NewBoardCoverage(reg map[string]sources.Source, catalog BoardCatalog) Source {
	return boardCoverage{reg: reg, catalog: catalog}
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
	return ResolveOnBoard(ctx, b.reg, b.catalog, source, board, raw)
}

// ResolveOnBoard fetches one tenant board through its ingest adapter and returns the posting
// raw points at. It is exported for the caller that already knows the board by other means:
// a vanity careers domain, where the host says nothing and only fetching the page reveals the
// embedded ATS (see internal/ingest/boardresolve). That path cannot go through Match — Find picks a
// single adapter and never tries the next — so the fallback is orchestrated a level up, and
// this is the piece it reuses.
//
// The board is read through catalog's entry for it when the catalogue has one (see
// BoardCatalog), so the posting comes back exactly as a crawl of that board builds it — the
// employer's name, and the stored board spelling its id is namespaced with.
//
// ok=false means the board carries no such posting; an error means the board could not be
// fetched (or its catalogue entry read), which the caller must not mistake for the posting
// being absent.
func ResolveOnBoard(ctx context.Context, reg map[string]sources.Source, catalog BoardCatalog, source, board, raw string) (sources.Job, bool, error) {
	adapter, has := reg[source]
	if !has || board == "" {
		return sources.Job{}, false, nil
	}

	entry := sources.CompanyEntry{Provider: source, Board: board}
	if catalog != nil {
		stored, found, err := catalog.Entry(ctx, source, board)
		if err != nil {
			return sources.Job{}, false, err
		}
		if found {
			entry = stored
		}
	}

	jobs, err := fetchForResolve(ctx, adapter, entry, raw)
	if err != nil {
		return sources.Job{}, false, err
	}

	job, found := pickPosting(jobs, raw)
	if !found {
		return sources.Job{}, false, nil
	}
	// Namespace the id by board exactly as the ingest pipeline does, so a later crawl of this
	// board dedups onto the row this import writes instead of adding a second posting.
	job.ExternalID = externalid.Namespace(entry.Board, job.ExternalID)
	return job, true, nil
}

// fetchForResolve fetches a board through the adapter, preferring a HydratingSource's FetchNew
// over Fetch: for such an adapter Fetch hydrates every posting (e.g. workday.Fetch), so resolving
// one pasted link would otherwise trigger a full multi-thousand-posting hydration crawl. FetchNew
// takes a seen predicate; supplying one that reports every posting EXCEPT the one raw points at as
// already seen limits hydration to just that posting — the mirror image of how
// pipeline.Runner.fetchBoard uses the same predicate to skip postings the store already has. The
// target's external id is guessed from raw's path segments and query values (postingRefs), the
// same heuristic pickPosting's id fallback uses to identify a posting once the board is in hand.
func fetchForResolve(ctx context.Context, adapter sources.Source, e sources.CompanyEntry, raw string) ([]sources.Job, error) {
	hs, hydrating := adapter.(sources.HydratingSource)
	if !hydrating {
		return adapter.Fetch(ctx, e)
	}
	want := postingRefs(raw)
	seen := func(externalID string) bool {
		return !slices.ContainsFunc(want, func(seg string) bool { return segmentNamesID(seg, externalID) })
	}
	return hs.FetchNew(ctx, e, seen)
}

// segmentNamesID reports whether a URL path segment names the posting id: the whole segment, or
// the id followed by a readable slug — SmartRecruiters' "<id>-<title-slug>", where the id is
// never a segment of its own.
func segmentNamesID(seg, id string) bool {
	return id != "" && (sameRef(seg, id) || strings.HasPrefix(seg, id+"-"))
}

// guidShape is a GUID in its canonical 8-4-4-4-12 hex spelling.
var guidShape = regexp.MustCompile(`^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}$`)

// sameRef reports whether two posting references are the same value. A GUID compares without
// regard to case, the one spelling difference that never changes what it names: ClearCompany's
// widget works with upper-case posting ids while the hrmdirect adapter stores and links them
// lower-case, so a link copied before the widget rewrote the address bar names its posting in
// the other case. Anything else compares exactly — a short code may well be case-significant.
func sameRef(a, b string) bool {
	return a == b || (guidShape.MatchString(a) && strings.EqualFold(a, b))
}

// pickPosting finds the posting a link refers to among a board's postings. It tries the URL
// first and the posting id second: a board often serves one posting under several URLs (a
// locale prefix, a slug that changed after the title was edited), but the id in the last path
// segment survives all of those.
//
// The URL comparison ignores the query, except that a parameter both URLs carry must agree. A
// pasted link's tracking parameters are noise, but a platform that names the posting ONLY in the
// query — eRecruiter's Offer.aspx?oid=<id>, the same path for every offer — is identified by
// exactly that parameter, and ignoring it handed back whichever offer came first. For the same
// reason the URL pass requires the link to carry such a parameter at all (sameURL).
//
// An Unreadable marker is never a match. It carries a posting's identity WITHOUT its detail,
// so it answers both predicates — and it is not the safe near-miss SeenRefresh is here, it is
// the opposite: the marker exists precisely because that posting could not be read, which for
// a pasted link is the target every time. Returning one would hand the caller a posting with
// no title, which job.New rejects as an invalid draft, which reaches the reader as a 500 on a
// URL whose honest answer is "we could not read a vacancy from that url".
func pickPosting(jobs []sources.Job, raw string) (sources.Job, bool) {
	for _, j := range jobs {
		if j.Unreadable {
			continue
		}
		if sameURL(raw, j.URL) {
			return j, true
		}
	}
	for _, seg := range postingRefs(raw) {
		for _, j := range jobs {
			if j.Unreadable || !queryAgrees(raw, j.URL) {
				continue
			}
			if segmentNamesID(seg, j.ExternalID) {
				return j, true
			}
		}
	}
	return sources.Job{}, false
}

// sameURL reports whether a link and a posting's URL address the same posting: equal once
// normalized (normalizeURL), and the link carrying every non-tracking query parameter the
// posting's URL does, with the same value. A parameter the link lacks makes them different
// addresses: ClearCompany serves every posting at its career page's own path under
// ?jobId=<id>, and without that rule the career page itself matched the board's first posting.
func sameURL(raw, posting string) bool {
	return normalizeURL(raw) == normalizeURL(posting) && queryMatches(raw, posting, true)
}

// queryAgrees reports whether no query parameter present in both URLs carries different values.
// It vetoes the id match too: a query value on a link (a tracking ref, a sibling id) can equal
// some other posting's id, and that posting's own URL then names a different oid than the link.
func queryAgrees(raw, posting string) bool {
	return queryMatches(raw, posting, false)
}

// queryMatches holds the posting URL's non-tracking query parameters against the link's: a
// parameter both carry must agree, and with linkMustCarry one the link lacks fails as well.
func queryMatches(raw, posting string, linkMustCarry bool) bool {
	a, errA := url.Parse(strings.TrimSpace(raw))
	b, errB := url.Parse(strings.TrimSpace(posting))
	if errA != nil || errB != nil {
		return true // nothing to disagree with
	}
	qa, qb := a.Query(), b.Query()
	for k, v := range qb {
		if trackingParam(k) {
			continue
		}
		w, ok := qa[k]
		if (!ok && linkMustCarry) || (ok && !slices.EqualFunc(v, w, sameRef)) {
			return false
		}
	}
	return true
}

// trackingParams are the query parameters that say how a link travelled rather than what it
// points at, besides the utm_ family trackingParam matches by prefix. A stored posting URL can
// carry them too (micro1 stores its utm_ referral values), so they must never veto a match.
var trackingParams = map[string]bool{
	"gclid": true, "fbclid": true, "msclkid": true, "trk": true, "gh_src": true,
	"ref": true, "src": true, "source": true,
}

func trackingParam(k string) bool {
	k = strings.ToLower(k)
	return strings.HasPrefix(k, "utm_") || trackingParams[k]
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

// postingRefs returns what in a link may name the posting's id, in the order to try them: the
// path segments last-first, then the query values. Most platforms put the id in the path; a few
// name it only in the query (eRecruiter's oid), and a query value that names no posting on the
// board matches nothing.
func postingRefs(raw string) []string {
	refs := pathSegmentsFromEnd(raw)
	u, err := url.Parse(raw)
	if err != nil {
		return refs
	}
	q := u.Query()
	for _, k := range slices.Sorted(maps.Keys(q)) {
		if !trackingParam(k) {
			refs = append(refs, q[k]...)
		}
	}
	return refs
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
