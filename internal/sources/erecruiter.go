package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// erecruiter adapts eRecruiter Polska public per-company career boards ("Strona Kariera")
// on skk.erecruiter.pl. The board is the company's cfg (a 32-hex config token embedded in
// its careers page). The keyless list endpoint returns a JSONP body ({"htm":"<tr…>"}) of
// offer rows carrying ids and a city cell but no description, so the adapter fetches each
// offer's Offer.aspx detail page for the body. Boards are paged; page 1's hidden marker row
// reports the total, giving a clean stop condition.
type erecruiter struct {
	http erecruiterHTTP
}

// erecruiterHTTP is the transport erecruiter needs: GetText for the JSONP list, GetHTML for
// each per-offer detail page.
type erecruiterHTTP interface {
	TextGetter
	HTMLGetter
}

// NewErecruiter builds the eRecruiter adapter over the given HTTP client.
func NewErecruiter(c erecruiterHTTP) Source { return erecruiter{http: c} }

func (erecruiter) Provider() string { return "erecruiter" }

const (
	erecruiterBase = "https://skk.erecruiter.pl"
	// erecruiterMaxPages bounds the walk so a feed that never reports its total (or keeps
	// returning full pages) cannot loop forever; it sits far above any real board's size.
	erecruiterMaxPages = 500
)

// erecruiterRow is one offer row parsed from the list JSONP. offerID is the per-publication
// id (unique per city variant, and the oid the detail URL is keyed by) used as the dedup
// ExternalID; externalJobOfferID is shared across a role's multi-city variants, so it is only
// carried for the detail URL, not used as identity.
type erecruiterRow struct {
	offerID  string
	extID    string
	regionID string
	comID    string
	title    string // list position cell — a fallback for the detail's #JobTitle
	city     string // list city cell — a fallback for the detail's #WorkPlace
}

func (e erecruiter) Fetch(ctx context.Context, entry CompanyEntry) ([]Job, error) {
	var jobs []Job
	total, seen := -1, 0 // total is unknown until page 1's marker row
	for page := 1; page <= erecruiterMaxPages; page++ {
		url := fmt.Sprintf("%s/GetHtml.ashx?cfg=%s&grid=rows&pn=%d", erecruiterBase, entry.Board, page)
		body, err := e.http.GetText(ctx, url)
		if err != nil {
			return nil, fmt.Errorf("erecruiter: list %s page %d: %w", entry.Board, page, err)
		}
		rows, pageTotal, err := parseErecruiterRows(body)
		if err != nil {
			return nil, fmt.Errorf("erecruiter: parse list %s page %d: %w", entry.Board, page, err)
		}
		if total < 0 && pageTotal >= 0 {
			total = pageTotal // page 1's marker carries the grand total; later pages repeat it
		}
		if len(rows) == 0 {
			break // ran out of offers (an empty page or a marker-only past-the-end page)
		}
		for _, r := range rows {
			if j, ok := e.toJob(ctx, r, entry); ok {
				jobs = append(jobs, j)
			}
		}
		if seen += len(rows); total >= 0 && seen >= total {
			break
		}
	}
	return jobs, nil
}

// toJob fetches a row's Offer.aspx detail and maps it to a Job, returning ok=false when the
// detail is unreachable or lacks a title/description (a dead or closed offer) — such a
// posting is skipped without aborting the board.
func (e erecruiter) toJob(ctx context.Context, r erecruiterRow, entry CompanyEntry) (Job, bool) {
	url := fmt.Sprintf("%s/Offer.aspx?oid=%s&cfg=%s&ejoId=%s&ejorId=%s&comId=%s",
		erecruiterBase, r.offerID, entry.Board, r.extID, r.regionID, r.comID)
	root, err := e.http.GetHTML(ctx, url)
	if err != nil {
		return Job{}, false
	}

	title, location, description := parseErecruiterDetail(root)
	if title == "" {
		title = r.title
	}
	if location == "" {
		location = r.city
	}
	if title == "" || description == "" {
		return Job{}, false
	}

	return Job{
		ExternalID:  r.offerID,
		URL:         url,
		Title:       title,
		Company:     entry.Company,
		Location:    location,
		Description: description,
	}, true
}

