package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// trinethire adapts TriNet Hire (app.trinethire.com), the applicant tracker TriNet bundles with
// its US PEO. A board is the company's path segment, "<numeric id>-<name slug>"
// (app.trinethire.com/companies/16150-automox-inc/jobs), stored as the platform spells it in a
// posting link. Only the numeric id addresses the company — "/companies/16150/jobs" and
// "/companies/16150-anything/jobs" serve the same page (measured 2026-10-24) — so two spellings
// differing in the slug are one crawl target, and boardIdentity folds them onto the id.
//
// Both pages are server-rendered Rails HTML with no JSON behind them and no schema.org markup:
//   - the listing is ONE unpaginated table (the largest of 100 tenants held 30 rows), each row
//     carrying the title, the employment type and the tenant's own LOCATION NAME — a label the
//     employer chose ("Remote in the Midwest", "Automox Inc. HQ"), not an address;
//   - the posting page is the only source of the body, and adds a "City, ST" address, the
//     platform's experience picklist and, on about half the postings, a salary line.
//
// So this is a HydratingSource: a stored posting is re-listed as a SeenRefresh and costs no
// detail request. There is no posting date on either page. It does not claim fullBoardListing:
// nothing on the page states a total to check the table against.
type trinethire struct {
	http HTMLGetter
}

// NewTrinetHire builds the TriNet Hire adapter over the given HTTP client.
func NewTrinetHire(c HTMLGetter) Source { return trinethire{http: c} }

func (trinethire) Provider() string { return "trinethire" }

const trinethireBase = "https://app.trinethire.com"

// trinethireRow is one row of a company's listing table.
type trinethireRow struct {
	ID, URL, Title, Type, Label string
}

// Fetch reads a company's listing and every posting's page.
func (t trinethire) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	rows, err := t.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return fetchDetails(rows, defaultDetailWorkers, func(r trinethireRow) (Job, bool) {
		return t.detail(ctx, e, r)
	}), nil
}

// FetchNew is Fetch that spends a page request only on postings the catalogue does not hold; a
// seen posting is re-listed as a SeenRefresh, the listing itself proving it is still open.
func (t trinethire) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	rows, err := t.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return fetchDetails(rows, defaultDetailWorkers, func(r trinethireRow) (Job, bool) {
		if seen(r.ID) {
			return Job{ExternalID: r.ID, URL: r.URL, Title: r.Title, Company: e.Company, SeenRefresh: true}, true
		}
		return t.detail(ctx, e, r)
	}), nil
}

// list reads the company's listing table. An unknown company is a 404, which fails the board.
func (t trinethire) list(ctx context.Context, board string) ([]trinethireRow, error) {
	if !trinethireBoardPattern.MatchString(board) {
		return nil, fmt.Errorf("trinethire: board %q is not a <id>-<slug> company segment", board)
	}
	root, err := t.http.GetHTML(ctx, trinethireBase+"/companies/"+board+"/jobs")
	if err != nil {
		return nil, fmt.Errorf("trinethire: list company %s: %w", board, err)
	}
	return trinethireRows(root), nil
}

// trinethireBoardPattern is the company path segment: the numeric id, optionally followed by
// the name slug. It keeps a malformed catalogue row from being spliced into a path.
var trinethireBoardPattern = regexp.MustCompile(`^\d+(-[a-z0-9-]+)?$`)

// trinethireJobPath matches a posting link and captures its numeric id, which is global across
// the platform (the job page 404s under any other company) and is what the apply form posts.
var trinethireJobPath = regexp.MustCompile(`^/companies/[^/]+/jobs/(\d+)(?:-[^/]*)?$`)

// trinethireRows reads every <tr class="job"> of the listing: its link, title, type and the
// tenant's location label. A row without a posting link is skipped.
func trinethireRows(root *html.Node) []trinethireRow {
	var rows []trinethireRow
	seen := map[string]bool{}
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "tr" || !HasClass(n, "job") {
			return true
		}
		var r trinethireRow
		walk(n, func(c *html.Node) bool {
			if c.Type != html.ElementNode {
				return true
			}
			switch {
			case c.Data == "a" && r.ID == "":
				href := Attr(c, "href")
				if u, err := url.Parse(href); err == nil {
					if id := firstSubmatch(trinethireJobPath, u.Path); id != "" {
						r.ID, r.URL, r.Title = id, trinethireBase+u.Path, textContent(c)
					}
				}
			case c.Data == "td" && HasClass(c, "type"):
				r.Type = textContent(c)
			case c.Data == "td" && HasClass(c, "location"):
				r.Label = textContent(c)
			}
			return true
		})
		if r.ID != "" && !seen[r.ID] {
			seen[r.ID] = true
			rows = append(rows, r)
		}
		return false
	})
	return rows
}

// detail fetches one posting page and maps it, with the listing row, to a Job. A 404 is the
// platform's own "no such posting" and drops it; any other failure, or a page with no body, is
// an unreadableDetail marker — the page is the only source of the body.
func (t trinethire) detail(ctx context.Context, e CompanyEntry, r trinethireRow) (Job, bool) {
	root, err := t.http.GetHTML(ctx, r.URL)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(r.ID, r.URL, e.Company), true
		}
		return Job{}, false
	}
	body := firstByClass(root, "job-descr")
	if body == nil || textContent(body) == "" {
		return unreadableDetail(r.ID, r.URL, e.Company), true
	}
	meta := trinethireMeta(root)

	title := r.Title
	if h := firstByClass(root, "job-name"); h != nil && textContent(h) != "" {
		title = textContent(h)
	}
	empType := meta.fields["Type"]
	if empType == "" {
		empType = r.Type // the listing states the same picklist
	}
	location := trinethireLocation(r.Label, meta.address)

	j := Job{
		ExternalID:     r.ID,
		URL:            r.URL,
		Title:          title,
		Company:        e.Company,
		Location:       location,
		Description:    sanitizeHTML(innerHTML(body)),
		Remote:         isRemote(location),
		EmploymentType: trinethireEmploymentTypes[empType],
		Seniority:      trinethireSeniorities[meta.fields["Min. Experience"]],
	}
	j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod = trinethireSalary(meta.fields["Salary"])
	return j, true
}

