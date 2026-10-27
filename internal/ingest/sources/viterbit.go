package sources

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// viterbit adapts the career sites of Viterbit, a Spanish ATS whose customers — staffing firms
// (Temps, Staff Global Group, Trianglerrhh), hospitals (Grup Montaner), consultancies, NGOs (CEAR)
// — publish at https://<board>.viterbit.site. The board is that label.
//
//	/                     every open job, one server-rendered page (a.vb-job-title cards)
//	/<title-slug>-<id>/   one job, with a schema.org JobPosting
//
// The page has no pagination: the largest site measured (temps, 1,877 jobs) is one 9.5 MB page.
// The id is the 12-character token that ends every job path; it is the only id a card carries,
// so it is the identity a HydratingSource tests before buying the page.
//
// Cloudflare challenges every plain client ("Just a moment...", 403) from the crawl host, the
// fixed proxy and a laptop alike, so on the crawl host the provider goes through the Scrapling
// tier's impersonating fetcher (scraplingtier.go). An unknown label answers 404.
type viterbit struct {
	http         HTMLGetter
	maxNewPerRun int
}

// NewViterbit builds the Viterbit adapter over the given HTML client.
func NewViterbit(c HTMLGetter) Source {
	return viterbit{http: c, maxNewPerRun: viterbitMaxNewPerBoard}
}

func (viterbit) Provider() string { return "viterbit" }

const (
	// viterbitMaxNewPerBoard bounds the bodies one board buys in a run, so the first crawl of a
	// staffing firm's 1,877 jobs is spread over several runs instead of outliving the timeout.
	viterbitMaxNewPerBoard = 300
	// viterbitDetailWorkers keeps the fan-out low: every request goes through the crawl host's
	// Scrapling service.
	viterbitDetailWorkers = 3
)

var (
	viterbitBoardPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	// viterbitJobPath captures a job's id from its path: a slug, a dash, and the 12-character id.
	viterbitJobPath = regexp.MustCompile(`^/[a-z0-9-]+-([A-Za-z0-9]{12})/?$`)
)

// viterbitCard is one job card of the site's page.
type viterbitCard struct {
	id, url, title string
}

// Fetch reads the site's page and every job's page, within the per-board budget.
func (s viterbit) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew reads the site's page and buys a job page only for jobs the catalogue does not hold,
// at most maxNewPerRun of them; a seen job is re-listed as a SeenRefresh, its card proving it is
// still open, and an unseen job past the budget is left for a later run.
func (s viterbit) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	board := strings.ToLower(strings.TrimSpace(e.Board))
	if !viterbitBoardPattern.MatchString(board) {
		return nil, fmt.Errorf("viterbit: board %q is not a site label", e.Board)
	}
	base := &url.URL{Scheme: "https", Host: board + ".viterbit.site", Path: "/"}
	root, err := s.http.GetHTML(ctx, base.String())
	if err != nil {
		return nil, fmt.Errorf("viterbit: board %s: %w", board, err)
	}
	var jobs []Job
	var fresh []viterbitCard
	deferred := 0
	for _, c := range viterbitCards(base, root) {
		switch {
		case seen(c.id):
			jobs = append(jobs, Job{ExternalID: c.id, URL: c.url, Title: c.title, Company: e.Company, SeenRefresh: true})
		case len(fresh) < s.maxNewPerRun:
			fresh = append(fresh, c)
		default:
			deferred++
		}
	}
	if deferred > 0 {
		log.Printf("viterbit: board %q: hydration budget %d reached; %d new jobs deferred to a later run",
			board, s.maxNewPerRun, deferred)
	}
	return append(jobs, fetchDetails(fresh, viterbitDetailWorkers, func(c viterbitCard) (Job, bool) {
		return s.detail(ctx, e, c)
	})...), nil
}

// viterbitCards reads the job cards of the site's page: each is an a.vb-job-title naming the job
// and linking to its page. The same job can be carded twice (a featured strip and the list);
// the id dedupes them.
func viterbitCards(base *url.URL, root *html.Node) []viterbitCard {
	var out []viterbitCard
	seen := map[string]bool{}
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "a" || !HasClass(n, "vb-job-title") {
			return true
		}
		ref, err := base.Parse(strings.TrimSpace(Attr(n, "href")))
		if err != nil || ref.Host != base.Host {
			return false
		}
		id := firstSubmatch(viterbitJobPath, ref.Path)
		if id == "" || seen[id] {
			return false
		}
		seen[id] = true
		ref.RawQuery, ref.Fragment = "", ""
		out = append(out, viterbitCard{id: id, url: ref.String(), title: strings.Join(strings.Fields(textContent(n)), " ")})
		return false
	})
	return out
}

// viterbitPosting is the schema.org JobPosting of a job page.
type viterbitPosting struct {
	Title           string       `json:"title"`
	Description     string       `json:"description"`
	DatePosted      string       `json:"datePosted"`
	EmploymentType  string       `json:"employmentType"`
	JobLocation     schemaPlaces `json:"jobLocation"`
	JobLocationType string       `json:"jobLocationType"`
}

// detail fetches one job's page and maps its JobPosting. A 404 is the site's own "no such job"
// and drops it; any other failure, or a page without a JobPosting, is an unreadable marker — the
// page is the only source of the body.
func (s viterbit) detail(ctx context.Context, e CompanyEntry, c viterbitCard) (Job, bool) {
	root, err := s.http.GetHTML(ctx, c.url)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(c.id, c.url, e.Company), true
		}
		return Job{}, false
	}
	var p viterbitPosting
	if !ldJobPosting(root, &p) || strings.TrimSpace(p.Description) == "" {
		return unreadableDetail(c.id, c.url, e.Company), true
	}
	var places []string
	var codes []string
	for _, pl := range p.JobLocation {
		a := pl.Address
		places = append(places, joinNonEmpty(a.AddressLocality, viterbitRegion(a), a.AddressCountry))
		codes = append(codes, a.AddressCountry)
	}
	j := Job{
		ExternalID:     c.id,
		URL:            c.url,
		Title:          firstNonEmpty(strings.TrimSpace(p.Title), c.title),
		Company:        e.Company,
		Location:       strings.Join(places, "; "),
		Description:    sanitizeHTML(html.UnescapeString(p.Description)),
		PostedAt:       parseRFC3339OrDate(p.DatePosted),
		EmploymentType: schemaEmploymentType(p.EmploymentType),
		Countries:      countriesFromCodes(codes),
	}
	// TELECOMMUTE is the platform's remote flag; its absence says nothing about hybrid vs onsite.
	if strings.EqualFold(p.JobLocationType, "TELECOMMUTE") {
		j.WorkMode, j.Remote = "remote", true
	}
	return j, true
}

// viterbitRegion is the address region unless it merely repeats the locality ("Madrid, Madrid").
func viterbitRegion(a schemaAddress) string {
	if strings.EqualFold(strings.TrimSpace(a.AddressRegion), strings.TrimSpace(a.AddressLocality)) {
		return ""
	}
	return a.AddressRegion
}
