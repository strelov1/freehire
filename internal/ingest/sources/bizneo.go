package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// bizneo adapts the career sites of Bizneo HR, a Spanish HR suite whose ATS publishes each
// customer's openings on a host under bizneo.cloud — usually <company>.careers.ats.bizneo.cloud,
// sometimes a shorter vanity label (jobs.abeienergy.bizneo.cloud, sivsa.bizneo.cloud). The board
// is that whole host, since the label alone does not say which of the two shapes it lives under.
//
// The site is server-rendered:
//
//	/jobs?page=N    one page of job cards (title, category, city, work mode), ~10 a page
//	/jobs/<slug>    one posting
//
// A page past the last one renders no card, which ends the walk. The posting page carries the
// title, the body in one or more .general-content blocks, and a side card of labelled fields
// (Ubicación, Modalidad de trabajo, Jornada laboral …) whose labels sit in the title attribute.
// Some sites also emit a schema.org JobPosting, the only place a posting's date appears; most do
// not (measured 2026-10-27: 1 of 33 live sites), so the page itself is what the adapter reads and
// the JobPosting only adds to it.
//
// The posting slug is the identity: the card links to nothing else, so it is the only id a
// HydratingSource can test before buying the page. Most slugs end in a UUID; a few are a bare
// title slug, which Bizneo keeps stable for the posting's life.
type bizneo struct {
	http HTMLGetter
}

// NewBizneo builds the Bizneo adapter over the given HTML client.
func NewBizneo(c HTMLGetter) Source { return bizneo{http: c} }

func (bizneo) Provider() string { return "bizneo" }

// bizneoMaxPages caps the ?page=N walk; the largest site seen (somos-grupo-mas) has 19 pages.
const bizneoMaxPages = 100

