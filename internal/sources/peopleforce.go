package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
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

// peopleforceMaxPages caps the ?page=N walk so a listing that never yields an empty page
// cannot loop forever (the largest boards seen are a few pages; this is ample headroom).
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

	// Page through the listing, collecting each card's detail URL + title until a page yields
	// no new links (the tail/empty page) or the safety cap is hit. A first-page failure is a
	// board-level error; a later-page failure just stops the walk with what we have.
	seen := map[string]struct{}{}
	var cards []peopleforceListing
	for page := 1; page <= peopleforceMaxPages; page++ {
		listURL := fmt.Sprintf("%s?page=%d", base, page)
		root, err := s.http.GetHTML(ctx, listURL)
		if err != nil {
			if page == 1 {
				return nil, fmt.Errorf("peopleforce: listing %s: %w", e.Board, err)
			}
			break
		}
		added := 0
		for _, c := range peopleforceListings(base, root) {
			if _, ok := seen[c.URL]; !ok {
				seen[c.URL] = struct{}{}
				cards = append(cards, c)
				added++
			}
		}
		if added == 0 {
			break
		}
	}

	// Each posting's description and structured fields come from its own detail fetch, fanned
	// out under the shared bounded pool.
	return fetchDetails(cards, peopleforceDetailWorkers, func(c peopleforceListing) (Job, bool) {
		return s.detail(ctx, e, c)
	}), nil
}

// detail fetches one job's detail page and maps it to a Job, returning ok=false when the fetch
// fails or the URL carries no native id, so the caller skips just that posting.
func (s peopleforce) detail(ctx context.Context, e CompanyEntry, c peopleforceListing) (Job, bool) {
	id := peopleforceJobID(c.URL)
	if id == "" {
		return Job{}, false
	}
	root, err := s.http.GetHTML(ctx, c.URL)
	if err != nil {
		return Job{}, false
	}

	fields := peopleforceDefList(root)
	location := fields["Location"]
	description := ""
	if col := firstByClass(root, "col-lg-8"); col != nil {
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

// peopleforceListings returns each job card's absolute detail URL and title from a listing
// page, resolved against base, deduplicated by URL in first-seen order.
func peopleforceListings(base *url.URL, root *html.Node) []peopleforceListing {
	var out []peopleforceListing
	seen := map[string]struct{}{}
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "a" {
			return true
		}
		href := attr(n, "href")
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
