package sources

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/dict/location"
)

// tencent adapts Tencent's own careers site, careers.tencent.com — the mainland-China
// experienced-hire catalogue. Single-company and boardless. Tencent's international hiring lives
// on its Workday tenant, which the workday adapter already crawls; this site re-lists those
// postings too (SourceID 4, the posting URL on myworkdayjobs.com), and they are skipped here.
// Every campus posting the site lists is such a copy; mainland campus hiring is a separate site.
//
// The crawl is scoped to the site's own 技术 (Technology, 40001) job category: our title
// dictionary cannot read Chinese titles (classify resolved a category for 5 of 708 of them and
// confirmed none of any category non-technical), so the source's category is what states that
// a posting is technical, and it sets IsTechHint.
//
// The listing carries the posting's responsibilities but not its requirements, so a posting
// the catalogue does not hold yet is hydrated from the site's own posting API (HydratingSource);
// a seen one costs no request.
//
// fullBoardListing: the listing is ONE request asking for more rows than the category holds,
// and it must read exactly the category's count, every id distinct; a category larger than
// tencentPageSize fails rather than paging.
type tencent struct {
	http JSONGetter
}

const (
	tencentQueryURL  = "https://careers.tencent.com/tencentcareer/api/post/Query"
	tencentDetailURL = "https://careers.tencent.com/tencentcareer/api/post/ByPostId"
	tencentJobURL    = "https://careers.tencent.com/jobdesc.html?postId="
	// tencentTechCategory is the site's top-level 技术 (Technology) job category.
	tencentTechCategory = "40001"
	// tencentPageSize is the most rows the listing request asks for: over twice the whole
	// technical category, and a ceiling that fails the crawl when it outgrows it.
	tencentPageSize = 2000
	// tencentNativeSource is the SourceID of a posting authored on this site; 4 is a re-listed
	// Workday posting.
	tencentNativeSource = 1
)

// tencentSubcategories maps the posting's sub-category (OuterPostTypeID, detail only) onto
// vocab.CategoryValues where it names one discipline; Software Engineering, Technology
// Operations, "AI, Algorithm and Big Data" and Enterprise Management span several.
var tencentSubcategories = map[string]string{
	"40001002": "qa",       // Quality Control
	"40001004": "security", // Security
}

var tencentPostIDRE = regexp.MustCompile(`^[0-9]+$`)

// NewTencent builds the Tencent careers adapter over the given HTTP client.
func NewTencent(c JSONGetter) Source { return tencent{http: c} }

func (tencent) Provider() string { return "tencent" }

// tencent is single-company, so its config entry carries no board.
func (tencent) boardless() {}

// fullBoardListing: see the type comment and the fullBoardListing interface (source.go).
func (tencent) fullBoardListing() {}

type tencentQueryResult struct {
	Code int `json:"Code"`
	Data *struct {
		Count *int          `json:"Count"`
		Posts []tencentPost `json:"Posts"`
	} `json:"Data"`
}

type tencentPost struct {
	PostID       string `json:"PostId"`
	Name         string `json:"RecruitPostName"`
	CountryName  string `json:"CountryName"`
	LocationName string `json:"LocationName"`
	LastUpdate   string `json:"LastUpdateTime"`
	SourceID     int    `json:"SourceID"`
}

type tencentDetailResult struct {
	Code int `json:"Code"`
	Data *struct {
		PostID         string `json:"PostId"`
		Name           string `json:"RecruitPostName"`
		Subcategory    string `json:"OuterPostTypeID"`
		Introduction   string `json:"Introduction"`
		Responsibility string `json:"Responsibility"`
		Requirement    string `json:"Requirement"`
	} `json:"Data"`
}

func (s tencent) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s tencent) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	list, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	return fetchDetails(list, defaultDetailWorkers, func(p tencentPost) (Job, bool) {
		if seen(p.PostID) {
			return Job{ExternalID: p.PostID, URL: tencentJobURL + p.PostID, SeenRefresh: true}, true
		}
		return s.detail(ctx, e, p)
	}), nil
}

