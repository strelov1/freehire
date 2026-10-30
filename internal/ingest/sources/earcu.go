package sources

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// earcu adapts an eArcu career site. eArcu is a multi-tenant UK ATS whose clients each run
// on their own careers host and publish a keyless RSS feed at /jobs/rss. The feed names every
// open posting, but what it says about one is thin: the body is a ~1,000-character preview
// ending in "..." (measured 2026-10-30 on 16 tenants, every non-empty body truncated), and the
// "Key: value, …" metadata prefix is whatever fields the tenant chose — Cambridge's carries a
// Country and a Location, most carry none ("Region: , Business Area: …"), and some fill
// "Region" with a person's name. The posting itself is the detail page: its schema.org
// JobPosting states jobLocation with a structured addressCountry, and its visible description
// block is the whole body. So the adapter is a HydratingSource — a new posting costs one
// detail request, a stored one is a SeenRefresh. Unlike Personio the board is the full
// careers host, not a subdomain.
//
// earcuHTTP is the transport earcu needs: an RSS feed plus HTML detail pages.
type earcuHTTP interface {
	XMLGetter
	HTMLGetter
}

type earcu struct {
	http earcuHTTP
}

// NewEarcu builds the eArcu adapter over the given HTTP client.
func NewEarcu(c earcuHTTP) Source { return earcu{http: c} }

func (earcu) Provider() string { return "earcu" }

// earcuRSS is a board's /jobs/rss document. Only the fields the adapter maps are decoded.
type earcuRSS struct {
	Items []earcuItem `xml:"channel>item"`
}

