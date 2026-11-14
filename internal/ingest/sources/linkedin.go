package sources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// linkedIn reads a bounded recent search, never an authoritative live catalogue.
// Board is the keyword query; Region is the location passed to LinkedIn search.
type linkedIn struct {
	http HTMLResolvedGetter
	pace waiter
}

const linkedInSearchURL = "https://www.linkedin.com/jobs-guest/jobs/api/seeMoreJobPostings/search"

// NewLinkedIn shares one request budget across all boards and their detail requests.
func NewLinkedIn(c HTMLResolvedGetter) Source {
	return linkedIn{http: c, pace: rate.NewLimiter(rate.Every(2*time.Second), 1)}
}

func (linkedIn) Provider() string { return "linkedin" }
func (linkedIn) aggregator()      {}
func (linkedIn) livenessProbed()  {}
func (linkedIn) probeClosing()    {}

var linkedInID = regexp.MustCompile(`(?:^|-)\d+$`)

// LinkedInPostingID returns the native numeric ID of a public LinkedIn job URL.
// Non-LinkedIn hosts and paths are rejected before a liveness request is issued.
func LinkedInPostingID(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || (u.Hostname() != "linkedin.com" && !strings.HasSuffix(u.Hostname(), ".linkedin.com")) || u.Port() != "" {
		return ""
	}
	path := strings.TrimSuffix(u.Path, "/")
	if !strings.HasPrefix(path, "/jobs/view/") {
		return ""
	}
	slug := strings.TrimPrefix(path, "/jobs/view/")
	if strings.Contains(slug, "/") {
		return ""
	}
	return strings.TrimPrefix(linkedInID.FindString(slug), "-")
}

// LinkedInPosting reads only a complete posting with a verified identity and an explicit
// primary liveness signal. Removed describes the page evidence; ingest does not emit it
// as a removal event, because cmd/liveness must confirm closure twice across runs.
func LinkedInPosting(root *html.Node, finalURL, id string) (Job, error) {
	u, err := url.Parse(finalURL)
	guest := err == nil && u.Scheme == "https" && u.Host == "www.linkedin.com" && u.Path == "/jobs-guest/jobs/api/jobPosting/"+id
	if id == "" || (!guest && LinkedInPostingID(finalURL) != id) {
		return Job{}, fmt.Errorf("linkedin: posting identity mismatch")
	}
	if canonical := metaProperty(root, "og:url"); canonical != "" && LinkedInPostingID(canonical) != id {
		return Job{}, fmt.Errorf("linkedin: canonical identity mismatch")
	}
	card := firstByClass(root, "top-card-layout")
	if card == nil {
		return Job{}, fmt.Errorf("linkedin: missing primary posting card")
	}
	text := func(class string) string {
		n := firstByClass(card, class)
		if n == nil {
			return ""
		}
		return strings.Join(strings.Fields(textContent(n)), " ")
	}
	j := Job{ExternalID: id, URL: "https://www.linkedin.com/jobs/view/" + id + "/", Title: text("top-card-layout__title"), Company: text("topcard__org-name-link")}
	walk(card, func(n *html.Node) bool {
		if j.Location == "" && HasClass(n, "topcard__flavor") && HasClass(n, "topcard__flavor--bullet") {
			j.Location = strings.Join(strings.Fields(textContent(n)), " ")
		}
		return true
	})
	if body := firstByClass(root, "show-more-less-html__markup"); body != nil {
		j.Description = sanitizeHTML(innerHTML(body))
	}
	if j.Title == "" || j.Company == "" || strings.TrimSpace(j.Description) == "" {
		return Job{}, fmt.Errorf("linkedin: incomplete posting")
	}
	closed := text("closed-job__flavor--closed")
	apply := false
	walk(card, func(n *html.Node) bool {
		if Attr(n, "data-modal") == "job-details-topcard-apply-modal" {
			apply = true
		}
		return true
	})
	if strings.EqualFold(closed, "No longer accepting applications") && !apply {
		j.Removed = true
		return j, nil
	}
	if closed == "" && apply {
		return j, nil
	}
	return Job{}, fmt.Errorf("linkedin: no unambiguous posting status")
}

