package sources

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// staffy adapts Staffy (jobs.wearestaffy.com), a single recruiting/staffing agency's own
// job board — boardless, like lumenalta, since the domain names exactly one company. Both
// the listing and every detail page are fully server-rendered static HTML with no XHR or
// client-side data fetch at all (confirmed live via a full network capture) — the simplest
// shape of any adapter in this initiative, needing only the shared DOM-walking helpers.
type staffy struct {
	http HTMLGetter
}

// NewStaffy builds the Staffy adapter over the given HTML client.
func NewStaffy(c HTMLGetter) Source { return staffy{http: c} }

func (staffy) Provider() string { return "staffy" }

// staffy is single-company, so its config entry carries no board.
func (staffy) boardless() {}

const staffyListingURL = "https://jobs.wearestaffy.com/vacantes"

// fullBoardListing: the listing's own declared total ("NN activas") is verified against
// the number of distinct posting links found, the same "prove wholeness against a
// declared count" posture scalis/recrutei/pyjamahr already established. A mismatch, a
// listing fetch failure, or finding no declared total at all fails the whole Fetch. A
// detail-fetch failure for one posting becomes an Unreadable marker rather than a silent
// drop, the same contract every other listing-then-detail adapter in this package gives.
func (staffy) fullBoardListing() {}

func (s staffy) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	locs, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	return fetchDetails(locs, defaultDetailWorkers, func(loc string) (Job, bool) {
		return s.detail(ctx, e, loc)
	}), nil
}

// list fetches the listing page and verifies its declared total against the number of
// distinct posting links found.
func (s staffy) list(ctx context.Context) ([]string, error) {
	root, err := s.http.GetHTML(ctx, staffyListingURL)
	if err != nil {
		return nil, fmt.Errorf("staffy: list: %w", err)
	}
	base, _ := url.Parse(staffyListingURL)
	locs := jobLinks(base, root, func(href string) bool { return strings.Contains(href, "/positions/") })

	declared, ok := staffyDeclaredTotal(root)
	if !ok {
		return nil, fmt.Errorf("staffy: list: no declared total found")
	}
	if len(locs) != declared {
		return nil, fmt.Errorf("staffy: list: declared total %d disagrees with %d links found", declared, len(locs))
	}
	return locs, nil
}

// staffyDeclaredTotal reads the listing page's own "NN activas" count.
func staffyDeclaredTotal(root *html.Node) (int, bool) {
	n := firstByClass(root, "section-kicker-title")
	if n == nil {
		return 0, false
	}
	text := textContent(n)
	digits := strings.TrimSpace(strings.SplitN(text, " ", 2)[0])
	total, err := strconv.Atoi(digits)
	if err != nil {
		return 0, false
	}
	return total, true
}

// detail fetches one posting's page and maps it to a Job. A transport failure that
// states nothing about the posting yields an Unreadable marker, since the detail page is
// this adapter's only source for everything but the URL itself.
func (s staffy) detail(ctx context.Context, e CompanyEntry, loc string) (Job, bool) {
	id := staffyJobID(loc)
	root, err := s.http.GetHTML(ctx, loc)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(id, loc, e.Company), true
		}
		return Job{}, false
	}

	titleNode := firstByClass(root, "position-content")
	if titleNode == nil {
		return unreadableDetail(id, loc, e.Company), true
	}
	// A missing metadata block means the page's markup has drifted from the shape this
	// adapter depends on: staffyDescriptionHTML's direct-child walk needs it to know
	// where the prose sections begin, and without it every structured field is lost too
	// — mark the whole posting Unreadable rather than silently ship an empty/mis-mapped
	// job, the same "page read successfully but doesn't have what we need" posture every
	// other DOM-scraping adapter in this package already gives a missing element.
	spans := staffyMetadataSpans(root)
	if len(spans) < 3 {
		return unreadableDetail(id, loc, e.Company), true
	}
	title := textContent(firstH1(titleNode))
	location, workText, seniorityText := spans[0], spans[1], spans[2]
	workMode := staffyWorkMode(workText)

	return Job{
		ExternalID:  id,
		URL:         loc,
		Title:       strings.TrimSpace(title),
		Company:     e.Company,
		Location:    location,
		Description: sanitizeHTML(staffyDescriptionHTML(titleNode)),
		Remote:      workMode == "remote",
		WorkMode:    workMode,
		Seniority:   staffySeniority(seniorityText),
	}, true
}

