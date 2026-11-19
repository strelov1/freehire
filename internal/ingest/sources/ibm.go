package sources

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// ibm adapts IBM's careers catalogue, a single-company source with no board id. The postings live
// on IBM's Avature tenant (careers.ibm.com), which is unusable to a crawler: every page, the
// sitemap included, answers an AWS WAF JavaScript challenge (HTTP 202, x-amzn-waf-action:
// challenge), and the ibmglobal.avature.net sitemap lists no postings. What ibm.com/careers/search
// itself calls is IBM's site-search API (an Elasticsearch front door at www-api.ibm.com), whose
// careers index holds every open posting WITH its full text in a `body` field. One paged POST
// yields the whole catalogue and no posting page is fetched.
type ibm struct {
	http JSONPoster
}

const (
	ibmSearchURL = "https://www-api.ibm.com/search/api/v2"
	// ibmJobURL is the posting's own page on IBM's careers site — the address every row's `url`
	// field states, built from the job id.
	ibmJobURL = "https://careers.ibm.com/careers/JobDetail?jobId=%d"
	// ibmPageSize is the API's ceiling: size=100 is served, size=101 is a 400.
	ibmPageSize = 100
	// ibmMaxPages bounds the walk: the API refuses from >= 9,990 outright, so a catalogue that
	// outgrew the window could not be read whole anyway. Reaching it fails the crawl.
	ibmMaxPages = 100
)

// NewIBM builds the IBM careers adapter over the given HTTP client.
func NewIBM(c JSONPoster) Source { return ibm{http: c} }

func (ibm) Provider() string { return "ibm" }

// ibm is single-company, so its config entry carries no board.
func (ibm) Boardless() {}

// fullBoardListing: every page carries the index's exact hit count (`relation: "eq"`), and
// Fetch fails unless the distinct postings it read equal that count. Any page failing fails the
// crawl — a later page included — so a truncated walk can never pass for a smaller catalogue.
func (ibm) FullBoardListing() {}