func (s linkedIn) get(ctx context.Context, raw string) (*html.Node, string, error) {
	if err := s.pace.Wait(ctx); err != nil {
		return nil, "", err
	}
	return s.http.GetHTMLResolved(ctx, raw)
}

func (s linkedIn) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s linkedIn) FetchNew(ctx context.Context, e CompanyEntry, seen func(string) bool) ([]Job, error) {
	if strings.TrimSpace(e.Board) == "" || strings.TrimSpace(e.Region) == "" {
		return nil, fmt.Errorf("linkedin: board keyword and region location are required")
	}
	var jobs []Job
	ids := make(map[string]bool)
	listed, read := 0, 0
	// The buffered pipeline discards every job returned alongside an error. Once any
	// posting was read, preserve progress on a later failure; no unseen sweep can act
	// on this partial listing, so stopping successfully cannot falsely close anything.
	stop := func(err error) ([]Job, error) {
		if read > 0 {
			log.Printf("linkedin: keeping %d postings after partial crawl: %v", len(jobs), err)
			return jobs, nil
		}
		return jobs, err
	}
	offset := 0
	for page := 0; page < 10; page++ {
		params := url.Values{"keywords": {e.Board}, "location": {e.Region}, "f_TPR": {"r604800"}, "sortBy": {"DD"}, "start": {strconv.Itoa(offset)}}
		root, final, err := s.get(ctx, linkedInSearchURL+"?"+params.Encode())
		if err != nil {
			return stop(fmt.Errorf("linkedin: listing: %w", err))
		}
		resolved, err := url.Parse(final)
		if err != nil || resolved.Host != "www.linkedin.com" || resolved.Path != "/jobs-guest/jobs/api/seeMoreJobPostings/search" {
			return stop(fmt.Errorf("linkedin: search redirected away from listing"))
		}
		var cards []Job
		pageSize := 0
		walk(root, func(n *html.Node) bool {
			urn := Attr(n, "data-entity-urn")
			if !strings.HasPrefix(urn, "urn:li:jobPosting:") {
				return true
			}
			id := strings.TrimPrefix(urn, "urn:li:jobPosting:")
			pageSize++
			if id == "" || LinkedInPostingID("https://www.linkedin.com/jobs/view/"+id+"/") != id || ids[id] {
				return false
			}
			title, company := firstByClass(n, "base-search-card__title"), firstByClass(n, "base-search-card__subtitle")
			if title == nil || company == nil {
				return false
			}
			j := Job{ExternalID: id, URL: "https://www.linkedin.com/jobs/view/" + id + "/", Title: strings.TrimSpace(textContent(title)), Company: strings.TrimSpace(textContent(company))}
			if j.Title != "" && j.Company != "" {
				ids[id] = true
				cards = append(cards, j)
			}
			return false
		})
		offset += pageSize
		if len(cards) == 0 {
			if page == 0 {
				return nil, fmt.Errorf("linkedin: first listing has no recognizable postings")
			}
			break
		}
		for _, card := range cards {
			listed++
			if seen(card.ExternalID) {
				card.SeenRefresh = true
				jobs = append(jobs, card)
				read++
				continue
			}
			root, final, err := s.get(ctx, "https://www.linkedin.com/jobs-guest/jobs/api/jobPosting/"+card.ExternalID)
			var status *StatusError
			if errors.As(err, &status) && (status.Code == 403 || status.Code == 429 || status.Code == 999) {
				return stop(fmt.Errorf("linkedin: detail refused: %w", err))
			}
			var j Job
			if err == nil {
				j, err = LinkedInPosting(root, final, card.ExternalID)
			}
			if err != nil {
				card.Unreadable = true
				jobs = append(jobs, card)
				continue
			}
			read++
			if !j.Removed {
				jobs = append(jobs, j)
			}
		}
	}
	if listed > 0 && read == 0 {
		return jobs, fmt.Errorf("linkedin: none of %d listed postings could be read", listed)
	}
	return jobs, nil
}
