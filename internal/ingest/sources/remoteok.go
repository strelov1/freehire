package sources

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	xhtml "golang.org/x/net/html"
)

// remoteokSource adapts remoteok.com, a remote-jobs aggregator. Boardless (one public
// site, no per-tenant board) and multi-company, so it stays in the source facet and takes
// each posting's company from the site.
//
// Two reads make up the crawl, because neither alone is complete (verified 2026-11-24):
//   - /api carries the newest 100 postings with their full HTML bodies, salary and location,
//     and nothing older (offset= is ignored; ?tag=X is a different 100-cap window per tag and
//     left 137 of 452 live postings unreachable after 14 tags).
//   - the listing the site's own "load more" calls, /?action=get_jobs&offset=N (50 rows a page,
//     an empty page past the end), names EVERY live posting (452 that day) with its id, company,
//     title, location, date and a plain-text body, but no salary that can be trusted ("$80k - $160k"
//     is RemoteOK's default placeholder, and the rest read "Upgrade to Premium").
//
// So a posting inside the API window is mapped from the API (best body, real salary), and the
// rest from the listing. cmd/liveness covers the closing side (the closed banner).
type remoteokSource struct {
	http interface {
		JSONGetter
		TextGetter
	}
}

const (
	remoteokListURL = "https://remoteok.com/api"
	remoteokPageURL = "https://remoteok.com/?action=get_jobs&offset=%d"
	remoteokOrigin  = "https://remoteOK.com"
	remoteokPerPage = 50
	// remoteokMaxPages bounds the walk; the site held 10 pages, so this is a 4x backstop.
	remoteokMaxPages = 40
)

// NewRemoteOK builds the RemoteOK adapter over the given HTTP client.
func NewRemoteOK(c interface {
	JSONGetter
	TextGetter
}) Source {
	return remoteokSource{http: c}
}

func (remoteokSource) Provider() string { return "remoteok" }

func (remoteokSource) Boardless() {}

func (remoteokSource) Aggregator() {}

// remoteok's feed carries only its newest postings; cmd/liveness reads the job page's closed banner.
func (remoteokSource) LivenessProbed() {}

// remoteokPosting is one posting from the /api feed. The feed's first element is a legal
// notice (no id/slug); it is skipped by the empty-id guard in toJob. Salary is annual USD
// and 0 when the poster stated none.
type remoteokPosting struct {
	ID          string  `json:"id"`
	Slug        string  `json:"slug"`
	Company     string  `json:"company"`
	Position    string  `json:"position"`
	Description string  `json:"description"`
	Location    string  `json:"location"`
	Date        string  `json:"date"`
	URL         string  `json:"url"`
	SalaryMin   float64 `json:"salary_min"`
	SalaryMax   float64 `json:"salary_max"`
}

// remoteokRow is one posting of the listing pages.
type remoteokRow struct {
	id, path, company, title, location, employmentType, body string
	posted                                                   *time.Time
}

func (s remoteokSource) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	jobs, _, err := s.crawl(ctx)
	return jobs, err
}

// FetchNew is the same crawl, but a posting outside the API window that the catalogue already
// holds is only re-listed: its stored body came from the API while it was new, and the
// listing's plain text would overwrite it with a poorer one.
func (s remoteokSource) FetchNew(ctx context.Context, _ CompanyEntry, seen func(string) bool) ([]Job, error) {
	jobs, fromAPI, err := s.crawl(ctx)
	if err != nil {
		return nil, err
	}
	for i, j := range jobs {
		if !fromAPI[j.ExternalID] && seen(j.ExternalID) {
			jobs[i] = Job{ExternalID: j.ExternalID, URL: j.URL, Title: j.Title, Company: j.Company, SeenRefresh: true}
		}
	}
	return jobs, nil
}

