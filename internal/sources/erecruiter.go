package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// erecruiter adapts eRecruiter Polska career boards ("Strona Kariera"). The board is the
// company's cfg config token (a public URL embeds it as skk.erecruiter.pl/Code.ashx?cfg=<hex>).
// The keyless list endpoint returns a JSONP-wrapped HTML row table with only a summary per
// posting, so the adapter fetches each posting's Offer.aspx detail page for its description.
type erecruiterHTTP interface {
	TextGetter
	HTMLGetter
}

type erecruiter struct {
	http erecruiterHTTP
}

// NewErecruiter builds the eRecruiter adapter over the given HTTP client.
func NewErecruiter(c erecruiterHTTP) Source { return erecruiter{http: c} }

func (erecruiter) Provider() string { return "erecruiter" }

const (
	erecruiterListURL   = "https://skk.erecruiter.pl/GetHtml.ashx?cfg=%s&grid=rows&pn=%d"
	erecruiterDetailURL = "https://skk.erecruiter.pl/Offer.aspx?oid=%s&cfg=%s&ejoId=%s&ejorId=%s&comId=%s"
	// erecruiterMaxPages bounds the walk so a feed that never reaches its reported total can
	// never loop forever. It sits far above any real company board's posting count.
	erecruiterMaxPages = 500
)

// erecruiterRowData is one list row's identifiers and summary cells.
type erecruiterRowData struct {
	offerID, ejoID, ejorID, comID string
	title, city                   string
}

func (s erecruiter) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	rows, err := s.listRows(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	// Each offer's description lives on its own Offer.aspx page; fetch them through the shared
	// bounded worker pool like the other detail-HTML adapters.
	return fetchDetails(rows, defaultDetailWorkers, func(r erecruiterRowData) (Job, bool) {
		return s.toJob(ctx, r, e)
	}), nil
}

// listRows pages the board's list endpoint and returns every offer row. It stops when it has
// collected the reported total, when a page carries no offer rows, or when a page fails to
// advance (repeats the previous page's first row — a board that ignores the pn parameter),
// all under a page cap so a pathological feed cannot loop forever.
func (s erecruiter) listRows(ctx context.Context, board string) ([]erecruiterRowData, error) {
	var rows []erecruiterRowData
	total, prevFirst := 0, ""
	for page := 1; page <= erecruiterMaxPages; page++ {
		body, err := s.http.GetText(ctx, fmt.Sprintf(erecruiterListURL, board, page))
		if err != nil {
			return nil, fmt.Errorf("erecruiter: list %s pn %d: %w", board, page, err)
		}
		pageRows, pageTotal, err := parseErecruiterRows(body)
		if err != nil {
			return nil, fmt.Errorf("erecruiter: parse %s pn %d: %w", board, page, err)
		}
		if pageTotal > 0 {
			total = pageTotal
		}
		if len(pageRows) == 0 || pageRows[0].offerID == prevFirst {
			break
		}
		prevFirst = pageRows[0].offerID
		rows = append(rows, pageRows...)
		// Stop once the reported total is collected; an over-reported total is bounded by the
		// empty/non-advancing-page break above and the page cap.
		if total > 0 && len(rows) >= total {
			break
		}
	}
	return rows, nil
}

// toJob fetches a row's detail page and maps it to a Job, returning ok=false when the row
// lacks the offer id needed to address the detail and dedup, when the detail page is gone (a
// closed posting), or when it carries no description.
//
// ExternalID is the offerId, not the externalJobOfferId: one posting spread over several
// cities shares an externalJobOfferId across its per-city rows (each a distinct offerId with
// its own detail page and location), so keying on externalJobOfferId would collapse the
// city variants into a single job under the (source, external_id) dedup key.
func (s erecruiter) toJob(ctx context.Context, r erecruiterRowData, e CompanyEntry) (Job, bool) {
	if r.offerID == "" {
		return Job{}, false
	}
	detailURL := fmt.Sprintf(erecruiterDetailURL, r.offerID, e.Board, r.ejoID, r.ejorID, r.comID)
	doc, err := s.http.GetHTML(ctx, detailURL)
	if err != nil {
		return Job{}, false
	}
	desc := erecruiterDescription(doc)
	if desc == "" {
		return Job{}, false
	}
	return Job{
		ExternalID:  r.offerID,
		URL:         detailURL,
		Title:       r.title,
		Company:     e.Company,
		Location:    r.city,
		Description: sanitizeHTML(desc),
	}, true
}