type earcuItem struct {
	Link        string `xml:"link"`
	Title       string `xml:"title"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
}

// earcuIDRe extracts the stable posting id from a vacancy detail URL of the form
// /jobs/vacancy/<slug>/<id>/description/. Anchored on the final /description so an
// earlier numeric path segment can't be mistaken for the id.
var earcuIDRe = regexp.MustCompile(`/(\d+)/description/?$`)

// earcuRefRe matches the trailing " (<ref>)" eArcu reference suffix in a feed title.
var earcuRefRe = regexp.MustCompile(`\s*\(\d+\)\s*$`)

// earcuFieldRe finds where each "Key: " of the metadata prefix starts. A key opens the prefix
// or follows ", ", starts with a letter and holds no comma or colon, so a comma inside
// a value ("£38,300- £49,775", "London, UK") or a colon inside one ("2026-11-15T00:00:00")
// never starts a field. Tenants name their own fields, so nothing anchors on a fixed
// successor — the old parser required ", eArcu Vacancy Reference:" after Location, which only
// Cambridge's feed carries.
var earcuFieldRe = regexp.MustCompile(`(?:^|,\s+)([A-Za-z][^,:]*):\s?`)

func (a earcu) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return a.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew lists the board's feed and hydrates only the postings the catalogue does not hold.
func (a earcu) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	url := fmt.Sprintf("https://%s/jobs/rss", e.Board)

	var feed earcuRSS
	if err := a.http.GetXML(ctx, url, &feed); err != nil {
		return nil, fmt.Errorf("earcu: fetch board %s: %w", e.Board, err)
	}

	jobs := make([]Job, 0, len(feed.Items))
	for _, it := range feed.Items {
		m := earcuIDRe.FindStringSubmatch(it.Link)
		if m == nil {
			continue // no stable posting id — skip rather than emit an empty key
		}
		job := Job{
			ExternalID: m[1],
			URL:        it.Link,
			Title:      strings.TrimSpace(earcuRefRe.ReplaceAllString(it.Title, "")),
			Company:    e.Company,
		}
		if seen(job.ExternalID) {
			job.SeenRefresh = true
			jobs = append(jobs, job)
			continue
		}
		prefix, body := earcuSplit(it.Description)
		job.Location = earcuLocation(prefix)
		job.Countries = countryFromCode(earcuField(prefix, "Country"))
		job.Description = sanitizeHTML(body)
		job.PostedAt = earcuPubDate(it.PubDate)
		a.hydrate(ctx, &job)
		job.Remote = isRemote(job.Location) // the platform has no structured remote flag
		jobs = append(jobs, job)
	}
	return jobs, nil
}

// earcuPosting is the subset of the detail page's schema.org JobPosting the adapter maps.
type earcuPosting struct {
	Description string       `json:"description"`
	JobLocation schemaPlaces `json:"jobLocation"`
}

// hydrate overlays what the posting's detail page states on the feed's reading. A failed
// fetch is non-fatal: the feed names the posting, so it is kept with the feed's preview body
// and whatever place the prefix gave.
func (a earcu) hydrate(ctx context.Context, job *Job) {
	root, err := a.http.GetHTML(ctx, job.URL)
	if err != nil {
		return
	}
	var ld earcuPosting // stays zero when the page renders no JobPosting
	ldJobPosting(root, &ld)

	if body := earcuDetailBody(root); body != "" {
		job.Description = body
	} else if ld.Description != "" {
		job.Description = sanitizeHTML(html.UnescapeString(ld.Description))
	}

	loc, countries := earcuPlaces(ld.JobLocation)
	if loc == "" {
		loc = earcuSummaryLocation(root)
	}
	if loc != "" {
		job.Location = loc
	}
	if countries != nil {
		job.Countries = countries
	}
}

// earcuDetailBody returns the detail page's visible posting body, sanitized; "" when the page
// renders neither block. PosDescriptionText is the whole posting — the tenant's intro, the
// description container and its benefits (Mitie); some templates render only the container
// (DX), others only the wrapper (Cromwell). The JobPosting description is not preferred: it is
// sometimes only the first paragraph (Mills & Reeve: 167 characters of a ~5k body), and some
// tenants render no JobPosting at all (Cromwell).
func earcuDetailBody(root *html.Node) string {
	for _, class := range []string{"PosDescriptionText", "earcu_posdescriptionContainer"} {
		if body := sanitizeHTML(elementInnerHTMLByClass(root, "div", class)); body != "" {
			return body
		}
	}
	return ""
}

// earcuPlaces renders the JobPosting's addresses as "Locality, Region, Country" entries joined
// by "; ", with the countries they state. eArcu often repeats the locality as the region
// ("Nairobi, Nairobi", "Home Based, Home Based"), so a part equal to an earlier one is dropped.
// Kennedys emits an empty PostalAddress, which yields nothing.
func earcuPlaces(places schemaPlaces) (string, []string) {
	var locs, codes []string
	for _, pl := range places {
		var parts []string
		for _, p := range []string{pl.Address.AddressLocality, pl.Address.AddressRegion, pl.Address.AddressCountry} {
			p = strings.TrimSpace(p)
			if p != "" && !containsFold(parts, p) {
				parts = append(parts, p)
			}
		}
		if loc := strings.Join(parts, ", "); loc != "" && !containsFold(locs, loc) {
			locs = append(locs, loc)
		}
		codes = append(codes, pl.Address.AddressCountry)
	}
	return strings.Join(locs, "; "), countriesFromCodes(codes)
}

func containsFold(list []string, s string) bool {
	for _, x := range list {
		if strings.EqualFold(x, s) {
			return true
		}
	}
	return false
}

// earcuSummaryLocation reads the detail page's summary list (li.jobSumItem rows of a
// jobSumLabel and a jobSumValue) for the first row whose label names a location — "Location:",
// or a tenant's own "Office Location:". The labels are tenant-configured, so the match is on the
// label's last word rather than a fixed string.
func earcuSummaryLocation(root *html.Node) string {
	var out string
	walk(root, func(n *html.Node) bool {
		if out != "" {
			return false
		}
		if n.Type != html.ElementNode || n.Data != "li" || !HasClass(n, "jobSumItem") {
			return true
		}
		var label, value string
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			switch {
			case c.Type != html.ElementNode:
			case HasClass(c, "jobSumLabel"):
				label = strings.TrimSuffix(textContent(c), ":")
			case HasClass(c, "jobSumValue"):
				value = textContent(c)
			}
		}
		if f := strings.Fields(label); len(f) > 0 && strings.EqualFold(f[len(f)-1], "location") {
			out = value
		}
		return false
	})
	return out
}

// earcuField returns the value of the named metadata-prefix field, "" when the prefix has no
// such field.
func earcuField(prefix, key string) string {
	locs := earcuFieldRe.FindAllStringSubmatchIndex(prefix, -1)
	for i, loc := range locs {
		if prefix[loc[2]:loc[3]] != key {
			continue
		}
		end := len(prefix)
		if i+1 < len(locs) {
			end = locs[i+1][0]
		}
		return strings.TrimSpace(prefix[loc[1]:end])
	}
	return ""
}

// earcuLocation returns the posting's location from the feed item's metadata prefix,
// preferring the Location field and falling back to Country; empty when neither is present.
func earcuLocation(prefix string) string {
	if loc := earcuField(prefix, "Location"); loc != "" {
		return loc
	}
	return earcuField(prefix, "Country")
}

// earcuSplit divides a feed item's description into the metadata prefix (the leading
// "Key: value, …" text) and the inline body (the rssjobdesc block, "" when absent).
// Splitting once keeps location parsing off the HTML body, where a stray "Country:"/
// "Location:" in the posting text would otherwise be mistaken for metadata.
func earcuSplit(desc string) (prefix, body string) {
	if i := strings.Index(desc, `<div id="rssjobdesc"`); i >= 0 {
		return desc[:i], desc[i:]
	}
	return desc, ""
}

// earcuPubDate parses an eArcu feed pubDate. eArcu emits an RFC822 date whose zone is a
// literal "Z" (UTC), sometimes with an unpadded day, so the primary layout uses a
// numeric-offset zone (which also accepts an explicit +hhmm) and an unpadded day; a feed
// using a named zone (GMT/MST) falls back to the standard RFC1123 forms.
func earcuPubDate(s string) *time.Time {
	if t := parseLayout("Mon, 2 Jan 2006 15:04:05 Z0700", s); t != nil {
		return t
	}
	return parsePubDate(s)
}
