package sources

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// neogov adapts NEOGOV career sites (governmentjobs.com and its education vertical
// schooljobs.com), used by US public colleges and school districts. The two domains are
// separate tenant spaces, so the board is "<domain>/<agency>" (e.g.
// "schooljobs.com/cochisecollege"). The listing is a Knockout SPA whose job cards are served
// only through an XHR partial — the same URL without the X-Requested-With header returns the
// empty JS shell. The partial is HTML (li.list-item cards), so this list-only adapter carries
// the card's snippet; the absolute posted date and full description live on the detail page
// (the list shows only a relative "Posted N weeks ago").
type neogov struct {
	http neogovHTTP
}

// neogovHTTP is the transport NEOGOV needs: a text GET that can set the XHR header the
// endpoint requires to return job cards instead of the SPA shell.
type neogovHTTP interface {
	GetTextWithHeaders(ctx context.Context, url string, headers map[string]string) (string, error)
}

// NewNeogov builds the NEOGOV adapter over the given HTTP client.
func NewNeogov(c neogovHTTP) Source { return neogov{http: c} }

func (neogov) Provider() string { return "neogov" }

const (
	// neogovPageSize is the listing's fixed jobs-per-page; total is read from the count span.
	neogovPageSize = 10
	// neogovMaxPages bounds the walk far above any real agency's posting count.
	neogovMaxPages = 200
)

// neogovXHR is the header the listing endpoint requires; without it it returns the SPA shell.
var neogovXHR = map[string]string{"X-Requested-With": "XMLHttpRequest"}

// neogovCountRe pulls the total open-postings count from the listing's header span.
var neogovCountRe = regexp.MustCompile(`id="job-postings-number">\s*(\d+)`)

func (s neogov) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	domain, agency, ok := strings.Cut(e.Board, "/")
	if !ok || domain == "" || agency == "" {
		return nil, fmt.Errorf("neogov: board %q is not <domain>/<agency>", e.Board)
	}

	var (
		jobs  []Job
		seen  = map[string]bool{}
		total int
	)
	for page := 1; page <= neogovMaxPages; page++ {
		url := fmt.Sprintf(
			"https://www.%s/careers/home/index?agency=%s&sort=PositionTitle&isDescendingSort=false&page=%d",
			domain, agency, page)
		frag, err := s.http.GetTextWithHeaders(ctx, url, neogovXHR)
		if err != nil {
			if page == 1 {
				return nil, fmt.Errorf("neogov: list %s: %w", e.Board, err)
			}
			break // a later page failing ends enumeration with what we have
		}
		if page == 1 {
			total = neogovTotal(frag)
		}
		pageJobs, err := neogovParseListing(frag, domain, agency)
		if err != nil {
			return nil, fmt.Errorf("neogov: parse %s: %w", e.Board, err)
		}
		added := 0
		for _, j := range pageJobs {
			if seen[j.ExternalID] {
				continue
			}
			seen[j.ExternalID] = true
			j.Company = e.Company
			jobs = append(jobs, j)
			added++
		}
		if added == 0 || (total > 0 && len(jobs) >= total) {
			break
		}
	}

	// The listing carries only a teaser snippet; fetch each card's detail page for the full
	// body. The detail page is server-rendered, so it needs no XHR header (unlike the
	// listing). A failed or bodyless detail keeps the listing snippet rather than dropping
	// the job, so a transient detail failure never yields a blank description.
	return fetchDetails(jobs, defaultDetailWorkers, func(j Job) (Job, bool) {
		if page, err := s.http.GetTextWithHeaders(ctx, j.URL, nil); err == nil {
			if full := neogovDetailDescription(page); full != "" {
				j.Description = full
			}
		}
		return j, true
	}), nil
}

// neogovTotal reads the open-postings count from the listing header, or 0 when absent (an
// invalid agency returns the SPA shell, which carries no such span).
func neogovTotal(fragment string) int {
	m := neogovCountRe.FindStringSubmatch(fragment)
	if m == nil {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}

// neogovParseListing parses a listing fragment's job cards into jobs. Each card is a
// li.list-item[data-job-id]; the title/href come from its item-details-link, the location
// from the first list-meta entry, and the snippet from list-entry.
func neogovParseListing(fragment, domain, agency string) ([]Job, error) {
	root, err := html.Parse(strings.NewReader(fragment))
	if err != nil {
		return nil, err
	}
	base := "https://www." + domain
	var jobs []Job
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "li" || !hasClass(n, "list-item") {
			return true
		}
		id := attr(n, "data-job-id")
		if id == "" {
			return true // e.g. a "no results" placeholder card
		}
		link := firstByClass(n, "item-details-link")
		if link == nil {
			return true
		}
		var desc string
		if entry := firstByClass(n, "list-entry"); entry != nil {
			desc = textContent(entry)
		}
		jobs = append(jobs, Job{
			ExternalID:  id,
			Title:       textContent(link),
			URL:         base + attr(link, "href"),
			Location:    neogovFirstMeta(n),
			Description: desc,
		})
		return true
	})
	return jobs, nil
}

// neogovDetailDescription extracts the full posting body from a detail page: the
// #details-info container (Froala's .fr-view) holding the Definition, Minimum
// Qualifications, and Supplemental Information sections. It returns the sanitized inner
// HTML, or "" when the container is absent or empty (the caller then keeps the listing
// snippet). The sibling #details-benefits/#details-questions tabs are deliberately not
// captured — only the job description belongs in the description.
func neogovDetailDescription(page string) string {
	root, err := html.Parse(strings.NewReader(page))
	if err != nil {
		return ""
	}
	info := firstByID(root, "details-info")
	if info == nil {
		return ""
	}
	return sanitizeHTML(innerHTML(info))
}

// neogovFirstMeta returns the text of the first <li> inside the card's list-meta list (the
// location), or "" when the card carries none.
func neogovFirstMeta(card *html.Node) string {
	meta := firstByClass(card, "list-meta")
	if meta == nil {
		return ""
	}
	var loc string
	walk(meta, func(n *html.Node) bool {
		if loc != "" {
			return false
		}
		if n.Type == html.ElementNode && n.Data == "li" {
			loc = textContent(n)
			return false
		}
		return true
	})
	return loc
}
