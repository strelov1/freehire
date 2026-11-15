package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/dict/location"
	"github.com/strelov1/freehire/internal/dict/skilltag"
)

// techInAsia adapts the Tech in Asia job board (techinasia.com/jobs), a Southeast Asian startup
// board whose postings each name their own employer: a boardless aggregator.
//
// The listing is the public Algolia index (`job_postings`) the site's own search page queries, and
// a hit carries the whole posting, body included, so the crawl is a handful of requests and no
// hydration. The search key is read from the current app bundle on every run: the bundle name
// rotates with each deploy and the key with it.
//
// Every run reads the WHOLE index and fails unless the distinct postings read equal the index's own
// exhaustive count, which is what earns fullCatalog. See the Tech in Asia traps in AGENTS.md.
type techInAsia struct {
	http techInAsiaHTTP
}

// techInAsiaHTTP is the transport the adapter needs: the search page and the app bundle as
// text, and the Algolia query as a POST carrying its credential headers.
type techInAsiaHTTP interface {
	TextGetter
	HeaderJSONPoster
}

const (
	techInAsiaSearchURL  = "https://www.techinasia.com/jobs/search"
	techInAsiaPostingURL = "https://www.techinasia.com/jobs/"
	techInAsiaReferer    = "https://www.techinasia.com/"
	techInAsiaAssetBase  = "https://static.techinasia.com/assets/v5/"
	techInAsiaIndex      = "job_postings"
	// techInAsiaHitsPerPage is Algolia's per-request ceiling; the whole board fits one page.
	techInAsiaHitsPerPage = 1000
	// techInAsiaMaxPages is a ceiling far above the board's size, never the expected end: reaching
	// it before the index's last page FAILS the crawl.
	techInAsiaMaxPages = 50
)

var (
	// techInAsiaAssetRE reads only the bundle's file name; the host it is fetched from is fixed
	// (techInAsiaAssetBase), never taken from the page.
	techInAsiaAssetRE = regexp.MustCompile(`/assets/v5/(app\.[0-9a-f]{8}\.js)`)
	techInAsiaCredsRE = regexp.MustCompile(`apiKey:\s*"([A-Za-z0-9]{16,64})"\s*,\s*appId:\s*"([A-Za-z0-9]{6,16})"`)
)

// techInAsiaShorteners are redirectors whose destination is opaque: an employer link through one
// cannot be told apart from a link anywhere else, so the posting page is kept instead.
var techInAsiaShorteners = map[string]bool{
	"bit.ly": true, "cutt.ly": true, "forms.gle": true, "goo.gl": true, "is.gd": true,
	"lnkd.in": true, "rb.gy": true, "rebrand.ly": true, "s.id": true, "shorturl.at": true,
	"t.co": true, "tinyurl.com": true,
}

// techInAsiaTechPositions are the platform's own position values whose postings are all software or
// IT work. "Data & Analytics" and "Hardware & Embedded Systems" are excluded: they carry non-IT
// roles (a newsroom "Researcher", a "Pilot Drone").
var techInAsiaTechPositions = map[string]bool{
	"Software Engineer":             true,
	"Web Development":               true,
	"Mobile Development":            true,
	"QA & Testing":                  true,
	"IT Engineer":                   true,
	"DevOps & Cloud Management":     true,
	"Enterprise Software & Systems": true,
}

// NewTechInAsia builds the Tech in Asia adapter over the given HTTP client.
func NewTechInAsia(c techInAsiaHTTP) Source { return techInAsia{http: c} }

func (techInAsia) Provider() string { return "techinasia" }
func (techInAsia) boardless()       {}
func (techInAsia) aggregator()      {}

// fullCatalog: Fetch reads every page of the one index and fails on any query error, on a
// count that is not exhaustive, on reaching techInAsiaMaxPages and on any gap between the
// postings read and the index's count. See the fullCatalog interface (source.go) for the bar.
func (techInAsia) fullCatalog() {}

type techInAsiaCreds struct {
	appID, apiKey string
}

type techInAsiaResult struct {
	Hits             []techInAsiaHit `json:"hits"`
	NbHits           int             `json:"nbHits"`
	NbPages          int             `json:"nbPages"`
	ExhaustiveNbHits bool            `json:"exhaustiveNbHits"`
}

type techInAsiaNamed struct {
	Name string `json:"name"`
}