// firstH1 returns the first <h1> descendant of n, or n itself if none is found (defensive
// fallback, never observed live).
func firstH1(n *html.Node) *html.Node {
	var found *html.Node
	walk(n, func(c *html.Node) bool {
		if found != nil {
			return false
		}
		if c.Type == html.ElementNode && c.Data == "h1" {
			found = c
			return false
		}
		return true
	})
	if found == nil {
		return n
	}
	return found
}

// staffyMetadataSpans returns the text of every <span> inside the page's .metadata block,
// in document order — confirmed live to always be exactly [location, work arrangement,
// seniority].
func staffyMetadataSpans(root *html.Node) []string {
	block := firstByClass(root, "metadata")
	if block == nil {
		return nil
	}
	var out []string
	walk(block, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "span" {
			out = append(out, strings.TrimSpace(textContent(n)))
		}
		return true
	})
	return out
}

// staffyDescriptionHTML concatenates every <h2>-delimited prose section below the
// position's metadata into one HTML blob, sanitized by the caller. Walks only the
// article's DIRECT children (the confirmed-live flat structure) rather than the whole
// subtree, so the leading "New opportunity" eyebrow paragraph above the metadata block
// is never mistaken for part of the description.
func staffyDescriptionHTML(article *html.Node) string {
	meta := firstByClass(article, "metadata")
	var b strings.Builder
	afterMeta := meta == nil // defensive: if no metadata block found, collect everything
	for n := article.FirstChild; n != nil; n = n.NextSibling {
		if n == meta {
			afterMeta = true
			continue
		}
		if !afterMeta || n.Type != html.ElementNode {
			continue
		}
		switch n.Data {
		case "h2":
			b.WriteString("<h2>" + strings.TrimSpace(textContent(n)) + "</h2>")
		case "p":
			b.WriteString("<p>" + innerHTML(n) + "</p>")
		case "ul":
			b.WriteString("<ul>" + innerHTML(n) + "</ul>")
		}
	}
	return b.String()
}

// staffyJobID extracts the posting slug from a detail page URL (the last path segment).
func staffyJobID(loc string) string {
	u, err := url.Parse(loc)
	if err != nil {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	return parts[len(parts)-1]
}

// staffySeniority maps the detail page's third metadata span onto vocab.SeniorityValues,
// using the standard Argentina/LatAm-market IT abbreviations (Jr/Ssr/Sr for
// Junior/Semi-Senior/Senior). Confirmed live across a 30+-posting sample: "Junior", "Jr",
// "Semi senior", "Semi Senior" (capitalization varies by posting), "Ssr", "Senior", "Sr",
// and "Staff". A compound/ambiguous label a posting itself doesn't commit to one level
// (e.g. "Senior/ Semi senior", also confirmed live) maps to "" rather than a guess, the
// same as any other unrecognized label.
func staffySeniority(label string) string {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "junior", "jr":
		return "junior"
	case "semi senior", "ssr":
		return "middle"
	case "senior", "sr":
		return "senior"
	case "staff":
		return "staff"
	default:
		return ""
	}
}

// staffyWorkMode maps the detail page's second metadata span — free Spanish text, not a
// clean enum (e.g. "Hibrido - 2 veces por semana", "Hibrido (Puerto Madero)") — via a
// local prefix check rather than the shared workplaceTypeMode helper, which only
// recognizes English spellings.
func staffyWorkMode(text string) string {
	t := strings.ToLower(strings.TrimSpace(text))
	switch {
	case strings.HasPrefix(t, "remoto"):
		return "remote"
	case strings.HasPrefix(t, "hibrido"), strings.HasPrefix(t, "híbrido"):
		return "hybrid"
	default:
		return ""
	}
}