// crawl returns every live posting and the set of ids that were mapped from the API.
func (s remoteokSource) crawl(ctx context.Context) ([]Job, map[string]bool, error) {
	var feed []remoteokPosting
	if err := s.http.GetJSON(ctx, remoteokListURL, &feed); err != nil {
		return nil, nil, fmt.Errorf("remoteok: list: %w", err)
	}
	rows := map[string]remoteokRow{}
	var order []string
	for page := 0; page < remoteokMaxPages; page++ {
		txt, err := s.http.GetText(ctx, fmt.Sprintf(remoteokPageURL, page*remoteokPerPage))
		var doc *xhtml.Node
		if err == nil {
			// The endpoint answers bare <tr> rows; outside a <table> the parser would drop them.
			doc, err = xhtml.Parse(strings.NewReader("<table>" + txt + "</table>"))
		}
		if err != nil {
			// A half-read listing would make every posting past the gap look gone.
			return nil, nil, fmt.Errorf("remoteok: listing page %d: %w", page, err)
		}
		got := remoteokRows(doc)
		added := 0
		for _, r := range got {
			if _, dup := rows[r.id]; !dup {
				rows[r.id] = r
				order = append(order, r.id)
				added++
			}
		}
		if added == 0 {
			break
		}
	}

	jobs := make([]Job, 0, len(feed)+len(order))
	fromAPI := map[string]bool{}
	for _, p := range feed {
		if job, ok := p.toJob(rows[p.ID]); ok {
			jobs = append(jobs, job)
			fromAPI[job.ExternalID] = true
		}
	}
	for _, id := range order {
		if !fromAPI[id] {
			if job, ok := rows[id].toJob(); ok {
				jobs = append(jobs, job)
			}
		}
	}
	return jobs, fromAPI, nil
}

// remoteokRows reads the postings out of one listing page. A posting is two <tr>s sharing
// data-id: the job row (data-slug, company, title, location, tags) and the "expand" row that
// holds its body in div.html (HTML) or div.markdown (text).
func remoteokRows(doc *xhtml.Node) []remoteokRow {
	var out []remoteokRow
	idx := map[string]int{}
	walk(doc, func(n *xhtml.Node) bool {
		if n.Type != xhtml.ElementNode || n.Data != "tr" || Attr(n, "data-id") == "" {
			return true
		}
		id := Attr(n, "data-id")
		if HasClass(n, "expand") {
			if i, ok := idx[id]; ok {
				walk(n, func(c *xhtml.Node) bool {
					if c.Type != xhtml.ElementNode {
						return true
					}
					switch {
					case HasClass(c, "html"): // a poster's own HTML body
						out[i].body = sanitizeHTML(innerHTML(c))
						return false
					case HasClass(c, "markdown"): // the plain-text rendering of a markdown body
						out[i].body = remoteokBody(textContent(c))
						return false
					}
					return true
				})
			}
			return false
		}
		r := remoteokRow{id: id, path: Attr(n, "data-href"), company: Attr(n, "data-company")}
		var places []string
		walk(n, func(c *xhtml.Node) bool {
			if c.Type != xhtml.ElementNode {
				return true
			}
			switch {
			case c.Data == "h2" && Attr(c, "itemprop") == "title":
				r.title = strings.TrimSpace(textContent(c))
			case c.Data == "time" && r.posted == nil:
				r.posted = parseRFC3339(Attr(c, "datetime"))
			case c.Data == "div" && HasClass(c, "location"):
				txt := strings.TrimSpace(textContent(c))
				switch {
				case strings.HasPrefix(txt, "💰"):
				case strings.HasPrefix(txt, "⏰"):
					r.employmentType = remoteokEmployment(txt)
				default:
					if p := remoteokPlace(txt); p != "" {
						places = append(places, p)
					}
				}
			}
			return true
		})
		r.location = strings.Join(places, ", ")
		idx[id] = len(out)
		out = append(out, r)
		return false
	})
	return out
}

// remoteokPlace drops the flag/emoji RemoteOK prefixes a place with and reads its own guess
// ("Probably worldwide", given where the poster named nothing) as worldwide.
func remoteokPlace(s string) string {
	s = strings.TrimSpace(strings.TrimLeftFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }))
	if strings.EqualFold(s, "Probably worldwide") {
		return "Worldwide"
	}
	return s
}

