package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"math"
	"regexp"
	"strings"
	"sync/atomic"
	"unicode"

	"github.com/strelov1/freehire/internal/dict/location"
)

// torre adapts Torre (torre.ai), a Latin-American-born talent marketplace whose postings each name
// their own employer. The provider is boardless: the site's sitemap index names the whole live
// catalogue (English and Spanish, many sitemaps) and is pruned as postings close. Every posting
// page carries a schema.org JobPosting, so this is a HydratingSource that buys a page only for a
// posting the catalogue lacks.
//
// The search API the site once used (search.torre.co) now refuses every request, and the job
// search moved into the app under /app, which robots.txt disallows; the sitemaps and /post/
// pages are what robots.txt leaves open. See the Torre traps in AGENTS.md.
type torre struct {
	http torreHTTP
}

type torreHTTP interface {
	XMLGetter
	HTMLGetter
}

const torreSitemapIndexURL = "https://torre.ai/sitemap-index.xml"

// torrePostRE is a posting page: /post/<id>, optionally followed by a slug of the employer and
// title. Only the id is the external id; some sitemap entries carry no slug at all.
var torrePostRE = regexp.MustCompile(`^https://torre\.ai/post/([A-Za-z0-9]+)(?:-[^/?#]*)?$`)

func NewTorre(c torreHTTP) Source { return torre{http: c} }

func (torre) Provider() string { return "torre" }
func (torre) Boardless()       {}
func (torre) Aggregator()      {}

type torreListing struct {
	ID  string
	URL string
}

func (s torre) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, CompanyEntry{}, func(string) bool { return false })
}

func (s torre) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	list, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, list, seen)
}

// list reads the sitemap index and EVERY sitemap it names. One failing sitemap fails the run:
// the sitemaps are independent shards of one catalogue, so a skipped shard would read as its
// postings closing and the sweep would close them.
func (s torre) list(ctx context.Context) ([]torreListing, error) {
	index, err := getSitemap(ctx, s.http, torreSitemapIndexURL)
	if err != nil {
		return nil, fmt.Errorf("torre: sitemap index: %w", err)
	}
	var out []torreListing
	seen := map[string]bool{}
	for _, sm := range index.Sitemaps {
		doc, err := getSitemap(ctx, s.http, strings.TrimSpace(sm.Loc))
		switch {
		case err != nil && !detailUnreadable(err):
			// The index also names sitemaps that answer 404 (sitemap--1.xml, sitemap--2.xml):
			// the platform's own answer that the shard holds nothing.
			log.Printf("torre: sitemap %s is gone; skipping: %v", sm.Loc, err)
			continue
		case err != nil:
			return nil, fmt.Errorf("torre: sitemap %s: %w", sm.Loc, err)
		}
		for _, u := range doc.URLs {
			loc := strings.TrimSpace(u.Loc)
			m := torrePostRE.FindStringSubmatch(loc)
			if m == nil || seen[m[1]] {
				continue
			}
			seen[m[1]] = true
			out = append(out, torreListing{ID: m[1], URL: loc})
		}
	}
	// The marketplace always carries postings; none at all means the sitemaps moved.
	if len(out) == 0 {
		return nil, fmt.Errorf("torre: the sitemaps name no posting pages")
	}
	return out, nil
}

// hydrate buys a posting page only for a posting the catalogue lacks. A page that could not be
// read, or carries no usable JobPosting, defers that posting to a later run rather than storing
// it body-less (a stored row is seen and would never be hydrated again).
func (s torre) hydrate(ctx context.Context, list []torreListing, seen func(string) bool) ([]Job, error) {
	var wanted, read atomic.Int64
	jobs := fetchDetails(list, defaultDetailWorkers, func(l torreListing) (Job, bool) {
		if seen(l.ID) {
			return Job{ExternalID: l.ID, URL: l.URL, SeenRefresh: true}, true
		}
		wanted.Add(1)
		root, err := s.http.GetHTML(ctx, l.URL)
		if err != nil {
			if detailUnreadable(err) {
				log.Printf("torre: detail %s failed; deferring: %v", l.URL, err)
			}
			return Job{}, false
		}
		var p torrePosting
		if !ldJobPosting(root, &p) {
			log.Printf("torre: detail %s carries no JobPosting; deferring", l.URL)
			return Job{}, false
		}
		job, ok := p.toJob(l)
		if !ok {
			log.Printf("torre: detail %s carries no usable JobPosting; deferring", l.URL)
			return Job{}, false
		}
		read.Add(1)
		return job, true
	})
	// A crawl that listed new postings and read none of them is broken, not quiet.
	if wanted.Load() > 0 && read.Load() == 0 {
		return nil, fmt.Errorf("torre: listed %d new postings and read none of their pages", wanted.Load())
	}
	return jobs, nil
}

