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

// zoho adapts Zoho Recruit career sites. A board is the careers host (e.g.
// "lithan.zohorecruit.com" or "talcom.zohorecruit.eu"); the TLD varies by the tenant's data
// region. The careers page embeds the whole opening list as an HTML-escaped JSON array in a
// hidden <input id="jobs">, so the listing is one fetch with no API; that array carries no
// body, so the description comes from a per-posting detail page that embeds the record as a
// JS-escaped blob (bounded-concurrency, like the other detail adapters).
type zoho struct {
	http HTMLGetter
}

// NewZoho builds the Zoho Recruit adapter over the given HTTP client.
func NewZoho(c HTMLGetter) Source { return zoho{http: c} }

func (zoho) Provider() string { return "zohorecruit" }

// zohoOpening is one record from the careers page's embedded #jobs array. City/Country are
// often null (decoding to ""); Remote_Job is the structured work-mode signal.
type zohoOpening struct {
	ID           string `json:"id"`
	PostingTitle string `json:"Posting_Title"`
	City         string `json:"City"`
	Country      string `json:"Country"`
	RemoteJob    bool   `json:"Remote_Job"`
	Publish      bool   `json:"Publish"`
}

func (z zoho) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	base := "https://" + e.Board
	root, err := z.http.GetHTML(ctx, base+"/jobs/Careers")
	if err != nil {
		return nil, fmt.Errorf("zoho: listing %s: %w", e.Board, err)
	}
	raw := elementAttrByID(root, "input", "jobs", "value")
	if raw == "" {
		return nil, fmt.Errorf("zoho: %s: no #jobs data", e.Board)
	}
	var openings []zohoOpening
	if err := json.Unmarshal([]byte(raw), &openings); err != nil {
		return nil, fmt.Errorf("zoho: %s: decode openings: %w", e.Board, err)
	}

	return fetchDetails(openings, defaultDetailWorkers, func(o zohoOpening) (Job, bool) {
		return z.toJob(ctx, base, e, o)
	}), nil
}

// toJob maps an opening to a Job, fetching the full description from its detail page. A
// posting that is unpublished, or whose detail carries no description, is dropped.
func (z zoho) toJob(ctx context.Context, base string, e CompanyEntry, o zohoOpening) (Job, bool) {
	if !o.Publish || o.ID == "" {
		return Job{}, false
	}
	url := fmt.Sprintf("%s/jobs/Careers/%s", base, o.ID)
	desc, ok := z.description(ctx, url)
	if !ok {
		return Job{}, false
	}
	location := joinNonEmpty(o.City, o.Country)
	return Job{
		ExternalID:  o.ID,
		URL:         url,
		Title:       o.PostingTitle,
		Company:     e.Company,
		Location:    location,
		Description: desc,
		Remote:      o.RemoteJob || isRemote(location),
		WorkMode:    workModeFromRemote(o.RemoteJob),
		PostedAt:    nil, // the embedded record carries no publish date
	}, true
}

// zohoDescPattern captures a detail record's Job_Description value out of the JS-escaped blob
// the page embeds: the value runs to the next field separator (an escaped `","` followed by
// the next field's name). The page escapes quotes as \x22, so the markers are literal \x22.
var zohoDescPattern = regexp.MustCompile(`\\x22Job_Description\\x22:\\x22(.*?)\\x22,\\x22[A-Za-z_]`)

// description fetches a detail page and returns the posting's sanitized body, or ok=false
// when the fetch fails or the page carries no Job_Description.
func (z zoho) description(ctx context.Context, url string) (string, bool) {
	root, err := z.http.GetHTML(ctx, url)
	if err != nil {
		return "", false
	}
	m := zohoDescPattern.FindStringSubmatch(textContent(root))
	if m == nil {
		return "", false
	}
	body := sanitizeHTML(html.UnescapeString(zohoUnescape(m[1])))
	if body == "" {
		return "", false
	}
	return body, true
}

// zohoHexEscape matches a \xNN JS hex escape, which Zoho uses for every non-alphanumeric
// byte of the embedded record (quotes, angle brackets, …).
var zohoHexEscape = regexp.MustCompile(`\\x[0-9a-fA-F]{2}`)

// zohoUnescape decodes the JS string escaping of an embedded Zoho value: \xNN hex escapes to
// their byte first (turning \x22 into a quote, \x3C into '<', …), then the standard
// backslash escapes. The result is HTML, which the caller sanitizes.
func zohoUnescape(s string) string {
	s = zohoHexEscape.ReplaceAllStringFunc(s, func(m string) string {
		b, err := strconv.ParseUint(m[2:], 16, 8)
		if err != nil {
			return m
		}
		return string(rune(b))
	})
	return strings.NewReplacer(
		`\/`, `/`,
		`\n`, "\n",
		`\t`, "\t",
		`\r`, "",
		`\"`, `"`,
		`\\`, `\`,
	).Replace(s)
}

// elementAttrByID returns the named attribute of the first element with the given tag and id,
// or "". The HTML parser decodes character references in the attribute value, so a value that
// embeds an &#34;-escaped JSON array comes back as plain JSON.
func elementAttrByID(root *html.Node, tag, id, name string) string {
	var out string
	found := false
	walk(root, func(n *html.Node) bool {
		if found {
			return false
		}
		if n.Type == html.ElementNode && n.Data == tag && attr(n, "id") == id {
			out = attr(n, name)
			found = true
			return false
		}
		return true
	})
	return out
}
