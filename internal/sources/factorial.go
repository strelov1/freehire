package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// factorial adapts Factorial (FactorialHR) ATS career sites. A board is the careers host
// (e.g. "muffin.factorial.it" or "highbras.factorialhr.com.br"); the host's TLD varies by
// the tenant's country, so it travels with the board id. The careers root server-renders the
// full job list as "/job_posting/<slug>-<id>" links; each job page is server-rendered HTML
// carrying the title (h1), the body (a div.styledText block), and an unlabeled metadata list,
// so the description comes from a per-job detail fetch (bounded-concurrency) like the other
// HTML detail adapters. The site exposes no ld+json or JSON API.
type factorial struct {
	http HTMLGetter
}

// NewFactorial builds the Factorial adapter over the given HTTP client.
func NewFactorial(c HTMLGetter) Source { return factorial{http: c} }

func (factorial) Provider() string { return "factorial" }

// factorialIDPattern captures the native posting id from a job URL's trailing "-<id>"
// segment (e.g. /job_posting/partnership-success-manager-305055 → 305055).
var factorialIDPattern = regexp.MustCompile(`-(\d+)/?$`)

func (f factorial) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	base, err := url.Parse(fmt.Sprintf("https://%s/", e.Board))
	if err != nil {
		return nil, fmt.Errorf("factorial: board %q: %w", e.Board, err)
	}
	root, err := f.http.GetHTML(ctx, base.String())
	if err != nil {
		return nil, fmt.Errorf("factorial: listing %s: %w", e.Board, err)
	}

	// The careers root renders every posting; dedup the links (the same job can appear under
	// multiple team sections).
	var urls []string
	seen := make(map[string]bool)
	for _, link := range jobLinks(base, root, func(href string) bool { return strings.Contains(href, "/job_posting/") }) {
		if !seen[link] {
			seen[link] = true
			urls = append(urls, link)
		}
	}

	return fetchDetails(urls, defaultDetailWorkers, func(u string) (Job, bool) {
		return f.detail(ctx, e, u)
	}), nil
}

// detail fetches one job page and maps it to a Job, returning ok=false when the page fetch
// fails or carries no parseable id/title, so the caller skips just that posting.
func (f factorial) detail(ctx context.Context, e CompanyEntry, jobURL string) (Job, bool) {
	m := factorialIDPattern.FindStringSubmatch(jobURL)
	if m == nil {
		return Job{}, false // no native id → would collide on the dedup key; skip it
	}
	root, err := f.http.GetHTML(ctx, jobURL)
	if err != nil {
		return Job{}, false
	}
	title := firstElementText(root, "h1")
	if title == "" {
		return Job{}, false
	}

	location, workMode := factorialLocation(metadataRows(root))
	return Job{
		ExternalID:  m[1],
		URL:         jobURL,
		Title:       title,
		Company:     e.Company,
		Location:    location,
		Description: sanitizeHTML(elementInnerHTMLByClass(root, "div", "styledText")),
		Remote:      workMode == "remote" || isRemote(location),
		WorkMode:    workMode,
		PostedAt:    nil, // Factorial job pages carry no publish date
	}, true
}

// factorialParenLoc matches the "<work-mode> (<places>)" location form Factorial renders in
// some locales (e.g. "Ibrido (Milano, Lombardia, Italia)"); the parenthetical must contain a
// comma so a team row like "S - Partnerships (PAR)" does not match.
var factorialParenLoc = regexp.MustCompile(`^(.+?)\s*\(([^()]*,[^()]*)\)\s*$`)

// factorialLocation picks the location out of a job's unlabeled metadata rows (contract,
// schedule, salary, location, team — in no guaranteed order). The location is the row that
// names places: it carries a comma and no currency symbol (which would make it the salary).
// It returns the geographic text and, when the row prefixes a work-mode word, the mapped
// WorkMode.
func factorialLocation(rows []string) (location, workMode string) {
	for _, row := range rows {
		row = strings.TrimSpace(row)
		if row == "" || strings.ContainsAny(row, "€$£₹¥") { // skip blanks and the salary row
			continue
		}
		if m := factorialParenLoc.FindStringSubmatch(row); m != nil {
			return strings.TrimSpace(m[2]), factorialWorkMode(m[1])
		}
		if strings.Contains(row, ",") { // plain "City, Region, Country"
			return row, ""
		}
	}
	return "", ""
}

// factorialWorkMode maps Factorial's localized work-mode label (Italian/Spanish/Portuguese/
// English) to our vocabulary; an unknown label yields "" (the pipeline then parses the
// location text).
func factorialWorkMode(label string) string {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "ibrido", "híbrido", "hibrido", "hybrid", "hibride":
		return "hybrid"
	case "da remoto", "remoto", "remota", "remote", "en remoto", "teletrabajo":
		return "remote"
	case "in sede", "presencial", "presenziale", "on-site", "on site", "onsite", "no escritório", "no local":
		return "onsite"
	default:
		return ""
	}
}

// firstElementText returns the trimmed text of the first element with the given tag, or "".
func firstElementText(root *html.Node, tag string) string {
	var out string
	walk(root, func(n *html.Node) bool {
		if out != "" {
			return false
		}
		if n.Type == html.ElementNode && n.Data == tag {
			out = textContent(n)
			return false
		}
		return true
	})
	return out
}

// elementInnerHTMLByClass returns the inner HTML of the first element with the given tag that
// carries the given class token, or "".
func elementInnerHTMLByClass(root *html.Node, tag, class string) string {
	var out string
	found := false
	walk(root, func(n *html.Node) bool {
		if found {
			return false
		}
		if n.Type == html.ElementNode && n.Data == tag && hasClass(n, class) {
			out = innerHTML(n)
			found = true
			return false
		}
		return true
	})
	return out
}

// metadataRows returns the text of the job-detail metadata list items — the icon rows that
// hold contract/schedule/salary/location/team. They share the "border-gray-50" divider
// class, which distinguishes them from the nav and description list items.
func metadataRows(root *html.Node) []string {
	var rows []string
	walk(root, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "li" && hasClass(n, "border-gray-50") {
			rows = append(rows, textContent(n))
		}
		return true
	})
	return rows
}