// list reads the technology category in one request and keeps the postings authored here.
func (s tencent) list(ctx context.Context) ([]tencentPost, error) {
	q := url.Values{}
	q.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))
	q.Set("parentCategoryId", tencentTechCategory)
	q.Set("pageIndex", "1")
	q.Set("pageSize", strconv.Itoa(tencentPageSize))
	// en-us names the country and city in English; the posting text stays as written.
	q.Set("language", "en-us")
	var r tencentQueryResult
	if err := s.http.GetJSON(ctx, tencentQueryURL+"?"+q.Encode(), &r); err != nil {
		return nil, fmt.Errorf("tencent: listing: %w", err)
	}
	if r.Code != 200 || r.Data == nil || r.Data.Count == nil {
		return nil, fmt.Errorf("tencent: listing: code %d", r.Code)
	}
	count := *r.Data.Count
	switch {
	case count > tencentPageSize:
		return nil, fmt.Errorf("tencent: the category lists %d postings, over the %d one request reads", count, tencentPageSize)
	case len(r.Data.Posts) != count:
		return nil, fmt.Errorf("tencent: read %d of %d postings", len(r.Data.Posts), count)
	}
	var out []tencentPost
	seen := map[string]bool{}
	for _, p := range r.Data.Posts {
		if !tencentPostIDRE.MatchString(p.PostID) || seen[p.PostID] {
			return nil, fmt.Errorf("tencent: posting id %q is malformed or listed twice", p.PostID)
		}
		seen[p.PostID] = true
		if p.SourceID == tencentNativeSource {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		// The category always holds Tencent's own postings; none is a changed search, and must
		// not read as every posting closed.
		return nil, fmt.Errorf("tencent: the technology category lists no Tencent-authored postings")
	}
	return out, nil
}

// detail hydrates a new posting from the posting API. A failure defers the posting to the next
// crawl rather than storing a body-less row, which would never be hydrated again once seen.
func (s tencent) detail(ctx context.Context, e CompanyEntry, p tencentPost) (Job, bool) {
	q := url.Values{}
	q.Set("timestamp", strconv.FormatInt(time.Now().UnixMilli(), 10))
	q.Set("postId", p.PostID)
	q.Set("language", "en-us")
	var r tencentDetailResult
	if err := s.http.GetJSON(ctx, tencentDetailURL+"?"+q.Encode(), &r); err != nil {
		log.Printf("tencent: detail %s failed; deferring: %v", p.PostID, err)
		return Job{}, false
	}
	d := r.Data
	if r.Code != 200 || d == nil || d.PostID != p.PostID {
		log.Printf("tencent: detail %s answered code %d; deferring", p.PostID, r.Code)
		return Job{}, false
	}
	title := strings.Join(strings.Fields(firstNonEmpty(d.Name, p.Name)), " ")
	body := plainTextToHTML(d.Introduction) + plainTextToHTML(d.Responsibility) + plainTextToHTML(d.Requirement)
	if title == "" || e.Company == "" || body == "" {
		log.Printf("tencent: detail %s carries no title or body; deferring", p.PostID)
		return Job{}, false
	}
	job := Job{
		ExternalID:  p.PostID,
		URL:         tencentJobURL + p.PostID,
		Title:       title,
		Company:     e.Company,
		Description: sanitizeHTML(body),
		Location:    distinctJoin([]string{p.LocationName, p.CountryName}, ", ", strings.TrimSpace),
		PostedAt:    tencentDate(p.LastUpdate),
		Category:    tencentSubcategories[d.Subcategory],
		IsTechHint:  true, // the crawl reads only the site's technology category
	}
	if c := location.NormalizeCountry(strings.TrimSpace(p.CountryName)); c != "" {
		job.Countries = []string{c}
	}
	return job, true
}

// tencentDate parses the en-us LastUpdateTime, "October 15,2026" — the posting's last update,
// the only date the site states.
func tencentDate(s string) *time.Time {
	return parseLayout("January 2,2006", strings.TrimSpace(s))
}