// torrePosting is the subset of the posting page's schema.org JobPosting the adapter maps.
type torrePosting struct {
	Title              string          `json:"title"`
	Description        string          `json:"description"`
	DatePosted         string          `json:"datePosted"`
	EmploymentType     json.RawMessage `json:"employmentType"`
	JobLocationTyp     string          `json:"jobLocationType"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation []struct {
		Address struct {
			StreetAddress   string `json:"streetAddress"`
			AddressLocality string `json:"addressLocality"`
			AddressRegion   string `json:"addressRegion"`
			AddressCountry  string `json:"addressCountry"`
		} `json:"address"`
	} `json:"jobLocation"`
	BaseSalary *struct {
		Currency string `json:"currency"`
		Value    struct {
			MinValue *float64 `json:"minValue"`
			MaxValue *float64 `json:"maxValue"`
			UnitText string   `json:"unitText"`
		} `json:"value"`
	} `json:"baseSalary"`
}

func (p torrePosting) toJob(l torreListing) (Job, bool) {
	title := strings.Join(strings.Fields(p.Title), " ")
	company := strings.Join(strings.Fields(p.HiringOrganization.Name), " ")
	description := sanitizeHTML(torreUnescape(p.Description))
	if title == "" || company == "" || strings.TrimSpace(textFromHTML(description)) == "" {
		return Job{}, false
	}
	job := Job{
		ExternalID:     l.ID,
		URL:            l.URL,
		Title:          title,
		Company:        company,
		Description:    description,
		PostedAt:       NotFuture(parseRFC3339(strings.TrimSpace(p.DatePosted))),
		EmploymentType: torreEmploymentType(p.EmploymentType),
	}
	p.place(&job)
	p.salary(&job)
	return job, true
}

// place maps the posting's places. A remote posting (TELECOMMUTE) lists the countries it hires
// from as its places; one with no place at all is the platform's "Remote - Anywhere", whose
// applicantLocationRequirements is a fixed 49-country filler and is not read. An on-site
// posting's place is an address whose streetAddress is sometimes the city and sometimes a
// street, so it is used only when it carries no digit.
func (p torrePosting) place(job *Job) {
	var parts, countries []string
	seenCountry := map[string]bool{}
	for _, pl := range p.JobLocation {
		a := pl.Address
		country := strings.TrimSpace(a.AddressCountry)
		if c := location.NormalizeCountry(country); c != "" && !seenCountry[c] {
			seenCountry[c] = true
			countries = append(countries, c)
		}
		city := strings.TrimSpace(a.AddressLocality)
		if street := strings.TrimSpace(a.StreetAddress); city == "" && !strings.ContainsFunc(street, unicode.IsDigit) {
			city = street
		}
		parts = append(parts, distinctJoin([]string{city, a.AddressRegion, country}, ", ", strings.TrimSpace))
	}
	where := distinctJoin(parts, "; ", strings.TrimSpace)
	if p.JobLocationTyp == "TELECOMMUTE" {
		job.Remote, job.WorkMode = true, "remote"
		if where == "" {
			where = "Worldwide"
		}
		where = "Remote, " + where
	}
	job.Location = where
	job.Countries = countries
}

var torreSalaryPeriods = map[string]string{
	"HOUR":  "hour",
	"DAY":   "day",
	"MONTH": "month",
	"YEAR":  "year",
}

// salary takes the JobPosting's range unless it is the placeholder a hidden salary carries:
// exactly 1 to 1000, whatever the employer's real (hidden) figures are. A maxValue of 0 states
// no upper bound.
func (p torrePosting) salary(job *Job) {
	b := p.BaseSalary
	if b == nil {
		return
	}
	period := torreSalaryPeriods[strings.ToUpper(strings.TrimSpace(b.Value.UnitText))]
	currency := strings.ToUpper(strings.TrimSpace(b.Currency))
	lo, hi := torreBound(b.Value.MinValue), torreBound(b.Value.MaxValue)
	if lo != nil && hi != nil && *lo == 1 && *hi == 1000 {
		return
	}
	if period == "" || len(currency) != 3 || lo == nil {
		return
	}
	job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.SalaryPeriod = lo, hi, currency, period
}

func torreBound(v *float64) *int {
	if v == nil || *v <= 0 {
		return nil
	}
	n := int(math.Round(*v))
	return &n
}

// torreEmploymentType maps the first schema.org employmentType the posting states onto
// vocab.EmploymentTypeValues. The field is a list ("CONTRACTOR", "PART_TIME").
func torreEmploymentType(raw json.RawMessage) string {
	var many []string
	if json.Unmarshal(raw, &many) != nil {
		var one string
		if json.Unmarshal(raw, &one) != nil {
			return ""
		}
		many = []string{one}
	}
	for _, t := range many {
		switch strings.ToUpper(strings.TrimSpace(t)) {
		case "FULL_TIME":
			return "full_time"
		case "PART_TIME":
			return "part_time"
		case "CONTRACTOR", "TEMPORARY":
			return "contract"
		case "INTERN":
			return "internship"
		}
	}
	return ""
}

// torreUnescape undoes the second string escaping Torre applies to the JobPosting description: once
// JSON-decoded it still carries \uXXXX for every non-ASCII character, \/ for a slash and \t for a
// tab. Decoding it again as a JSON string literal restores the text; a description that does not
// decode is kept as it is.
//
// A posting Torre crawled from elsewhere can also carry its markup entity-escaped inside a
// paragraph ("<p>&lt;p&gt;Blue Recruitment Personnel is recruiting…"), which would render as
// literal tags; that one level of entity escaping is undone too.
func torreUnescape(s string) string {
	if strings.Contains(s, `\`) {
		var out string
		if json.Unmarshal([]byte(`"`+s+`"`), &out) == nil {
			s = out
		}
	}
	if strings.Contains(s, "&lt;p&gt;") || strings.Contains(s, "&lt;br") {
		s = html.UnescapeString(s)
	}
	return s
}
