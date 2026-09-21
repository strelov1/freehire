package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// peopleforce adapts PeopleForce career sites. The board is the tenant subdomain (e.g.
// "gigacloud"), forming the host "<board>.peopleforce.io". The tenant's listing is
// server-rendered HTML paginated via ?page=N, each job card linking to a
// /careers/v/<id>-<slug> detail page. The detail page carries no schema.org JobPosting, so
// its fields are read from the DOM: the description is the Bootstrap col-lg-8 column and a
// <dl> sidebar holds Work type / Location. The title comes from the listing anchor (the
// detail <h1> is the generic "Work at <Company>").
type peopleforce struct {
	http HTMLGetter
}

// NewPeopleForce builds the PeopleForce adapter over the given HTML client.
func NewPeopleForce(c HTMLGetter) Source { return peopleforce{http: c} }

func (peopleforce) Provider() string { return "peopleforce" }

// fullBoardListing: Fetch proves completeness by paginating until the listing's own nav stops
// naming a page after this one, and treats a page failure or reaching peopleforceMaxPages as a
// hard Fetch failure. See the fullBoardListing interface (source.go) for the bar.
func (peopleforce) fullBoardListing() {}

// peopleforceMaxPages caps the ?page=N walk so a listing whose nav never stops naming a next
// page cannot loop forever (the largest boards seen are a few pages; this is ample headroom).
//
// It is a backstop, not the end signal, and the difference is what freehire#3046 was about:
// while the walk waited for an EMPTY page — which this source does not serve, clamping an
// out-of-range ?page=N to the last real one instead — this cap was reached on every crawl of
// 85 of the provider's 93 boards, and reaching it discards the whole board.
const peopleforceMaxPages = 100

// peopleforceDetailWorkers throttles the per-board detail fan-out below the shared
// defaultDetailWorkers (8): peopleforce.io rate-limits by request volume (429), and a wide
// burst across 61 boards poisons the egress IP — starving later boards whose listing then
// 429s. A narrow pool keeps each board's burst small; the crawl also egresses through the
// proxy (see proxiedProviders) so the volume never lands on the prod IP.
const peopleforceDetailWorkers = 3

// peopleforceListing is one job card read from a listing page: its canonical detail URL and
// the title from the anchor text (the detail page's own <h1> is not the job title).
type peopleforceListing struct {
	URL   string
	Title string
}

func (s peopleforce) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	base, err := url.Parse(fmt.Sprintf("https://%s.peopleforce.io/careers", e.Board))
	if err != nil {
		return nil, fmt.Errorf("peopleforce: board %q: %w", e.Board, err)
	}

	// Page through the listing, collecting each card's detail URL + title until the page's own
	// pagination nav stops naming a page after this one — the proof of completeness this walk
	// has. Any page failure, or reaching peopleforceMaxPages without that proof, is a hard Fetch
	// failure rather than a partial success. See the fullBoardListing interface (source.go) for
	// the bar.
	seen := map[string]struct{}{}
	var cards []peopleforceListing
	done := false
	for page := 1; page <= peopleforceMaxPages; page++ {
		listURL := fmt.Sprintf("%s?page=%d", base, page)
		root, err := s.http.GetHTML(ctx, listURL)
		if err != nil {
			return nil, fmt.Errorf("peopleforce: listing %s page %d: %w", e.Board, page, err)
		}
		pageCards := peopleforceListings(base, root)
		for _, c := range pageCards {
			if _, ok := seen[c.URL]; !ok {
				seen[c.URL] = struct{}{}
				cards = append(cards, c)
			}
		}
		// The source's own pagination nav says whether there is another page, and it is the only
		// thing that can: an out-of-range ?page=N is CLAMPED to the last real page rather than
		// answered empty, so waiting for an empty page waits forever. Measured 2026-09-21 against
		// akvelon.peopleforce.io — 30 postings on 3 pages, and pages 4, 5, 20 and 5000 each return
		// page 3 byte for byte. A board that fits on one page renders no nav at all, which is the
		// same answer.
		//
		// The old rule was "stop on a page with no cards", chosen over "stop on a page that adds
		// nothing new" because a duplicate-only page is not proof of the end. That reasoning is
		// still correct, and is why the fix is not to relax it: the nav is a direct statement from
		// the source, so a duplicate-only page mid-listing is still walked through.
		//
		// Kept alongside it: a genuinely empty page also ends the walk. The live site does not
		// serve one, but a board whose every posting closed between two crawls plausibly would,
		// and there is nothing left to page through either way.
		if len(pageCards) == 0 || !peopleforceHasPageAfter(root, page) {
			done = true
			break
		}
	}
	if !done {
		return nil, fmt.Errorf("peopleforce: listing %s: reached the %d-page safety ceiling without finding the board's end", e.Board, peopleforceMaxPages)
	}

	// Each posting's description and structured fields come from its own detail fetch, fanned
	// out under the shared bounded pool.
	return fetchDetails(cards, peopleforceDetailWorkers, func(c peopleforceListing) (Job, bool) {
		return s.detail(ctx, e, c)
	}), nil
}

