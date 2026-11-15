package sources

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"

	xhtml "golang.org/x/net/html"
)

// pythonorg adapts the Python.org job board (python.org/jobs), the Python Software Foundation's
// own board. The whole catalogue is one server-rendered listing (/jobs/, 25 postings a page), so
// the provider is boardless. A listing card states the posting's id, title, employer, place and
// posting date; the body lives only on the posting page, so this is a HydratingSource that buys a
// page only for a posting the catalogue lacks.
//
// The board's RSS feed carries bodies but only the newest 20 postings, so it cannot be the
// listing. The walk follows the ordinary "which page failed decides" rule and claims neither
// fullBoardListing nor fullCatalog. See the Python.org traps in AGENTS.md.
type pythonorg struct {
	http HTMLGetter
}

const (
	pythonorgBase    = "https://www.python.org"
	pythonorgListURL = pythonorgBase + "/jobs/"
	// pythonorgMaxPages is a ceiling far above the catalogue size, never the expected end.
	pythonorgMaxPages = 40
)

// pythonorgPostingRE is a posting's own path; its number is the external id.
var pythonorgPostingRE = regexp.MustCompile(`^/jobs/([0-9]+)/$`)

func NewPythonOrg(c HTMLGetter) Source { return pythonorg{http: c} }

func (pythonorg) Provider() string { return "pythonorg" }
func (pythonorg) boardless()       {}
func (pythonorg) aggregator()      {}

// pythonorgCard is what a listing card states about a posting.
type pythonorgCard struct {
	ID       string
	URL      string
	Title    string
	Company  string
	Location string
	Posted   string
}

func (s pythonorg) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, CompanyEntry{}, func(string) bool { return false })
}

func (s pythonorg) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	cards, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, cards, seen)
}

// list walks the listing until a page adds no card. The page past the last one answers 404,
// which the walk reads as the end of a listing it has already started.
func (s pythonorg) list(ctx context.Context) ([]pythonorgCard, error) {
	byURL := map[string]pythonorgCard{}
	urls, err := crawlPagedLinks(ctx, s.http, pythonorgMaxPages, pythonorgPageURL, func(root *xhtml.Node) []string {
		var out []string
		for _, c := range parsePythonOrgCards(root) {
			byURL[c.URL] = c
			out = append(out, c.URL)
		}
		return out
	})
	if err != nil {
		return nil, fmt.Errorf("pythonorg: listing: %w", err)
	}
	// The board always lists postings, so a first page without a card is a markup change.
	if len(urls) == 0 {
		return nil, fmt.Errorf("pythonorg: listing page 1 carries no job cards")
	}
	out := make([]pythonorgCard, 0, len(urls))
	for _, u := range urls {
		out = append(out, byURL[u])
	}
	return out, nil
}

func pythonorgPageURL(page int) string {
	if page == 1 {
		return pythonorgListURL
	}
	return pythonorgListURL + "?page=" + strconv.Itoa(page)
}

// parsePythonOrgCards reads every card of a listing page's ol.list-recent-jobs. A card's
// span.listing-company-name holds the posting link (the title) followed by the employer as bare
// text; a card missing the id, title or employer is skipped.
func parsePythonOrgCards(root *xhtml.Node) []pythonorgCard {
	list := firstByClass(root, "list-recent-jobs")
	if list == nil {
		return nil
	}
	var out []pythonorgCard
	for li := list.FirstChild; li != nil; li = li.NextSibling {
		if li.Type != xhtml.ElementNode || li.Data != "li" {
			continue
		}
		if c, ok := parsePythonOrgCard(li); ok {
			out = append(out, c)
		}
	}
	return out
}

func parsePythonOrgCard(li *xhtml.Node) (pythonorgCard, bool) {
	name := firstByClass(li, "listing-company-name")
	if name == nil {
		return pythonorgCard{}, false
	}
	var c pythonorgCard
	var company []string
	afterLink := false
	for n := name.FirstChild; n != nil; n = n.NextSibling {
		switch {
		case n.Type == xhtml.ElementNode && n.Data == "a":
			m := pythonorgPostingRE.FindStringSubmatch(Attr(n, "href"))
			if m == nil {
				return pythonorgCard{}, false
			}
			c.ID, c.URL = m[1], pythonorgBase+m[0]
			c.Title = strings.Join(strings.Fields(textContent(n)), " ")
			afterLink = true
		case afterLink && n.Type == xhtml.TextNode:
			company = append(company, n.Data)
		}
	}
	c.Company = strings.Join(strings.Fields(strings.Join(company, " ")), " ")
	if loc := firstByClass(li, "listing-location"); loc != nil {
		c.Location = strings.Join(strings.Fields(textContent(loc)), " ")
	}
	if t := firstByTag(li, "time"); t != nil {
		c.Posted = strings.TrimSpace(Attr(t, "datetime"))
	}
	if c.ID == "" || c.Title == "" || c.Company == "" {
		return pythonorgCard{}, false
	}
	return c, true
}

// toJob is the posting as its card states it, without a body.
func (c pythonorgCard) toJob() Job {
	return Job{
		ExternalID: c.ID,
		URL:        c.URL,
		Title:      c.Title,
		Company:    c.Company,
		Location:   c.Location,
		PostedAt:   NotFuture(parseRFC3339(c.Posted)),
		IsTechHint: true, // the Python Software Foundation's board lists Python roles only
	}
}

// hydrate buys a posting page only for a posting the catalogue lacks. A page that could not be
// read defers that posting to a later run rather than storing it body-less (a stored row is
// seen and would never be hydrated again).
func (s pythonorg) hydrate(ctx context.Context, cards []pythonorgCard, seen func(string) bool) ([]Job, error) {
	var wanted, read atomic.Int64
	jobs := fetchDetails(cards, defaultDetailWorkers, func(c pythonorgCard) (Job, bool) {
		job := c.toJob()
		if seen(c.ID) {
			job.SeenRefresh = true
			return job, true
		}
		wanted.Add(1)
		root, err := s.http.GetHTML(ctx, c.URL)
		if err != nil {
			if detailUnreadable(err) {
				log.Printf("pythonorg: detail %s failed; deferring: %v", c.URL, err)
			}
			return Job{}, false
		}
		job.Description = pythonorgDescription(root)
		if strings.TrimSpace(textFromHTML(job.Description)) == "" {
			log.Printf("pythonorg: detail %s carries no job description; deferring", c.URL)
			return Job{}, false
		}
		read.Add(1)
		return job, true
	})
	// A crawl that listed new postings and read none of them is broken, not quiet.
	if wanted.Load() > 0 && read.Load() == 0 {
		return nil, fmt.Errorf("pythonorg: listed %d new postings and read none of their pages", wanted.Load())
	}
	return jobs, nil
}

// pythonorgDescription is the posting page's div.job-description: the employer's text and the
// board's fixed sections (Restrictions, Requirements, About the Company, Contact Info). The
// leading "Job Title" section only repeats the title, so it is dropped.
func pythonorgDescription(root *xhtml.Node) string {
	div := firstByClass(root, "job-description")
	if div == nil {
		return ""
	}
	for n := div.FirstChild; n != nil; n = n.NextSibling {
		if n.Type == xhtml.ElementNode && n.Data == "h2" &&
			strings.EqualFold(strings.TrimSpace(textContent(n)), "Job Title") {
			// The heading, then the bare title text up to the next element.
			for next := n.NextSibling; next != nil && next.Type == xhtml.TextNode; next = n.NextSibling {
				div.RemoveChild(next)
			}
			div.RemoveChild(n)
			break
		}
	}
	return sanitizeHTML(innerHTML(div))
}
