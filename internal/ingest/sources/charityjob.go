package sources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"path"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// CharityJob's own UK charity-sector listing covers every role. Its cards name open postings;
// each posting's own page carries the full body in JobPosting JSON-LD.
type charityjob struct {
	http         HTMLGetter
	maxNewPerRun int
}

const (
	charityjobHost = "www.charityjob.co.uk"
	charityjobBase = "https://" + charityjobHost

	// The crawl-host browser took about six seconds per page. One sequential browser request
	// at a time, with at least five seconds between starts, paces listing and detail together.
	charityjobRequestInterval = 5 * time.Second
	charityjobMaxPages        = 250
	// On the measured 115-page listing this keeps a typical cold run around 36 minutes.
	charityjobDetailBudget = 250
	charityjobBacklogLane  = 50
)

// NewCharityJob builds the CharityJob adapter over the supplied HTML client.
func NewCharityJob(c HTMLGetter) Source {
	return charityjob{http: c, maxNewPerRun: charityjobDetailBudget}
}

func pacedCharityJob(c HTMLGetter) Source {
	return NewCharityJob(pacedHTMLGetter(c, charityjobRequestInterval, 1))
}

func (charityjob) Provider() string { return "charityjob" }
func (charityjob) Boardless()       {}
func (charityjob) Aggregator()      {}

type charityjobCard struct {
	id, url, employer string
}

func (s charityjob) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s charityjob) FetchNew(ctx context.Context, _ CompanyEntry, seen func(string) bool) ([]Job, error) {
	cards, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	var pending []charityjobCard
	for _, card := range cards {
		if seen(card.id) {
			jobs = append(jobs, Job{ExternalID: card.id, URL: card.url, SeenRefresh: true})
		} else {
			pending = append(pending, card)
		}
	}
	backlog := min(charityjobBacklogLane, s.maxNewPerRun)
	queued := hydrationLanes(pending, s.maxNewPerRun-backlog, backlog, time.Now())
	if deferred := len(pending) - len(queued); deferred > 0 {
		log.Printf("charityjob: %d pending postings deferred by the detail budget", deferred)
	}
	answered := false
	for _, card := range queued {
		job, ok, err := s.detail(ctx, card)
		if err != nil {
			log.Printf("charityjob: posting %s unreadable: %v", card.id, err)
			jobs = append(jobs, unreadableDetail(card.id, card.url, card.employer))
			continue
		}
		answered = true
		if ok {
			jobs = append(jobs, job)
		}
	}
	if len(queued) > 0 && !answered {
		return nil, fmt.Errorf("charityjob: attempted %d details and none answered", len(queued))
	}
	return jobs, nil
}

func charityjobListingURL(page int) string {
	if page == 1 {
		return charityjobBase + "/jobs"
	}
	return fmt.Sprintf("%s/jobs?page=%d", charityjobBase, page)
}

// The visible page summary identifies the actual page and its terminal position. A redirect,
// generic 200, or a full repeated page cannot silently cut the crawl short.
func (s charityjob) list(ctx context.Context) ([]charityjobCard, error) {
	var cards []charityjobCard
	seen := map[string]bool{}
	pages := 0
	for page := 1; ; page++ {
		root, err := s.http.GetHTML(ctx, charityjobListingURL(page))
		if err != nil {
			return nil, fmt.Errorf("charityjob: listing page %d: %w", page, err)
		}
		pager := firstByClass(root, "job-paging-summary")
		if pager == nil {
			return nil, fmt.Errorf("charityjob: listing page %d carries no pager", page)
		}
		var current, last int
		if _, err := fmt.Sscanf(textContent(pager), "Page %d of %d", &current, &last); err != nil ||
			current != page || last < page || last > charityjobMaxPages {
			return nil, fmt.Errorf("charityjob: listing page %d has an invalid pager", page)
		}
		if page == 1 {
			pages = last
		}
		if last != pages {
			return nil, fmt.Errorf("charityjob: listing page %d changed the page count from %d to %d", page, pages, last)
		}
		before := len(cards)
		for _, card := range charityjobCards(root) {
			if !seen[card.id] {
				seen[card.id] = true
				cards = append(cards, card)
			}
		}
		if len(cards) == before {
			return nil, fmt.Errorf("charityjob: listing page %d has no new open posting IDs", page)
		}
		if page == pages {
			return cards, nil
		}
	}
}

var charityjobPostingPath = regexp.MustCompile(`^/jobs/[^/]+/[^/]+/([0-9]+)$`)