// parseErecruiterRows unwraps the JSONP list body (`({"htm":"<tr...>"});`), parses its row
// table, and returns the offer rows plus the board's total result count (carried on a hidden
// marker row's `tr` attribute). The htm fragment is wrapped in <table> so the HTML parser
// keeps the <tr>/<td> structure instead of foster-parenting it away.
func parseErecruiterRows(body string) ([]erecruiterRowData, int, error) {
	start := strings.IndexByte(body, '{')
	end := strings.LastIndexByte(body, '}')
	if start < 0 || end < start {
		return nil, 0, fmt.Errorf("no JSON object in JSONP body")
	}
	var env struct {
		Htm string `json:"htm"`
	}
	if err := json.Unmarshal([]byte(body[start:end+1]), &env); err != nil {
		return nil, 0, err
	}
	doc, err := html.Parse(strings.NewReader("<table>" + env.Htm + "</table>"))
	if err != nil {
		return nil, 0, err
	}

	var rows []erecruiterRowData
	total := 0
	walk(doc, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "tr" {
			return true
		}
		// The hidden marker row carries the board's total on its `tr` attribute (the HTML
		// parser lowercases attribute names, so the offer ids read lowercase below).
		if v := attr(n, "tr"); v != "" {
			if t, convErr := strconv.Atoi(v); convErr == nil {
				total = t
			}
		}
		offerID := attr(n, "offerid")
		if offerID == "" {
			return true // marker/non-offer row
		}
		// The cells are, in order, position / category / city; the first is the title and the
		// last is the location.
		row := erecruiterRowData{
			offerID: offerID,
			ejoID:   attr(n, "externaljobofferid"),
			ejorID:  attr(n, "externaljobofferregionid"),
			comID:   attr(n, "comid"),
		}
		if cells := erecruiterCells(n); len(cells) > 0 {
			row.title = cells[0]
			row.city = cells[len(cells)-1]
		}
		rows = append(rows, row)
		return false // rows don't nest; skip descending into the cells
	})
	return rows, total, nil
}

// erecruiterCells returns the trimmed text of each <td> in a row, in document order.
func erecruiterCells(tr *html.Node) []string {
	var cells []string
	walk(tr, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "td" {
			cells = append(cells, textContent(n))
		}
		return true
	})
	return cells
}

// erecruiterSkipIDs are the offer-container sections that are not the job body: the header
// (title/workplace, already carried as structured Job fields) and the GDPR consent clause.
var erecruiterSkipIDs = map[string]bool{"JobTitle": true, "WorkPlace": true, "Clause": true}

// erecruiterDescription returns the detail page's job body as HTML. Company career-page
// templates vary (some render the body in a <div id="t1">, others in id="opis"/"description"/
// …), so rather than target specific blocks it takes the whole offer container (<div
// id="offCont">) minus the header and consent-clause sections. Returns "" when the container
// is absent, so the posting is dropped.
func erecruiterDescription(doc *html.Node) string {
	root := elementByID(doc, "offCont")
	if root == nil {
		return ""
	}
	var drop []*html.Node
	walk(root, func(n *html.Node) bool {
		if n.Type == html.ElementNode && erecruiterSkipIDs[attr(n, "id")] {
			drop = append(drop, n)
			return false
		}
		return true
	})
	for _, n := range drop {
		n.Parent.RemoveChild(n)
	}
	return innerHTML(root)
}