// bizneoBoard is a career-site host: one or more labels under bizneo.cloud.
var bizneoBoard = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+bizneo\.cloud$`)

// bizneoSlug captures a posting's slug from its page URL.
var bizneoSlug = regexp.MustCompile(`/jobs/([A-Za-z0-9][A-Za-z0-9_.~%-]*)/?$`)

// bizneoCard is one job card of the listing.
type bizneoCard struct {
	slug, url, title, city, mode string
}

// Fetch reads the listing and every posting's page.
func (s bizneo) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	cards, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return fetchDetails(cards, defaultDetailWorkers, func(c bizneoCard) (Job, bool) {
		return s.detail(ctx, e, c)
	}), nil
}

// FetchNew is Fetch that spends a page request only on postings the catalogue does not hold; a
// seen posting is re-listed as a SeenRefresh, its card proving it is still open.
func (s bizneo) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	cards, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return fetchDetails(cards, defaultDetailWorkers, func(c bizneoCard) (Job, bool) {
		if seen(c.slug) {
			return Job{ExternalID: c.slug, URL: c.url, Title: c.title, Company: e.Company, SeenRefresh: true}, true
		}
		return s.detail(ctx, e, c)
	}), nil
}

// list walks the listing pages until one adds no card. The first page failing is a board-level
// error; a later page failing ends the walk with the cards gathered so far.
func (s bizneo) list(ctx context.Context, rawBoard string) ([]bizneoCard, error) {
	board := strings.ToLower(strings.TrimSpace(rawBoard))
	if !bizneoBoard.MatchString(board) {
		return nil, fmt.Errorf("bizneo: board %q is not a bizneo.cloud career-site host", rawBoard)
	}
	base := &url.URL{Scheme: "https", Host: board, Path: "/jobs"}
	var cards []bizneoCard
	seen := map[string]bool{}
	for page := 1; page <= bizneoMaxPages; page++ {
		root, err := s.http.GetHTML(ctx, fmt.Sprintf("https://%s/jobs?page=%d", board, page))
		if err != nil {
			if page == 1 {
				return nil, fmt.Errorf("bizneo: board %s listing: %w", board, err)
			}
			break
		}
		added := 0
		for _, c := range bizneoCards(base, root) {
			if !seen[c.slug] {
				seen[c.slug] = true
				cards = append(cards, c)
				added++
			}
		}
		if added == 0 {
			break
		}
	}
	return cards, nil
}

// bizneoCards reads the job cards of one listing page: each is an a.job-card linking to the
// posting, with the title in .title and the card's details (category, city, work mode) as the
// spans of .details.
func bizneoCards(base *url.URL, root *html.Node) []bizneoCard {
	var out []bizneoCard
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "a" || !HasClass(n, "job-card") {
			return true
		}
		ref, err := base.Parse(strings.TrimSpace(Attr(n, "href")))
		if err != nil || ref.Host != base.Host {
			return false
		}
		slug := firstSubmatch(bizneoSlug, ref.Path)
		if slug == "" {
			return false
		}
		ref.RawQuery, ref.Fragment = "", ""
		c := bizneoCard{slug: slug, url: ref.String()}
		if t := firstByClass(n, "title"); t != nil {
			c.title = bizneoText(textContent(t))
		}
		if d := firstByClass(n, "details"); d != nil {
			for sp := d.FirstChild; sp != nil; sp = sp.NextSibling {
				if sp.Type != html.ElementNode || sp.Data != "span" || HasClass(sp, "aggregation") {
					continue
				}
				v := bizneoText(textContent(sp))
				if m := bizneoWorkMode(v); m != "" {
					c.mode = m
				} else if c.city == "" {
					c.city = v
				}
			}
		}
		out = append(out, c)
		return false
	})
	return out
}

// bizneoPosting is the schema.org JobPosting a few sites emit beside the page.
type bizneoPosting struct {
	DatePosted     string       `json:"datePosted"`
	EmploymentType string       `json:"employmentType"`
	JobLocation    schemaPlaces `json:"jobLocation"`
}

// detail fetches one posting's page and maps it, with its card, to a Job. A 404 is the site's own
// "no such posting" and drops it; any other failure, or a page without a body, is an unreadable
// marker — the page is the only source of the body.
func (s bizneo) detail(ctx context.Context, e CompanyEntry, c bizneoCard) (Job, bool) {
	root, err := s.http.GetHTML(ctx, c.url)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(c.slug, c.url, e.Company), true
		}
		return Job{}, false
	}
	var body []string
	walk(root, func(n *html.Node) bool {
		if n.Type == html.ElementNode && HasClass(n, "general-content") {
			body = append(body, innerHTML(n))
			return false
		}
		return true
	})
	if len(body) == 0 {
		return unreadableDetail(c.slug, c.url, e.Company), true
	}
	fields := bizneoFields(root)
	title := c.title
	if h := firstByTag(root, "h1"); h != nil {
		title = firstNonEmpty(bizneoText(textContent(h)), title)
	}
	mode := firstNonEmpty(bizneoWorkMode(fields["Modalidad de trabajo"]), bizneoWorkMode(fields["Modalidade de trabalho"]),
		bizneoWorkMode(fields["Modalità di lavoro"]), bizneoWorkMode(fields["Work modality"]), c.mode)
	j := Job{
		ExternalID:     c.slug,
		URL:            c.url,
		Title:          title,
		Company:        e.Company,
		Location:       firstNonEmpty(fields["Ubicación"], fields["Localização"], fields["Località"], fields["Location"], c.city),
		Description:    sanitizeHTML(strings.Join(body, "\n")),
		WorkMode:       mode,
		Remote:         mode == "remote",
		EmploymentType: bizneoEmploymentType(firstNonEmpty(fields["Jornada laboral"], fields["Jornada de trabalho"])),
	}
	var p bizneoPosting
	if ldJobPosting(root, &p) {
		j.PostedAt = parseRFC3339(p.DatePosted)
		if j.EmploymentType == "" {
			j.EmploymentType = schemaEmploymentType(p.EmploymentType)
		}
		var codes []string
		for _, pl := range p.JobLocation {
			codes = append(codes, pl.Address.AddressCountry)
		}
		j.Countries = countriesFromCodes(codes)
	}
	return j, true
}

// bizneoFields reads the posting's side card: each field is a div whose title attribute names it
// ("Ubicación") and whose span holds the value; a field with several values joins them.
func bizneoFields(root *html.Node) map[string]string {
	out := map[string]string{}
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "div" {
			return true
		}
		label := strings.TrimSpace(Attr(n, "title"))
		if label == "" {
			return true
		}
		var vals []string
		walk(n, func(m *html.Node) bool {
			if m.Type == html.ElementNode && m.Data == "span" {
				if v := bizneoText(textContent(m)); v != "" {
					vals = append(vals, v)
				}
				return false
			}
			return true
		})
		if len(vals) > 0 {
			out[label] = strings.Join(vals, ", ")
		}
		return false
	})
	return out
}

// bizneoWorkMode maps Bizneo's work-mode label, in the site's language, to our vocabulary; any
// other text yields "".
func bizneoWorkMode(label string) string {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "teletrabajo/remoto", "teletrabajo", "remoto", "remote", "teletrabalho/remoto", "teletrabalho", "telelavoro/remoto", "da remoto":
		return "remote"
	case "híbrido", "hibrido", "hybrid", "ibrido", "mixto":
		return "hybrid"
	case "presencial", "on-site", "onsite", "in sede", "presenziale":
		return "onsite"
	}
	return ""
}

// bizneoEmploymentType maps the "Jornada laboral" field. "Indiferente" and anything else says
// nothing about the arrangement.
func bizneoEmploymentType(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "completa", "jornada completa", "tiempo completo", "integral", "tempo integral", "full time":
		return "full_time"
	case "parcial", "jornada parcial", "media jornada", "tiempo parcial", "meio período", "part time":
		return "part_time"
	}
	return ""
}

// bizneoText collapses the whitespace the templates indent their text with.
func bizneoText(s string) string { return strings.Join(strings.Fields(s), " ") }