func charityjobPostingLink(href string) (id, canonical string) {
	u, err := url.Parse(href)
	if err != nil || u.User != nil || (u.Host != "" && (u.Scheme != "https" || u.Host != charityjobHost)) ||
		(u.Host == "" && u.Scheme != "") {
		return "", ""
	}
	// Reject paths that could normalize away the permitted posting route.
	if path.Clean(u.Path) != u.Path || strings.Contains(u.Path, "\\") {
		return "", ""
	}
	match := charityjobPostingPath.FindStringSubmatch(u.Path)
	if match == nil {
		return "", ""
	}
	return match[1], charityjobBase + u.EscapedPath()
}

func charityjobCards(root *html.Node) []charityjobCard {
	var cards []charityjobCard
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "article" || !HasClass(n, "job-card-wrapper") {
			return true
		}
		if Attr(n, "is-expired-job") == "true" || Attr(n, "is-future-job") == "true" {
			return false
		}
		card := charityjobCard{id: Attr(n, "job-id")}
		if logo := firstByClass(n, "job-card-logo"); logo != nil {
			card.employer = collapseSpace(Attr(logo, "title"))
		}
		walk(n, func(link *html.Node) bool {
			if card.url != "" {
				return false
			}
			if link.Type == html.ElementNode && link.Data == "a" {
				if id, canonical := charityjobPostingLink(Attr(link, "href")); id == card.id {
					card.url = canonical
				}
			}
			return true
		})
		if card.url != "" {
			cards = append(cards, card)
		}
		return false
	})
	return cards
}

type charityjobPosting struct {
	URL                string `json:"url"`
	Title              string `json:"title"`
	Description        string `json:"description"`
	DatePosted         string `json:"datePosted"`
	ValidThrough       string `json:"validThrough"`
	EmploymentType     any    `json:"employmentType"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation                   schemaPlaces          `json:"jobLocation"`
	JobLocationType               string                `json:"jobLocationType"`
	ApplicantLocationRequirements schemaNamedAreas      `json:"applicantLocationRequirements"`
	BaseSalary                    *schemaMonetaryAmount `json:"baseSalary"`
}

func (s charityjob) detail(ctx context.Context, card charityjobCard) (Job, bool, error) {
	root, err := s.http.GetHTML(ctx, card.url)
	if err != nil {
		if !detailUnreadable(err) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	var ld charityjobPosting
	if !ldJobPosting(root, &ld) {
		return Job{}, false, errors.New("page carries no JobPosting data")
	}
	id, _ := charityjobPostingLink(ld.URL)
	title, employer := collapseSpace(ld.Title), collapseSpace(ld.HiringOrganization.Name)
	body := sanitizeHTML(ld.Description)
	if id != card.id || title == "" || employer == "" || strings.TrimSpace(textFromHTML(body)) == "" {
		return Job{}, false, errors.New("page carries no usable JobPosting for the listed ID")
	}
	location, countries := schemaPlacesLocation(ld.JobLocation)
	remote := ld.JobLocationType == "TELECOMMUTE"
	mode := workModeFromRemote(remote)
	if remote {
		location = "Remote"
		if allowed := countriesFromCodes(ld.ApplicantLocationRequirements.Names()); len(allowed) > 0 {
			countries = allowed
		}
	} else {
		mode = charityjobOfficeMode(root)
	}
	j := Job{
		ExternalID:     card.id,
		URL:            card.url,
		Title:          title,
		Company:        employer,
		Location:       location,
		Countries:      countries,
		Description:    body,
		Remote:         remote,
		WorkMode:       mode,
		PostedAt:       parseDate(ld.DatePosted),
		EmploymentType: schemaScheduleType(ld.EmploymentType),
	}
	// The published closing day can be in the future.
	if deadline, err := time.Parse(time.DateOnly, ld.ValidThrough); err == nil {
		j.ApplicationDeadline = &deadline
	}
	ld.BaseSalary.apply(&j, "GBP")
	return j, true, nil
}

// JSON-LD names the place but omits the hybrid and on-site distinction.
func charityjobOfficeMode(root *html.Node) string {
	summary := firstByClass(root, "job-details-summary")
	if summary == nil {
		return ""
	}
	if label := firstByClass(summary, "location-wrapper"); label != nil {
		text := textContent(label)
		switch {
		case strings.HasSuffix(text, "(Hybrid)"):
			return "hybrid"
		case strings.HasSuffix(text, "(On-site)"):
			return "onsite"
		}
	}
	return ""
}
