package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"

	"golang.org/x/net/html"
)

// selfrecruit adapts selfrecruit.ge ("Self.hr"), a Georgian-market multi-tenant ATS. The
// board is the tenant subdomain (e.g. "dressup" for dressup.selfrecruit.ge). The platform
// exposes no JSON API and no schema.org/ld+json markup: the listing is paged by OFFSET
// (`/vacancies/<offset>` in steps of 10 — `/vacancies/0` confirmed identical to the bare
// tenant root), and each posting's title and description sit in fixed, classed DOM
// elements ("vacancy_title_inner", "pub_vac_text_detail") read via the shared html.go DOM
// walkers.
type selfrecruit struct {
	http HTMLGetter
}

// NewSelfRecruit builds the selfrecruit.ge adapter over the given HTML client.
func NewSelfRecruit(c HTMLGetter) Source { return selfrecruit{http: c} }

func (selfrecruit) Provider() string { return "selfrecruit" }

// selfrecruitPageSize is the listing's fixed page size (confirmed live: offsets 0, 10, 20
// each answer a distinct set of up to 10 postings).
const selfrecruitPageSize = 10

// selfrecruitMaxPages bounds the offset walk. The natural stop is a page adding no new
// posting link — confirmed live even past the end, since a page beyond the last one
// redirects back to the tenant root, whose links are already seen — but a misbehaving
// tenant that never repeats a link would otherwise loop forever.
const selfrecruitMaxPages = 100

// fullBoardListing: crawlAllPagedLinks fails the whole Fetch on ANY page failing, not just
// the first, so this marker's "whole listing or fail outright" guarantee holds even
// though the listing is paginated.
func (selfrecruit) fullBoardListing() {}

func (s selfrecruit) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	base, err := url.Parse(fmt.Sprintf("https://%s.selfrecruit.ge/", e.Board))
	if err != nil {
		return nil, fmt.Errorf("selfrecruit: board %q: %w", e.Board, err)
	}

	locs, err := crawlAllPagedLinks(ctx, s.http, selfrecruitMaxPages,
		func(page int) string {
			return fmt.Sprintf("%svacancies/%d", base, (page-1)*selfrecruitPageSize)
		},
		func(root *html.Node) []string {
			return jobLinks(base, root, func(href string) bool { return selfrecruitJobID(href) != "" })
		})
	if err != nil {
		return nil, fmt.Errorf("selfrecruit: listing %s: %w", e.Board, err)
	}

	return fetchDetails(locs, defaultDetailWorkers, func(loc string) (Job, bool) {
		return s.detail(ctx, e, loc)
	}), nil
}

// detail fetches one posting page and extracts its title and description by DOM class,
// returning ok=false when the fetch fails for a reason that states the posting is gone
// (dropped) or an unreadable marker when it doesn't (kept, since this page is the
// adapter's only source for the posting).
func (s selfrecruit) detail(ctx context.Context, e CompanyEntry, loc string) (Job, bool) {
	id := selfrecruitJobID(loc)
	if id == "" {
		return Job{}, false
	}

	root, err := s.http.GetHTML(ctx, loc)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(id, loc, e.Company), true
		}
		return Job{}, false
	}

	// firstByClass returns nil when the page carries neither classed element — textContent/
	// innerHTML would panic on a nil node, and a page that answered but carries no posting
	// markup at all is unreadable, not a posting with an empty title, the same "successful
	// fetch, no usable content" reading detailUnreadable already gives a transport failure.
	titleNode := firstByClass(root, "vacancy_title_inner")
	descNode := firstByClass(root, "pub_vac_text_detail")
	if titleNode == nil || descNode == nil {
		return unreadableDetail(id, loc, e.Company), true
	}
	title := textContent(titleNode)
	description := innerHTML(descNode)

	return Job{
		ExternalID:  id,
		URL:         loc,
		Title:       title,
		Company:     e.Company,
		Description: sanitizeHTML(description),
		Remote:      isRemote(title),
	}, true
}

// selfrecruitJobIDPattern captures a posting's UUID from its detail URL. selfrecruit.ge
// links every posting at the tenant root ("/<uuid>"), so the pattern anchors on a bare
// UUID path with nothing before it — a static page like "/cv" never matches.
var selfrecruitJobIDPattern = regexp.MustCompile(`^/([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})(?:$|[/?#])`)

// selfrecruitJobID extracts the native posting UUID from a detail URL or path, or "" when
// it names no posting.
func selfrecruitJobID(loc string) string {
	p := loc
	if parsed, err := url.Parse(loc); err == nil {
		p = parsed.Path
	}
	return firstSubmatch(selfrecruitJobIDPattern, p)
}
