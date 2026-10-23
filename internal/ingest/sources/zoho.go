package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

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
	// The listing JSON is embedded in a hidden <input id="jobs" value="…">; the HTML parser
	// decodes the &#34;-escaped JSON in the value attribute back to plain JSON. Ids are unique,
	// so the id alone locates the input.
	raw := ""
	if n := firstByID(root, "jobs"); n != nil {
		raw = Attr(n, "value")
	}
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
	d, ok := z.detail(ctx, url)
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
		Description: d.body,
		Remote:      o.RemoteJob || isRemote(location),
		WorkMode:    workModeFromRemote(o.RemoteJob),
		PostedAt:    d.postedAt,
	}, true
}

// zohoDescPattern captures a detail record's Job_Description value out of the JS-escaped blob
// the page embeds: the value runs to the next field separator (an escaped `","` followed by
// the next field's name). The page escapes quotes as \x22, so the markers are literal \x22.
var zohoDescPattern = regexp.MustCompile(`\\x22Job_Description\\x22:\\x22(.*?)\\x22,\\x22[A-Za-z_]`)

// zohoOpenedPattern captures the same record's Date_Opened, a plain date the blob writes with
// escaped hyphens ("2024\-11\-11"). The listing's #jobs array carries it too, but only on the
// tenants that publish the field there (8 of 15 sampled 2026-10-23 did not); the detail record
// carried it on 9 of the 10 checked, those 8 included, and the detail page is fetched for the
// body anyway. A tenant that does not use the field at all leaves the posting undated.
var zohoOpenedPattern = regexp.MustCompile(`\\x22Date_Opened\\x22:\\x22([0-9\\-]+)\\x22`)

// zohoDetail is what a posting's detail page contributes to its Job.
type zohoDetail struct {
	body     string
	postedAt *time.Time
}

// detail fetches a detail page and returns the posting's sanitized body and its opening date,
// or ok=false when the fetch fails or the page carries no Job_Description. A missing or
// unparseable Date_Opened only leaves the posting undated.
func (z zoho) detail(ctx context.Context, url string) (zohoDetail, bool) {
	root, err := z.http.GetHTML(ctx, url)
	if err != nil {
		return zohoDetail{}, false
	}
	text := textContent(root)
	m := zohoDescPattern.FindStringSubmatch(text)
	if m == nil {
		return zohoDetail{}, false
	}
	body := sanitizeHTML(html.UnescapeString(zohoUnescape(m[1])))
	if body == "" {
		return zohoDetail{}, false
	}
	d := zohoDetail{body: body}
	if m := zohoOpenedPattern.FindStringSubmatch(text); m != nil {
		d.postedAt = parseDate(zohoUnescape(m[1]))
	}
	return d, true
}

// zohoUnescape decodes the JS string escaping of an embedded Zoho value into HTML, which the
// caller sanitizes. The record is serialized into the page as a JS string whose richtext field
// values are themselves already escaped, so markup arrives escaped twice: a closing tag's slash
// as \\\/ (an escaped backslash followed by an escaped slash) and a bullet as \\u2022. A single
// pass would peel only the outer layer, leaving a stray backslash that makes <\/span> an invalid
// tag the sanitizer emits as visible &lt;\/span&gt; text. We therefore decode one level per pass
// and repeat until the string stabilizes; this terminates because every changing pass removes at
// least one (finite) backslash, and leaves single-escaped values (which stabilize after one pass)
// untouched.
func zohoUnescape(s string) string {
	for range zohoMaxUnescapePasses {
		next := zohoUnescapeOnce(s)
		if next == s {
			break
		}
		s = next
	}
	return s
}

// zohoMaxUnescapePasses caps the decode loop as a guard; the observed nesting is two levels, so
// this is slack, not a functional bound.
const zohoMaxUnescapePasses = 8

// zohoUnescapeOnce decodes one level of JS string escaping in a single left-to-right pass, so an
// escaped backslash (\\) is consumed as one unit before the following character is read — the
// ordering that lets \\\/ collapse to / across two passes. \xNN and \uNNNN decode to their rune;
// \n\t\r\"\/ are the standard escapes; an unknown \X yields X (matching JS, and folding Zoho's
// stray \- back to -).
func zohoUnescapeOnce(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if s[i] != '\\' || i+1 >= len(s) {
			b.WriteByte(s[i])
			i++
			continue
		}
		switch c := s[i+1]; c {
		case '\\':
			b.WriteByte('\\')
			i += 2
		case '/':
			b.WriteByte('/')
			i += 2
		case 'n':
			b.WriteByte('\n')
			i += 2
		case 't':
			b.WriteByte('\t')
			i += 2
		case 'r':
			i += 2
		case '"':
			b.WriteByte('"')
			i += 2
		case 'x':
			if i+4 <= len(s) {
				if v, err := strconv.ParseUint(s[i+2:i+4], 16, 8); err == nil {
					b.WriteRune(rune(v))
					i += 4
					continue
				}
			}
			b.WriteByte(c)
			i += 2
		case 'u':
			if i+6 <= len(s) {
				if v, err := strconv.ParseUint(s[i+2:i+6], 16, 16); err == nil {
					b.WriteRune(rune(v))
					i += 6
					continue
				}
			}
			b.WriteByte(c)
			i += 2
		default:
			b.WriteByte(c)
			i += 2
		}
	}
	return b.String()
}