// remoteokEmployment maps the "⏰ Contractor" / "⏰ Part time" / "⏰ Temporary" tag into
// vocab.EmploymentTypeValues; anything else states no type.
func remoteokEmployment(s string) string {
	switch strings.ToLower(strings.TrimSpace(strings.TrimPrefix(s, "⏰"))) {
	case "contract", "contractor", "temporary":
		return "contract"
	case "part time":
		return "part_time"
	case "full time":
		return "full_time"
	case "internship":
		return "internship"
	}
	return ""
}

// remoteokBody turns the listing's plain-text body into HTML: the site writes its newlines
// as the two characters `\n`, and a blank line separates paragraphs.
func remoteokBody(txt string) string {
	txt = strings.TrimSpace(strings.ReplaceAll(txt, `\n`, "\n"))
	var b strings.Builder
	for _, para := range strings.Split(txt, "\n\n") {
		if para = strings.TrimSpace(para); para != "" {
			b.WriteString("<p>" + strings.ReplaceAll(html.EscapeString(para), "\n", "<br>") + "</p>")
		}
	}
	return sanitizeHTML(b.String())
}

// remoteokUnmojibake repairs text the API serves UTF-8-decoded-as-Latin-1 ("MÃ¼nchen",
// "youâ\u0080\u0099ll"). A string that is not wholly Latin-1 or does not re-read as UTF-8 was
// not mangled that way and is returned as it is.
func remoteokUnmojibake(s string) string {
	if !strings.ContainsAny(s, "ÃÂâ") {
		return s
	}
	b := make([]byte, 0, len(s))
	for _, r := range s {
		if r > 0xFF {
			return s
		}
		b = append(b, byte(r)) //nolint:gosec // r <= 0xFF is checked above
	}
	if !utf8.Valid(b) {
		return s
	}
	return string(b)
}

// toJob maps an API posting to a Job, returning ok=false for the leading legal-notice
// element (no id) and any posting missing the company (which would break the slug). RemoteOK
// lists only remote jobs, so the work mode is always remote. row is the same posting from the
// listing (zero when it is not there) and fills what the API leaves empty or mangles.
func (p remoteokPosting) toJob(row remoteokRow) (Job, bool) {
	if p.ID == "" || p.Company == "" {
		return Job{}, false
	}
	loc := remoteokUnmojibake(p.Location)
	if loc == "" {
		loc = row.location
	}
	job := Job{
		ExternalID:     p.ID,
		URL:            p.URL,
		Title:          remoteokUnmojibake(p.Position),
		Company:        remoteokUnmojibake(p.Company),
		Location:       loc,
		Description:    sanitizeHTML(remoteokUnmojibake(p.Description)),
		Remote:         true,
		WorkMode:       "remote",
		EmploymentType: row.employmentType,
		PostedAt:       parseRFC3339(p.Date),
	}
	if p.SalaryMin > 0 || p.SalaryMax > 0 {
		job.SalaryMin, job.SalaryMax = roundSalaryPart(p.SalaryMin), roundSalaryPart(p.SalaryMax)
		job.SalaryCurrency, job.SalaryPeriod = "USD", "year"
	}
	return job, true
}

// toJob maps a listing row, the only source for a posting older than the API window.
func (r remoteokRow) toJob() (Job, bool) {
	if r.id == "" || r.company == "" || r.path == "" {
		return Job{}, false
	}
	if _, err := strconv.Atoi(r.id); err != nil {
		return Job{}, false
	}
	return Job{
		ExternalID:     r.id,
		URL:            remoteokOrigin + (&url.URL{Path: r.path}).EscapedPath(),
		Title:          r.title,
		Company:        r.company,
		Location:       r.location,
		Description:    r.body,
		Remote:         true,
		WorkMode:       "remote",
		EmploymentType: r.employmentType,
		PostedAt:       r.posted,
	}, true
}