// detail fetches one job's detail page and maps it to a Job. A URL carrying no native id is a
// plain drop (ok=false) — it could never have been stored, so no close can reach it. A page the
// platform answers 404/410 for is dropped too: that is the platform's own evidence the posting is
// gone. Everything else the fetch could fail with comes back as an unreadableDetail marker
// instead, since this crawl is now trusted (fullBoardListing) for the sweep's board-scoped close,
// and a plain drop here would be indistinguishable from the posting having been taken down.
func (s peopleforce) detail(ctx context.Context, e CompanyEntry, c peopleforceListing) (Job, bool) {
	id := peopleforceJobID(c.URL)
	if id == "" {
		return Job{}, false
	}
	root, err := s.http.GetHTML(ctx, c.URL)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(id, c.URL, e.Company), true
		}
		return Job{}, false
	}

	fields := peopleforceDefList(root)
	location := fields["Location"]
	description := ""
	col := firstByClass(root, "col-lg-8")
	if col == nil {
		// PeopleForce is mid-rollout of a Tailwind-based theme: a tenant on the new theme
		// renders the same column prefixed "tw-", so a board can carry a mix of both across
		// its own postings depending on when each was last touched on their end.
		col = firstByClass(root, "tw-col-lg-8")
	}
	if col != nil {
		description = sanitizeHTML(innerHTML(col))
	}

	return Job{
		ExternalID:     id,
		URL:            c.URL,
		Title:          c.Title,
		Company:        e.Company,
		Location:       location,
		Description:    description,
		Remote:         isRemote(location),
		EmploymentType: peopleforceEmploymentType(fields["Work type"]),
	}, true
}

// peopleforceJobIDPattern captures the numeric job id from a /careers/v/<id>-<slug> URL.
var peopleforceJobIDPattern = regexp.MustCompile(`/careers/v/(\d+)`)

// peopleforceJobID extracts the native numeric job id from a detail URL, or "" when the URL
// is not a job posting.
func peopleforceJobID(loc string) string {
	return firstSubmatch(peopleforceJobIDPattern, loc)
}

// peopleforcePageHref matches the page number in a pagination link, whether it arrives
// absolute-path ("/careers?page=4") or bare ("?page=4").
var peopleforcePageHref = regexp.MustCompile(`[?&]page=(\d+)`)

// peopleforceHasPageAfter reports whether the listing's own pagination nav links to any page
// numbered above current — the source's statement that there is more to walk.
//
// This is the ONLY end-of-listing proof the source gives, because it does not serve an empty
// page: an out-of-range ?page=N is clamped to the last real one. A board with a single page
// renders no nav at all (pagy draws one only when there is more than one page), so "no link
// above current" and "no nav" are the same answer and both mean stop.
//
// It reads every page link rather than looking for a "next" control: pagy labels that control
// per-locale and the tenants render several themes, while the numbered links are structural.
func peopleforceHasPageAfter(root *html.Node, current int) bool {
	found := false
	walk(root, func(n *html.Node) bool {
		if found || n.Type != html.ElementNode || n.Data != "a" {
			return !found
		}
		m := peopleforcePageHref.FindStringSubmatch(Attr(n, "href"))
		if m == nil {
			return true
		}
		if p, err := strconv.Atoi(m[1]); err == nil && p > current {
			found = true
			return false
		}
		return true
	})
	return found
}

// peopleforceListings returns each job card's absolute detail URL and title from a listing
// page, resolved against base, deduplicated by URL in first-seen order.
func peopleforceListings(base *url.URL, root *html.Node) []peopleforceListing {
	var out []peopleforceListing
	seen := map[string]struct{}{}
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "a" {
			return true
		}
		href := Attr(n, "href")
		if peopleforceJobID(href) == "" {
			return true
		}
		ref, err := url.Parse(href)
		if err != nil {
			return true
		}
		abs := base.ResolveReference(ref).String()
		if _, ok := seen[abs]; ok {
			return true
		}
		seen[abs] = struct{}{}
		out = append(out, peopleforceListing{URL: abs, Title: textContent(n)})
		return true
	})
	return out
}

// peopleforceDefList reads the detail page's <dl> sidebar into a label→value map, pairing each
// <dd> with its preceding <dt> (e.g. "Work type"→"Full-time", "Location"→"Kyiv").
func peopleforceDefList(root *html.Node) map[string]string {
	out := map[string]string{}
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "dl" {
			return true
		}
		var label string
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type != html.ElementNode {
				continue
			}
			switch c.Data {
			case "dt":
				label = textContent(c)
			case "dd":
				if label != "" {
					out[label] = textContent(c)
					label = ""
				}
			}
		}
		return false // a dl is a leaf for our purposes; do not descend further
	})
	return out
}

// peopleforceEmploymentType maps PeopleForce's "Work type" label onto the freehire vocabulary,
// returning "" for an unknown or absent value so the description parser decides.
func peopleforceEmploymentType(workType string) string {
	switch strings.ToLower(strings.TrimSpace(workType)) {
	case "full-time", "full time":
		return "full_time"
	case "part-time", "part time":
		return "part_time"
	case "contract", "freelance", "temporary":
		return "contract"
	case "internship", "intern", "trainee":
		return "internship"
	}
	return ""
}
