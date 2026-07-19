package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// paylocity adapts recruiting.paylocity.com career sites. The board is the company GUID
// (e.g. "1a06dc72-45ee-4c90-a268-fe881bbeb577"). The listing GET
// /Recruiting/Jobs/All/<guid> is server-rendered HTML embedding a `window.pageData` blob
// whose Jobs[] array enumerates the company's open postings (id, title, location, date);
// the full description comes from a per-job detail fetch of /Recruiting/Jobs/Details/<id>,
// whose schema.org JobPosting ld+json carries the body — the listing-plus-ld+json-detail
// shape shared with careerspage/icims.
type paylocity struct {
	http paylocityHTTP
}

// paylocityHTTP is the client capability paylocity needs: the listing as raw text (to slice
// the embedded window.pageData blob) and each detail page as parsed HTML (for its ld+json).
type paylocityHTTP interface {
	TextGetter
	HTMLGetter
}

const paylocityBase = "https://recruiting.paylocity.com"

// NewPaylocity builds the recruiting.paylocity.com adapter over the given client.
func NewPaylocity(c paylocityHTTP) Source { return paylocity{http: c} }

func (paylocity) Provider() string { return "paylocity" }

func (s paylocity) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	listURL := fmt.Sprintf("%s/Recruiting/Jobs/All/%s", paylocityBase, e.Board)
	body, err := s.http.GetText(ctx, listURL)
	if err != nil {
		return nil, fmt.Errorf("paylocity: listing %q: %w", e.Board, err)
	}
	// The company's postings ride in window.pageData's Jobs[] array; slice the balanced
	// array out of the page and decode it. A board with no openings still renders the
	// page with an empty array.
	raw, ok := bracketSlice(body, `"Jobs":`, '[', ']')
	if !ok {
		return nil, fmt.Errorf("paylocity: board %q: no Jobs array in listing", e.Board)
	}
	var briefs []paylocityBrief
	if err := json.Unmarshal([]byte(raw), &briefs); err != nil {
		return nil, fmt.Errorf("paylocity: board %q: decode Jobs: %w", e.Board, err)
	}

	// Each posting's description comes from its own detail fetch, fanned out under a
	// bounded pool like the other JSON-LD detail adapters.
	return fetchDetails(briefs, defaultDetailWorkers, func(b paylocityBrief) (Job, bool) {
		return s.detail(ctx, e, b)
	}), nil
}

// detail builds a Job from a listing brief, enriching it with the description from the
// posting's detail page. The listing brief is authoritative for the posting's existence and
// structured fields (id/title/location/date), so a failed detail fetch only costs the
// description, not the whole posting. ok=false only for a brief with no id (which would
// collide on the dedup key).
func (s paylocity) detail(ctx context.Context, e CompanyEntry, b paylocityBrief) (Job, bool) {
	if b.JobID == 0 {
		return Job{}, false
	}
	detailURL := fmt.Sprintf("%s/Recruiting/Jobs/Details/%d", paylocityBase, b.JobID)

	description, company := "", e.Company
	if root, err := s.http.GetHTML(ctx, detailURL); err == nil {
		description = paylocityDescription(root)
		if description == "" {
			// Legacy tenants may still serve the schema.org ld+json the current page dropped.
			var p paylocityPosting
			if ldJobPosting(root, &p) {
				description = html.UnescapeString(p.Description)
				company = firstNonEmpty(e.Company, p.HiringOrganization.Name)
			}
		}
		description = sanitizeHTML(description)
	}

	return Job{
		ExternalID:  strconv.Itoa(b.JobID),
		URL:         detailURL,
		Title:       b.JobTitle,
		Company:     company,
		Location:    b.LocationName,
		Description: description,
		Remote:      b.IsRemote || isRemote(b.LocationName),
		PostedAt:    parseRFC3339(b.PublishedDate),
	}, true
}

// paylocityDescription extracts the posting body from a detail page. Paylocity's current
// pages are a client-rendered shell with no ld+json, but the description still renders
// server-side as the <div> immediately following the "Description" section header
// (<div class="job-listing-header">Description</div>). Returns "" when the block is absent.
func paylocityDescription(root *html.Node) string {
	var out string
	walk(root, func(n *html.Node) bool {
		if out != "" {
			return false
		}
		if n.Type == html.ElementNode && n.Data == "div" && hasClass(n, "job-listing-header") &&
			strings.EqualFold(textContent(n), "Description") {
			for sib := n.NextSibling; sib != nil; sib = sib.NextSibling {
				if sib.Type == html.ElementNode {
					out = strings.TrimSpace(innerHTML(sib))
					break
				}
			}
			return false
		}
		return true
	})
	return out
}

// paylocityBrief is one entry of the listing's window.pageData Jobs[] array.
type paylocityBrief struct {
	JobID         int    `json:"JobId"`
	JobTitle      string `json:"JobTitle"`
	LocationName  string `json:"LocationName"`
	PublishedDate string `json:"PublishedDate"`
	IsRemote      bool   `json:"IsRemote"`
}

// paylocityPosting is the schema.org JobPosting decoded from a detail page's ld+json.
type paylocityPosting struct {
	Description        string `json:"description"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
}