// techInAsiaHit is one posting as the index states it.
type techInAsiaHit struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	ExternalLink string `json:"external_link"`
	PublishedAt  string `json:"published_at"`
	Company      struct {
		Name string `json:"name"`
	} `json:"company"`
	City *struct {
		Name        string `json:"name"`
		CountryName string `json:"country_name"`
	} `json:"city"`
	Currency *struct {
		Code string `json:"currency_code"`
	} `json:"currency"`
	Position        *techInAsiaNamed  `json:"position"`
	JobType         *techInAsiaNamed  `json:"job_type"`
	JobSkills       []techInAsiaNamed `json:"job_skills"`
	WorkArrangement string            `json:"work_arrangement"`
	IsRemote        int               `json:"is_remote"`
	IsSalaryVisible int               `json:"is_salary_visible"`
	SalaryMin       float64           `json:"salary_min"`
	SalaryMax       float64           `json:"salary_max"`
	ExperienceMin   *int              `json:"experience_min"`
}

func (s techInAsia) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	creds, err := s.creds(ctx)
	if err != nil {
		return nil, err
	}
	hits, err := s.list(ctx, creds)
	if err != nil {
		return nil, err
	}
	// A link more than one posting points at is an intake form, not that posting's own page,
	// and a URL shared by several postings would read as one posting to every URL consumer.
	links := map[string]int{}
	for _, h := range hits {
		if l := h.externalLink(); l != "" {
			links[l]++
		}
	}
	jobs := make([]Job, 0, len(hits))
	for _, h := range hits {
		if j, ok := h.toJob(links); ok {
			jobs = append(jobs, j)
		}
	}
	return jobs, nil
}

// list reads every page of the index. Any page failing fails the crawl — the provider is swept
// by source, so a truncated read must not look like a shrunken catalogue.
func (s techInAsia) list(ctx context.Context, creds techInAsiaCreds) ([]techInAsiaHit, error) {
	var out []techInAsiaHit
	seen := map[string]bool{}
	total := -1
	for page := 0; ; page++ {
		if page == techInAsiaMaxPages {
			return nil, fmt.Errorf("techinasia: %d pages read and the index has more", techInAsiaMaxPages)
		}
		r, err := s.query(ctx, creds, page)
		if err != nil {
			return nil, err
		}
		if !r.ExhaustiveNbHits {
			return nil, fmt.Errorf("techinasia: page %d: the index's count %d is an estimate", page, r.NbHits)
		}
		if total < 0 {
			// The board always lists hundreds of postings; an empty index is a changed index
			// or key, never an empty market.
			if r.NbHits == 0 {
				return nil, fmt.Errorf("techinasia: the index lists no postings")
			}
			total = r.NbHits
		} else if r.NbHits != total {
			return nil, fmt.Errorf("techinasia: the index changed mid-read (%d postings, then %d)", total, r.NbHits)
		}
		for _, h := range r.Hits {
			if h.ID == "" || seen[h.ID] {
				continue
			}
			seen[h.ID] = true
			out = append(out, h)
		}
		if page+1 >= r.NbPages || len(r.Hits) == 0 {
			break
		}
	}
	if len(seen) != total {
		return nil, fmt.Errorf("techinasia: read %d distinct postings of the %d the index lists", len(seen), total)
	}
	return out, nil
}

func (s techInAsia) query(ctx context.Context, creds techInAsiaCreds, page int) (techInAsiaResult, error) {
	p := url.Values{}
	p.Set("query", "")
	p.Set("hitsPerPage", strconv.Itoa(techInAsiaHitsPerPage))
	p.Set("page", strconv.Itoa(page))
	p.Set("analytics", "false")
	p.Set("attributesToHighlight", "[]")
	u := fmt.Sprintf("https://%s-dsn.algolia.net/1/indexes/%s/query", strings.ToLower(creds.appID), techInAsiaIndex)
	headers := map[string]string{
		"X-Algolia-Application-Id": creds.appID,
		"X-Algolia-API-Key":        creds.apiKey,
		"Referer":                  techInAsiaReferer,
	}
	var r techInAsiaResult
	if err := s.http.PostJSONWithHeaders(ctx, u, headers, map[string]string{"params": p.Encode()}, &r); err != nil {
		return techInAsiaResult{}, fmt.Errorf("techinasia: page %d: %w", page, err)
	}
	return r, nil
}