// parseErecruiterRows unwraps the ({"htm":"…"}) JSONP list body and parses its offer rows,
// returning the rows plus the total result count from the hidden marker row (tr attribute),
// or -1 when this page carries no marker.
func parseErecruiterRows(body string) ([]erecruiterRow, int, error) {
	s := strings.TrimSpace(body)
	s = strings.TrimSpace(strings.TrimSuffix(s, ";")) // an optional trailing ; after the )
	s = strings.TrimPrefix(s, "(")
	s = strings.TrimSuffix(s, ")")
	if s == "" {
		return nil, -1, nil // a blank body ends the walk gracefully, not a parse error
	}
	var env struct {
		Htm string `json:"htm"`
	}
	if err := json.Unmarshal([]byte(s), &env); err != nil {
		return nil, -1, fmt.Errorf("unwrap jsonp: %w", err)
	}

	// The rows are bare <tr> fragments; wrap them in a <table> so the HTML parser keeps
	// them (a <tr> outside a table is otherwise dropped).
	doc, err := html.Parse(strings.NewReader("<table>" + env.Htm + "</table>"))
	if err != nil {
		return nil, -1, fmt.Errorf("parse rows: %w", err)
	}

	var rows []erecruiterRow
	total := -1
	walk(doc, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "tr" {
			return true
		}
		// The marker row (a non-offer <tr>) carries the total in its tr attribute.
		if attr(n, "skkresult") != "offer" {
			if t := attr(n, "tr"); t != "" {
				if v, err := strconv.Atoi(t); err == nil {
					total = v
				}
			}
			return true
		}
		if r, ok := erecruiterOfferRow(n); ok {
			rows = append(rows, r)
		}
		return true
	})
	return rows, total, nil
}

// erecruiterOfferRow reads one offer <tr>'s ids and cells (position, city). It returns
// ok=false when the row has no offerId — without it there is no dedup key or detail URL.
// The HTML parser lower-cases attribute names, so the camelCase source attributes are read
// in lower case.
func erecruiterOfferRow(tr *html.Node) (erecruiterRow, bool) {
	r := erecruiterRow{
		offerID:  attr(tr, "offerid"),
		extID:    attr(tr, "externaljobofferid"),
		regionID: attr(tr, "externaljobofferregionid"),
		comID:    attr(tr, "comid"),
	}
	if r.offerID == "" {
		return erecruiterRow{}, false
	}

	var cells []*html.Node
	walk(tr, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "td" {
			cells = append(cells, n)
		}
		return true
	})
	for _, td := range cells {
		if hasClass(td, "skk_positionName") {
			r.title = textContent(td)
		}
	}
	if len(cells) > 0 {
		r.city = textContent(cells[len(cells)-1]) // the city is always the last cell
	}
	return r, true
}

// parseErecruiterDetail reads an Offer.aspx detail tree: the #JobTitle heading, the
// #WorkPlace line (stripping its "Miejsce pracy:" label), and the body assembled from the
// #t1 / #Opportunities / #CompanyDescription sections (sanitized). A field absent from the
// markup comes back empty for the caller to fall back on the list row or skip.
func parseErecruiterDetail(root *html.Node) (title, location, description string) {
	title = erecruiterIDText(root, "JobTitle")
	location = strings.TrimSpace(erecruiterIDText(root, "WorkPlace"))
	// The line is labelled ("Miejsce pracy: <city>", or "Workplace: <city>" in the en locale);
	// keep only the value after the label. A label-less value (no colon) is used as-is.
	if _, city, ok := strings.Cut(location, ":"); ok {
		location = strings.TrimSpace(city)
	}

	var body strings.Builder
	for _, id := range []string{"t1", "Opportunities", "CompanyDescription"} {
		if n := elementByID(root, id); n != nil {
			body.WriteString(innerHTML(n))
		}
	}
	return title, location, sanitizeHTML(body.String())
}

// erecruiterIDText returns the trimmed text of the element with the given id, or "".
func erecruiterIDText(root *html.Node, id string) string {
	if n := elementByID(root, id); n != nil {
		return textContent(n)
	}
	return ""
}

// erecruiterCfgRe matches the cfg board token in the eRecruiter career-widget script tag
// (<script src="https://skk.erecruiter.pl/Code.ashx?cfg=<32-hex>">) a company embeds on its
// careers page.
var erecruiterCfgRe = regexp.MustCompile(`(?i)Code\.ashx\?cfg=([0-9A-Fa-f]{32})`)

// ExtractErecruiterCfg returns the cfg board token embedded in a company careers page's
// eRecruiter widget script, or "" when the page carries no such widget. It backs
// cmd/harvest-erecruiter, the discovery bridge from a careers URL to a board entry.
func ExtractErecruiterCfg(page string) string {
	m := erecruiterCfgRe.FindStringSubmatch(page)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// ProbeErecruiterCfg live-validates a cfg by fetching its first list page and reporting how
// many offer rows it carries; 0 means the token resolves to no public board. The harvester
// keeps a discovered cfg only when this is > 0.
func ProbeErecruiterCfg(ctx context.Context, http TextGetter, cfg string) (int, error) {
	url := fmt.Sprintf("%s/GetHtml.ashx?cfg=%s&grid=rows&pn=1", erecruiterBase, cfg)
	body, err := http.GetText(ctx, url)
	if err != nil {
		return 0, err
	}
	rows, _, err := parseErecruiterRows(body)
	if err != nil {
		return 0, err
	}
	return len(rows), nil
}
