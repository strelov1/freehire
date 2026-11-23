package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/strelov1/freehire/internal/platform/flexjson"
	"golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// jobsACUK adapts the public UK higher-education jobs board. Its unfiltered search lists
// academic, professional and support roles, with the employer and full body in each detail's
// JobPosting. See providers/jobsacuk.md for the measured listing and detail shapes.
type jobsACUK struct {
	http    HTMLGetter
	limiter waiter
	now     func() time.Time
}

const (
	jobsACUKBaseURL        = "https://www.jobs.ac.uk"
	jobsACUKSearchURL      = jobsACUKBaseURL + "/search/?sortOrder=%d&pageSize=25&startIndex=%d"
	jobsACUKPageSize       = 25
	jobsACUKMaxPages       = 200
	jobsACUKFreshPerRun    = 600
	jobsACUKBacklogPerRun  = 200
	jobsACUKDetailWorkers  = 2
	jobsACUKDeadlineMargin = 3 * time.Minute
)

var errJobsACUKListingDrift = errors.New("jobsacuk: listing shifted during pagination")

var jobsACUKPostingPath = regexp.MustCompile(`^/job/([A-Z]{3}[0-9]{3})/[^/?#]+$`)

// NewJobsACUK builds the adapter over the shared HTTP client.
func NewJobsACUK(c HTMLGetter) Source {
	return jobsACUK{http: c, limiter: rate.NewLimiter(rate.Every(time.Second), 1), now: time.Now}
}

func (jobsACUK) Provider() string { return "jobsacuk" }
func (jobsACUK) Boardless()       {}
func (jobsACUK) Aggregator()      {}

func (s jobsACUK) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s jobsACUK) FetchNew(ctx context.Context, _ CompanyEntry, seen func(string) bool) ([]Job, error) {
	postings, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var unseen []listedPosting
	jobs := make([]Job, 0, len(postings))
	for _, p := range postings {
		if seen(p.id) {
			jobs = append(jobs, Job{ExternalID: p.id, URL: p.url, SeenRefresh: true})
		} else {
			unseen = append(unseen, p)
		}
	}
	// Keep a rotating window so listings that are routinely rejected or unreadable cannot
	// monopolize a bounded batch. The primary search orders the first lane by date placed.
	picked := hydrationLanes(unseen, jobsACUKFreshPerRun, jobsACUKBacklogPerRun, s.now())
	var attempted, answered atomic.Int64
	details := fetchDetails(picked, jobsACUKDetailWorkers, func(p listedPosting) (Job, bool) {
		if d, ok := ctx.Deadline(); ok && time.Until(d) < jobsACUKDeadlineMargin {
			return Job{}, false
		}
		attempted.Add(1)
		job, ok, err := s.detail(ctx, p)
		if err != nil {
			log.Printf("jobsacuk: posting %s failed; deferring: %v", p.id, err)
			return unreadableDetail(p.id, p.url, ""), true
		}
		answered.Add(1)
		return job, ok
	})
	if n := attempted.Load(); n > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("jobsacuk: requested %d posting pages and none answered", n)
	}
	return append(jobs, details...), nil
}

func (s jobsACUK) get(ctx context.Context, u string) (*html.Node, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	return s.http.GetHTML(ctx, u)
}

func (s jobsACUK) list(ctx context.Context) ([]listedPosting, error) {
	postings, err := s.listSnapshot(ctx, 1)
	if !errors.Is(err, errJobsACUKListingDrift) {
		return postings, err
	}
	log.Printf("%v; retrying from the Closing Date order", err)
	return s.listSnapshot(ctx, 2)
}

// listSnapshot verifies one complete order or fills unreadable pages from the other order.
// A changing total or a duplicate within one order invalidates the whole snapshot.
func (s jobsACUK) listSnapshot(ctx context.Context, firstOrder int) ([]listedPosting, error) {
	var postings []listedPosting
	distinct := make(map[string]bool)
	total := -1
	var firstGap error
	for _, order := range []int{firstOrder, 3 - firstOrder} {
		withinOrder := make(map[string]bool)
		for offset := 1; total < 0 || offset <= max(1, total); offset += jobsACUKPageSize {
			count, page, err := s.searchPage(ctx, order, offset)
			if count >= 0 {
				switch {
				case total < 0 && count > jobsACUKPageSize*jobsACUKMaxPages:
					return nil, fmt.Errorf("jobsacuk: %d results exceed the %d-page listing budget", count, jobsACUKMaxPages)
				case total < 0:
					total = count
				case count != total:
					return nil, fmt.Errorf("%w: search total changed from %d to %d at sort %d offset %d", errJobsACUKListingDrift, total, count, order, offset)
				}
			}
			if err != nil {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				if firstGap == nil {
					firstGap = err
				}
				if total < 0 {
					break
				}
				continue
			}
			if total == 0 {
				return nil, nil
			}
			for _, p := range page {
				if withinOrder[p.id] {
					return nil, fmt.Errorf("%w: sort %d offset %d repeats posting %s", errJobsACUKListingDrift, order, offset, p.id)
				}
				withinOrder[p.id] = true
				if !distinct[p.id] {
					distinct[p.id] = true
					postings = append(postings, p)
				}
			}
			if len(postings) > total {
				return nil, fmt.Errorf("jobsacuk: search returned %d distinct postings, more than the reported %d", len(postings), total)
			}
		}
		if total > 0 && len(postings) == total {
			return postings, nil
		}
	}
	if total < 0 {
		return nil, fmt.Errorf("jobsacuk: neither search order reported a total: %w", firstGap)
	}
	return nil, fmt.Errorf("jobsacuk: both search orders yielded %d of %d postings: %w", len(postings), total, firstGap)
}