// creds reads the Algolia app id and search key from the app bundle the search page names.
func (s techInAsia) creds(ctx context.Context) (techInAsiaCreds, error) {
	page, err := s.http.GetText(ctx, techInAsiaSearchURL)
	if err != nil {
		return techInAsiaCreds{}, fmt.Errorf("techinasia: search page: %w", err)
	}
	asset := techInAsiaAssetRE.FindStringSubmatch(page)
	if asset == nil {
		return techInAsiaCreds{}, fmt.Errorf("techinasia: search page names no app bundle")
	}
	bundle, err := s.http.GetText(ctx, techInAsiaAssetBase+asset[1])
	if err != nil {
		return techInAsiaCreds{}, fmt.Errorf("techinasia: app bundle: %w", err)
	}
	m := techInAsiaCredsRE.FindStringSubmatch(bundle)
	if m == nil {
		return techInAsiaCreds{}, fmt.Errorf("techinasia: app bundle carries no Algolia credentials")
	}
	return techInAsiaCreds{apiKey: m[1], appID: m[2]}, nil
}

// externalLink is the posting's own apply link when it states a usable one: http(s) and not
// through a redirector.
func (h techInAsiaHit) externalLink() string {
	raw := strings.TrimSpace(h.ExternalLink)
	if !validHTTPURL(raw) {
		return ""
	}
	u, _ := url.Parse(raw)
	if techInAsiaShorteners[strings.ToLower(u.Hostname())] {
		return ""
	}
	return raw
}

func (h techInAsiaHit) toJob(linkCounts map[string]int) (Job, bool) {
	title := strings.Join(strings.Fields(h.Title), " ")
	company := strings.TrimSpace(h.Company.Name)
	if title == "" || company == "" {
		return Job{}, false
	}
	jobURL := techInAsiaPostingURL + url.PathEscape(h.ID)
	if l := h.externalLink(); l != "" && linkCounts[l] == 1 {
		jobURL = l
	}
	job := Job{
		ExternalID:     h.ID,
		URL:            jobURL,
		Title:          title,
		Company:        company,
		Description:    sanitizeHTML(h.Description),
		PostedAt:       techInAsiaPostedAt(h.PublishedAt),
		WorkMode:       techInAsiaWorkMode(h.WorkArrangement, h.IsRemote),
		EmploymentType: techInAsiaEmploymentType(h.JobType),
		IsTechHint:     h.Position != nil && techInAsiaTechPositions[h.Position.Name],
	}
	job.Remote = job.WorkMode == "remote"
	if h.City != nil {
		job.Location = joinNonEmpty(strings.TrimSpace(h.City.Name), strings.TrimSpace(h.City.CountryName))
		if c := location.NormalizeCountry(strings.TrimSpace(h.City.CountryName)); c != "" {
			job.Countries = []string{c}
		}
	}
	if job.Location == "" && job.Remote {
		job.Location = "Remote"
	}
	skills := make([]string, 0, len(h.JobSkills))
	for _, s := range h.JobSkills {
		skills = append(skills, s.Name)
	}
	job.Skills = skilltag.Canonicalize(skills)
	if h.ExperienceMin != nil && *h.ExperienceMin > 0 {
		years := *h.ExperienceMin
		job.ExperienceYearsMin = &years
	}
	h.salary(&job)
	return job, true
}

// salary takes the posting's published range. The platform states no period, but every observed
// amount is monthly. An all-zero range means "not stated".
func (h techInAsiaHit) salary(job *Job) {
	if h.IsSalaryVisible != 1 || h.Currency == nil {
		return
	}
	currency := strings.ToUpper(strings.TrimSpace(h.Currency.Code))
	lo, hi := roundSalaryPart(h.SalaryMin), roundSalaryPart(h.SalaryMax)
	if len(currency) != 3 || (lo == nil && hi == nil) {
		return
	}
	job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.SalaryPeriod = lo, hi, currency, "month"
}

// techInAsiaPostedAt reads the index's zone-less "2006-01-02 15:04:05" as UTC.
func techInAsiaPostedAt(s string) *time.Time {
	return parseLayout("2006-01-02 15:04:05", strings.TrimSpace(s))
}

func techInAsiaWorkMode(arrangement string, isRemote int) string {
	switch arrangement {
	case "on-site":
		return "onsite"
	case "hybrid":
		return "hybrid"
	case "remote":
		return "remote"
	}
	if isRemote == 1 {
		return "remote"
	}
	return ""
}

// techInAsiaEmploymentType maps the platform's five job types onto vocab.EmploymentTypeValues.
func techInAsiaEmploymentType(t *techInAsiaNamed) string {
	if t == nil {
		return ""
	}
	switch t.Name {
	case "Full-time":
		return "full_time"
	case "Part-time":
		return "part_time"
	case "Internship":
		return "internship"
	case "Contract", "Freelance":
		return "contract"
	default:
		return ""
	}
}