// ibmPosting is one hit's _source. The field_keyword_NN names are the index's generic facet
// slots; what each holds was read off the live index (see ibmSourceFields).
type ibmPosting struct {
	JobID    int64  `json:"field_text_01"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	Date     string `json:"dcdate"`
	Country  string `json:"field_keyword_05"`
	Area     string `json:"field_keyword_08"`
	WorkMode string `json:"field_keyword_17"`
	Level    string `json:"field_keyword_18"`
	City     string `json:"field_keyword_19"`
	Entity   string `json:"field_keyword_20"`
}

// ibmSourceFields is the _source projection requested. The API returns NO _source unless it is
// named field by field (`true` and `["*"]` both come back empty).
var ibmSourceFields = []string{
	"field_text_01",    // job id (an integer)
	"title",            // posting title
	"body",             // full posting text, flattened, facet values appended (ibmDescription)
	"dcdate",           // YYYY-MM-DD
	"field_keyword_05", // country name
	"field_keyword_08", // area of work ("Software Engineering", "Consulting", ...)
	"field_keyword_17", // "Hybrid", "Remote" or "" (on-site is never stated)
	"field_keyword_18", // "Professional", "Entry Level" or "Internship"
	"field_keyword_19", // "Bangalore, IN" or "Multiple Cities"
	"field_keyword_20", // hiring legal entity, "(0063) IBM India Private Limited"
}

type ibmSearchResponse struct {
	Hits struct {
		Total struct {
			Value    int    `json:"value"`
			Relation string `json:"relation"`
		} `json:"total"`
		Hits []struct {
			Source ibmPosting `json:"_source"`
		} `json:"hits"`
	} `json:"hits"`
}

// ibmSearchBody is the request for one page. The sort is the load-bearing part: the UI's own
// sort (_score, pageviews) ties on every document of an empty query, so successive identical
// requests return them in a different order and offset paging would skip and repeat postings.
// `id` is a unique keyword per document, which makes the order total.
func ibmSearchBody(from int) map[string]any {
	return map[string]any{
		"appId":          "careers",
		"scopes":         []string{"careers2"},
		"query":          map[string]any{"bool": map[string]any{"must": []any{}}},
		"post_filter":    map[string]any{"bool": map[string]any{"must": []any{}}},
		"size":           ibmPageSize,
		"from":           from,
		"sort":           []any{map[string]string{"id": "asc"}},
		"lang":           "zz",
		"localeSelector": map[string]any{},
		"sm":             map[string]string{"query": "", "lang": "zz"},
		"_source":        ibmSourceFields,
	}
}

func (s ibm) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	postings, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	jobs := make([]Job, 0, len(postings))
	for _, p := range postings {
		if job, ok := p.toJob(e); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// list walks the index to its end and proves the result whole: ANY page failing is an error
// (fullBoardListing), and the distinct postings read must equal the last page's exact total —
// a posting published or withdrawn mid-walk shifts the offsets, which this catches and the next
// run repeats.
func (s ibm) list(ctx context.Context) ([]ibmPosting, error) {
	var (
		postings []ibmPosting
		seen     = map[int64]bool{}
		total    = -1
	)
	for page := 0; page < ibmMaxPages; page++ {
		var resp ibmSearchResponse
		if err := s.http.PostJSON(ctx, ibmSearchURL, ibmSearchBody(page*ibmPageSize), &resp); err != nil {
			return nil, fmt.Errorf("ibm: list page %d: %w", page, err)
		}
		if resp.Hits.Total.Relation != "eq" {
			return nil, fmt.Errorf("ibm: list page %d: total is %q %d, not an exact count", page, resp.Hits.Total.Relation, resp.Hits.Total.Value)
		}
		total = resp.Hits.Total.Value
		for _, h := range resp.Hits.Hits {
			if p := h.Source; p.JobID > 0 && !seen[p.JobID] {
				seen[p.JobID] = true
				postings = append(postings, p)
			}
		}
		if len(resp.Hits.Hits) < ibmPageSize {
			if len(postings) != total {
				return nil, fmt.Errorf("ibm: read %d distinct postings, the index states %d", len(postings), total)
			}
			return postings, nil
		}
	}
	return nil, fmt.Errorf("ibm: no end of the listing after %d pages (last stated total %d)", ibmMaxPages, total)
}

// toJob maps one posting, returning ok=false for a row with no title or no body.
func (p ibmPosting) toJob(e CompanyEntry) (Job, bool) {
	title := strings.Join(strings.Fields(p.Title), " ")
	desc := ibmDescription(p)
	if title == "" || desc == "" {
		return Job{}, false
	}
	mode := workplaceTypeMode(p.WorkMode)
	job := Job{
		ExternalID:  strconv.FormatInt(p.JobID, 10),
		URL:         fmt.Sprintf(ibmJobURL, p.JobID),
		Title:       title,
		Company:     e.Company,
		Location:    ibmLocation(p.City, p.Country),
		Countries:   ibmCountries(p.City, p.Country),
		Description: desc,
		WorkMode:    mode,
		Remote:      mode == "remote",
		PostedAt:    parseDate(p.Date),
	}
	switch strings.TrimSpace(p.Level) {
	case "Internship":
		job.EmploymentType, job.Seniority = "internship", "intern"
	case "Entry Level":
		job.Seniority = "junior"
	}
	return job, true
}

// ibmCityCountry matches the "<city>, <alpha-2>" shape of the city facet.
var ibmCityCountry = regexp.MustCompile(`^(.+),\s*([A-Za-z]{2})$`)

// ibmLocation renders "<city>, <country>". The city facet is "BUDAPEST, HU" (case as the
// recruiter typed it) or the placeholder "Multiple Cities", which names no place, so then the
// country alone is the location. A country name the dictionary cannot read ("Korea, Republic
// of") is not spliced in; the city keeps its own alpha-2 code instead.
func ibmLocation(city, country string) string {
	city, country = strings.TrimSpace(city), strings.TrimSpace(country)
	if strings.EqualFold(city, "Multiple Cities") || city == "" {
		return country
	}
	if m := ibmCityCountry.FindStringSubmatch(city); m != nil && countryFromCode(country) != nil {
		return strings.TrimSpace(m[1]) + ", " + country
	}
	return city
}

// ibmCountries reads the structured country: the country facet's name, else the alpha-2 code the
// city facet ends in (which places "Korea, Republic of" postings).
func ibmCountries(city, country string) []string {
	if c := countryFromCode(country); c != nil {
		return c
	}
	if m := ibmCityCountry.FindStringSubmatch(strings.TrimSpace(city)); m != nil {
		return countryFromCode(m[2])
	}
	return nil
}

// ibmBulletGlyph matches an inline bullet: the index flattens the page to one line, so a list
// survives only as "• item • item" (or "·", "*", "●", "▪") between whitespace that is as often a
// no-break space as an ASCII one. Dashes are left alone: between spaces they are far more often
// prose than a list.
var ibmBulletGlyph = regexp.MustCompile(`[\s\p{Zs}]+[•·*●▪][\s\p{Zs}]+`)

// ibmDescription rebuilds the posting body. The index stores the page text with every newline
// collapsed and the posting's own facet values appended ("... Netherlands Consulting Remote
// Professional AMSTERDAM, NL (0096) IBM Nederland B.V."); that suffix is the non-empty facets
// joined by single spaces and is cut, or it would read as the last sentence. The inline bullets
// are put back on their own lines so plainTextToHTML can rebuild the lists; headings are not
// recoverable.
func ibmDescription(p ibmPosting) string {
	body := strings.TrimSpace(p.Body)
	var facets []string
	for _, f := range []string{p.Country, p.Area, p.WorkMode, p.Level, p.City, p.Entity} {
		if f = strings.TrimSpace(f); f != "" {
			facets = append(facets, f)
		}
	}
	if suffix := strings.Join(facets, " "); suffix != "" {
		body = strings.TrimSpace(strings.TrimSuffix(body, suffix))
	}
	if body == "" {
		return ""
	}
	// Some bodies carry the right single quote double-encoded (UTF-8 read as Windows-1252:
	// "Youâ€™ll"), often beside correctly encoded ones in the same text, so the string as a whole
	// cannot be re-decoded. That sequence was the only mojibake seen, so exactly it is repaired.
	body = strings.ReplaceAll(body, "â€™", "’")
	body = ibmBulletGlyph.ReplaceAllString(body, "\n• ")
	return sanitizeHTML(plainTextToHTML(body))
}
