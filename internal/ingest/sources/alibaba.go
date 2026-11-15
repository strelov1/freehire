package sources

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// alibaba adapts Alibaba Group's experienced-hire (社招) careers portal, talent.alibaba.com,
// whose search lists the postings of every Alibaba business (Alibaba Cloud, Taotian, Cainiao,
// Lazada, Amap, ...) under one index. Single-company and boardless; the business is the title's
// prefix ("阿里云智能-…"). Campus hiring is a separate site and is not read.
//
// The crawl is scoped to the portal's own 技术类 (Technology) category, asked for by its sub-category
// codes: our title dictionary cannot read Chinese titles, so the source's category is what states
// a posting is technical, and it sets IsTechHint.
//
// The search is a POST behind a stateless double-submit CSRF check: any request whose
// XSRF-TOKEN cookie matches its X-XSRF-TOKEN header is served, so each run mints one token.
// A row carries the whole posting, body included; no hydration.
//
// fullBoardListing: the walk reads pages until the distinct postings equal the search's count,
// any page failing fails the crawl, and a row whose stated category is not technical — the
// search ignoring the filter — fails it too.
type alibaba struct {
	http HeaderJSONPoster
}

const (
	alibabaSearchURL = "https://talent.alibaba.com/position/search"
	// alibabaJobURL is the posting's page. talent.alibaba.com redirects every deep link — this
	// one included, in a real browser too — to its portal home; talent-holding.alibaba.com, the
	// same platform's site for the holding company, renders any posting id.
	alibabaJobURL = "https://talent-holding.alibaba.com/off-campus/position-detail?lang=zh&positionId="
	// alibabaPageSize is the search's own ceiling: a larger pageSize is served as 500 rows while
	// the page offset still advances by the size asked for, which would skip rows.
	alibabaPageSize = 500
	// alibabaMaxPages bounds the walk at five times the technology category; reaching it fails.
	alibabaMaxPages = 15
)

// alibabaTechSubcategories are the codes under 技术类 (130) in the portal's own category list (POST
// /category/list). Asking for the parent code alone matches only the few postings filed under the
// parent itself, so the children are named.
const alibabaTechSubcategories = "133,135,136,137,407,408,409,410,411,511,702,703,704,747,764,769,798,811"

// NewAlibaba builds the Alibaba careers adapter over the given HTTP client.
func NewAlibaba(c HeaderJSONPoster) Source { return alibaba{http: c} }

func (alibaba) Provider() string { return "alibaba" }

// alibaba is single-company, so its config entry carries no board.
func (alibaba) boardless() {}

// fullBoardListing: see the type comment and the fullBoardListing interface (source.go).
func (alibaba) fullBoardListing() {}

type alibabaQuery struct {
	Channel       string   `json:"channel"`
	Language      string   `json:"language"`
	BatchID       string   `json:"batchId"`
	Categories    string   `json:"categories"`
	DeptCodes     []string `json:"deptCodes"`
	Key           string   `json:"key"`
	PageIndex     int      `json:"pageIndex"`
	PageSize      int      `json:"pageSize"`
	Regions       string   `json:"regions"`
	SubCategories string   `json:"subCategories"`
}

type alibabaResult struct {
	Success  bool   `json:"success"`
	ErrorMsg string `json:"errorMsg"`
	Content  *struct {
		TotalCount *int          `json:"totalCount"`
		Datas      []alibabaPost `json:"datas"`
	} `json:"content"`
}

type alibabaPost struct {
	ID            int64    `json:"id"`
	Name          string   `json:"name"`
	Categories    []string `json:"categories"`
	PublishTime   int64    `json:"publishTime"`
	WorkLocations []string `json:"workLocations"`
	Description   string   `json:"description"`
	Requirement   string   `json:"requirement"`
	Experience    *struct {
		From *int `json:"from"`
	} `json:"experience"`
}

func (s alibaba) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	token, err := alibabaToken()
	if err != nil {
		return nil, err
	}
	headers := map[string]string{"Cookie": "XSRF-TOKEN=" + token, "X-XSRF-TOKEN": token}
	var (
		jobs  []Job
		seen  = map[int64]bool{}
		count = -1
	)
	for page := 1; count < 0 || len(seen) < count; page++ {
		if page > alibabaMaxPages {
			return nil, fmt.Errorf("alibaba: %d pages read %d of %d postings", alibabaMaxPages, len(seen), count)
		}
		q := alibabaQuery{Channel: "group_official_site", Language: "zh", DeptCodes: []string{},
			PageIndex: page, PageSize: alibabaPageSize, SubCategories: alibabaTechSubcategories}
		var r alibabaResult
		if err := s.http.PostJSONWithHeaders(ctx, alibabaSearchURL, headers, q, &r); err != nil {
			return nil, fmt.Errorf("alibaba: page %d: %w", page, err)
		}
		if !r.Success || r.Content == nil || r.Content.TotalCount == nil {
			return nil, fmt.Errorf("alibaba: page %d: %s", page, r.ErrorMsg)
		}
		if count < 0 {
			count = *r.Content.TotalCount
			if count == 0 {
				// The portal always lists technical postings; none is a changed search, and
				// must not read as every posting closed.
				return nil, fmt.Errorf("alibaba: the technology category lists no postings")
			}
		}
		if len(r.Content.Datas) == 0 {
			return nil, fmt.Errorf("alibaba: page %d is empty with %d of %d postings read", page, len(seen), count)
		}
		for _, p := range r.Content.Datas {
			// Most rows state no category at all; one that states a non-technical one means
			// the filter was not applied.
			for _, c := range p.Categories {
				if !strings.HasPrefix(c, "技术") {
					return nil, fmt.Errorf("alibaba: posting %d is filed under %q, outside the technology category", p.ID, c)
				}
			}
			if p.ID <= 0 || seen[p.ID] {
				continue
			}
			seen[p.ID] = true
			if j, ok := p.toJob(e.Company); ok {
				jobs = append(jobs, j)
			}
		}
	}
	if len(seen) != count {
		return nil, fmt.Errorf("alibaba: read %d distinct postings of %d", len(seen), count)
	}
	return jobs, nil
}

// alibabaToken mints the run's CSRF token, sent as both the cookie and the header.
func alibabaToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("alibaba: csrf token: %w", err)
	}
	return hex.EncodeToString(b), nil
}

func (p alibabaPost) toJob(company string) (Job, bool) {
	title := strings.Join(strings.Fields(p.Name), " ")
	body := plainTextToHTML(p.Description) + plainTextToHTML(p.Requirement)
	if title == "" || company == "" || body == "" {
		return Job{}, false
	}
	id := strconv.FormatInt(p.ID, 10)
	job := Job{
		ExternalID:  id,
		URL:         alibabaJobURL + id,
		Title:       title,
		Company:     company,
		Description: sanitizeHTML(body),
		PostedAt:    parseEpochMillis(p.PublishTime),
		IsTechHint:  true, // the crawl reads only the portal's technology category
	}
	job.Location, job.Countries = cnPlaces(p.WorkLocations)
	if p.Experience != nil && p.Experience.From != nil && *p.Experience.From > 0 {
		years := *p.Experience.From
		job.ExperienceYearsMin = &years
	}
	return job, true
}