// trinethirePageMeta is the posting page's <ul class="job-meta">: the address line (the item
// carrying the map-pin icon) and the "<strong>Name:</strong> value" items.
type trinethirePageMeta struct {
	address string
	fields  map[string]string
}

func trinethireMeta(root *html.Node) trinethirePageMeta {
	m := trinethirePageMeta{fields: map[string]string{}}
	ul := firstByClass(root, "job-meta")
	if ul == nil {
		return m
	}
	for li := ul.FirstChild; li != nil; li = li.NextSibling {
		if li.Type != html.ElementNode || li.Data != "li" {
			continue
		}
		if firstByClass(li, "map-pin-icon") != nil {
			m.address = strings.Join(strings.Fields(textContent(li)), " ")
			continue
		}
		label := firstByTag(li, "strong")
		if label == nil {
			continue
		}
		name := strings.TrimSuffix(strings.TrimSpace(textContent(label)), ":")
		value := strings.TrimPrefix(strings.Join(strings.Fields(textContent(li)), " "),
			strings.Join(strings.Fields(textContent(label)), " "))
		m.fields[name] = strings.TrimSpace(value)
	}
	return m
}

// trinethireLocation builds the location from the posting's address and the tenant's label.
//
// The address is a "City, ST" pair the employer fills in, and the form takes a wildcard: "*, NY"
// and "All, All" are how a remote posting says "anywhere in New York" and "anywhere", so a part
// that is only a wildcard is dropped rather than read as a city named "*". The label is the
// employer's own name for the location and is usually not a place ("Automox Inc. HQ", a
// recruiter's name), but it is where a remote posting says so — the address of "Remote in the
// Midwest" is "Chicago, IL". So the label contributes exactly one thing, the word "Remote", and
// only when the address does not already carry it.
func trinethireLocation(label, address string) string {
	var parts []string
	for _, p := range strings.Split(address, ",") {
		p = strings.TrimSpace(p)
		if p == "" || p == "*" || strings.EqualFold(p, "all") {
			continue
		}
		parts = append(parts, p)
	}
	loc := strings.Join(parts, ", ")
	if isRemote(label) && !isRemote(loc) {
		return joinNonEmpty("Remote", loc)
	}
	return loc
}

// trinethireEmploymentTypes maps the platform's closed "Type" picklist (the same five values on
// the listing and the posting page across 300 sampled postings).
var trinethireEmploymentTypes = map[string]string{
	"Full-time":  "full_time",
	"Part-time":  "part_time",
	"Contractor": "contract",
	"Temporary":  "contract",
	"Intern":     "internship",
}

// trinethireSeniorities maps the "Min. Experience" picklist where a value names a seniority.
// "Some Experience", "Manager", "Director", "Executive" and "Senior Executive" are left to the
// title dictionary: the first names no level, and the rest are a role's rank rather than the
// individual-contributor ladder freehire's seniority describes.
var trinethireSeniorities = map[string]string{
	"Intern":       "intern",
	"Entry Level":  "junior",
	"Senior Level": "senior",
}

// trinethireSalaryAmount is one dollar figure of the salary line ("$125,000", "$45.00").
var trinethireSalaryAmount = regexp.MustCompile(`\$([0-9][0-9,]*(?:\.[0-9]+)?)`)

// trinethireSalary reads the posting page's salary line, which the platform renders from a
// structured pay field: "$90,000 - $120,000", "$165,000", or "$45.00 - $50.00 per hour". Of 300
// postings sampled the only unit the platform ever printed was "per hour"; a line without one is
// the annual pay type. That reading is checked rather than trusted: two postings typed an hourly
// figure ("$23 - $25") with no unit, so an annual figure under 1,000 is dropped rather than
// published as a $25 salary. The currency is the dollar sign the line prints — TriNet's PEO
// serves US employers.
func trinethireSalary(line string) (lo, hi *int, currency, period string) {
	if line == "" {
		return nil, nil, "", ""
	}
	var amounts []float64
	for _, m := range trinethireSalaryAmount.FindAllStringSubmatch(line, 2) {
		v, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64)
		if err != nil {
			return nil, nil, "", ""
		}
		amounts = append(amounts, v)
	}
	if len(amounts) == 0 {
		return nil, nil, "", ""
	}
	period = "year"
	if strings.Contains(strings.ToLower(line), "per hour") {
		period = "hour"
	}
	if period == "year" && slices.Min(amounts) < 1000 {
		return nil, nil, "", ""
	}
	lo = roundSalaryPart(amounts[0])
	hi = lo
	if len(amounts) == 2 {
		hi = roundSalaryPart(amounts[1])
	}
	return lo, hi, "USD", period
}

// trinethireCompanyID folds a board onto the numeric company id that alone addresses it, so
// "16150-automox-inc" and "16150-automox" are recognised as one crawl target.
func trinethireCompanyID(board string) string {
	id, _, _ := strings.Cut(board, "-")
	return id
}