func (s jobsACUK) searchPage(ctx context.Context, order, offset int) (int, []listedPosting, error) {
	root, err := s.get(ctx, fmt.Sprintf(jobsACUKSearchURL, order, offset))
	if err != nil {
		return -1, nil, fmt.Errorf("sort %d offset %d: %w", order, offset, err)
	}
	total, page, err := jobsACUKSearchPage(root)
	if err != nil {
		return total, nil, fmt.Errorf("sort %d offset %d: %w", order, offset, err)
	}
	if expected := min(jobsACUKPageSize, total-offset+1); len(page) != expected {
		return total, nil, fmt.Errorf("sort %d offset %d returned %d results, expected %d", order, offset, len(page), expected)
	}
	return total, page, nil
}

func jobsACUKSearchPage(root *html.Node) (int, []listedPosting, error) {
	count := firstByClass(root, "job-count")
	if count == nil {
		return -1, nil, errors.New("page lacks the result count")
	}
	total, err := strconv.Atoi(strings.ReplaceAll(textContent(count), ",", ""))
	if err != nil || total < 0 {
		return -1, nil, errors.New("page has an invalid result count")
	}
	listing := firstByID(root, "job-listings")
	if listing == nil {
		return total, nil, errors.New("page lacks the listings container")
	}
	var postings []listedPosting
	walk(listing, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "a" {
			path := Attr(n, "href")
			if id := jobsACUKPostingPath.FindStringSubmatch(path); id != nil {
				postings = append(postings, listedPosting{id: id[1], url: jobsACUKBaseURL + path})
			}
		}
		return true
	})
	return total, postings, nil
}

type jobsACUKPosting struct {
	Title              string           `json:"title"`
	Description        string           `json:"description"`
	DatePosted         string           `json:"datePosted"`
	ValidThrough       string           `json:"validThrough"`
	EmploymentType     string           `json:"employmentType"`
	HiringOrganization sitemapldjsonOrg `json:"hiringOrganization"`
	JobLocation        schemaPlaces     `json:"jobLocation"`
	BaseSalary         json.RawMessage  `json:"baseSalary"`
}

func (s jobsACUK) detail(ctx context.Context, p listedPosting) (Job, bool, error) {
	root, err := s.get(ctx, p.url)
	if err != nil {
		if detailUnreadable(err) {
			return Job{}, false, err
		}
		return Job{}, false, nil
	}
	var ld jobsACUKPosting
	if !ldJobPosting(root, &ld) {
		return Job{}, false, errors.New("page carries no JobPosting")
	}
	title, company := strings.TrimSpace(ld.Title), strings.TrimSpace(ld.HiringOrganization.Name)
	if title == "" || company == "" {
		return Job{}, false, errors.New("posting lacks a title or employer")
	}
	// The advert's closing date is inclusive. The JSON-LD serializes it at midnight.
	date, _, _ := strings.Cut(strings.TrimSpace(ld.ValidThrough), "T")
	var deadline *time.Time
	if end, err := time.Parse(time.DateOnly, date); err == nil {
		if !s.now().Before(end.AddDate(0, 0, 1)) {
			return Job{}, false, nil
		}
		deadline = &end
	}
	body := sanitizeHTML(ld.Description)
	if textFromHTML(body) == "" {
		return Job{}, false, errors.New("posting description has no readable text")
	}
	location, countries := schemaPlacesLocation(ld.JobLocation)
	j := Job{
		ExternalID:          p.id,
		URL:                 p.url,
		Title:               title,
		Company:             company,
		Location:            location,
		Countries:           countries,
		Description:         body,
		PostedAt:            NotFuture(parseRFC3339OrDate(ld.DatePosted)),
		ApplicationDeadline: deadline,
		EmploymentType:      jobsACUKEmploymentType(ld.EmploymentType),
	}
	jobsACUKApplySalary(&j, ld.BaseSalary)
	return j, true, nil
}

func jobsACUKEmploymentType(raw string) string {
	var full, part bool
	for _, label := range strings.Split(raw, ",") {
		switch strings.ToLower(strings.TrimSpace(label)) {
		case "full time", "full-time", "full_time":
			full = true
		case "part time", "part-time", "part_time":
			part = true
		}
	}
	if full && !part {
		return "full_time"
	}
	if part && !full {
		return "part_time"
	}
	return ""
}

func jobsACUKApplySalary(j *Job, raw json.RawMessage) {
	var amount struct {
		Currency string `json:"currency"`
		Value    struct {
			Value    flexjson.Float `json:"value"`
			MinValue flexjson.Float `json:"minValue"`
			MaxValue flexjson.Float `json:"maxValue"`
			UnitText string         `json:"unitText"`
		} `json:"value"`
	}
	if json.Unmarshal(raw, &amount) != nil {
		return
	}
	currency := strings.ToUpper(strings.TrimSpace(amount.Currency))
	if currency == "" {
		return
	}
	var salary schemaMonetaryAmount
	salary.Currency = currency
	salary.Value.Value = float64(amount.Value.Value)
	salary.Value.MinValue = float64(amount.Value.MinValue)
	salary.Value.MaxValue = float64(amount.Value.MaxValue)
	salary.Value.UnitText = amount.Value.UnitText
	salary.apply(j, currency)
}
