package sources

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// onstrider adapts Strider's careers site (www.onstrider.com), a HubSpot-hosted LATAM talent
// marketplace. It is the DataArt shape — the sitemap enumerates every vacancy URL and each
// vacancy page server-renders a schema.org JobPosting ld+json block, so this adapter enumerates
// from the sitemap and maps per-vacancy details over the shared fetchDetails + ldJobPosting
// helpers. The real employer is hidden (hiringOrganization is always "Strider"), so every
// posting maps to the single board-file company; the adapter is boardless single-company.
//
// Open/closed is read from the hero's data-position-open attribute, NOT from the presence of the
// JobPosting markup. The markup was the original signal and it is far too weak: measured live
// over the whole sitemap (308 vacancies, 2026-09-01), only 34 had dropped it while 266 of the
// 274 readable pages were closed — so the old rule admitted a catalogue that was ~97% dead
// links. Nothing in the visible text distinguishes them either: the page always ships BOTH the
// "Accepting Applications" and "Closed" spans and lets CSS pick one off the same attribute, so
// matching on the text reads "closed" for every vacancy.
type onstrider struct {
	http onstriderHTTP
	// referral is the Strider referral handle outbound links are attributed to, from
	// ONSTRIDER_REFERRAL_HANDLE (see registry.go). Empty means unattributed: URL stays the
	// canonical vacancy page, which is what an installation that is not a Strider referral
	// partner — including every fork of this repository — gets.
	referral string
}

// onstriderHTTP is the transport onstrider needs: the XML sitemap plus HTML detail pages.
type onstriderHTTP interface {
	XMLGetter
	HTMLGetter
}

const onstriderSitemapURL = "https://www.onstrider.com/sitemap.xml"

// NewOnstrider builds the onstrider adapter over the given HTTP client, attributing outbound
// links to the given Strider referral handle (empty = unattributed, see onstrider.referral).
func NewOnstrider(c onstriderHTTP, referral string) Source {
	if !onstriderReferralHandlePattern.MatchString(referral) {
		referral = "" // absent or malformed: crawl unattributed rather than emit a broken link
	}
	return onstrider{http: c, referral: referral}
}

func (onstrider) Provider() string { return "onstrider" }

// onstrider is single-company (the real employer is hidden), so its config entry carries no board.
func (onstrider) boardless() {}

func (s onstrider) Fetch(ctx context.Context, ce CompanyEntry) ([]Job, error) {
	var sitemap struct {
		URLs []struct {
			Loc string `xml:"loc"`
		} `xml:"url"`
	}
	if err := s.http.GetXML(ctx, onstriderSitemapURL, &sitemap); err != nil {
		return nil, fmt.Errorf("onstrider: sitemap: %w", err)
	}

	// Keep only canonical vacancy URLs: the sitemap also carries blog, marketing, localized
	// (/pt/, /en/), and /preview-slug-…/ duplicate URLs, none of which are ingestable vacancies.
	var urls []string
	for _, u := range sitemap.URLs {
		if onstriderVacancyURL(u.Loc) {
			urls = append(urls, u.Loc)
		}
	}

	jobs := fetchDetails(urls, defaultDetailWorkers, func(u string) (Job, bool) {
		return s.detail(ctx, ce, u)
	})
	// The sitemap listed vacancies but not one detail mapped — every fetch was blocked or empty
	// (the Cloudflare edge can 403 the prod datacenter IP for the detail pages while the CDN
	// still serves the sitemap). Fail the board rather than return an empty success, which the
	// post-run unseen sweep would read as "all vacancies gone" and false-close the catalogue.
	if len(urls) > 0 && len(jobs) == 0 {
		return nil, fmt.Errorf("onstrider: %d vacancy URLs but 0 mapped; treating as a fetch failure", len(urls))
	}
	return jobs, nil
}

// detail fetches one vacancy page and maps its JobPosting ld+json to a Job. It returns ok=false
// when the fetch fails, the vacancy is closed, the page carries no JobPosting block, or the
// posting has no identifier.value (no dedup key) — so the caller drops just that vacancy.
func (s onstrider) detail(ctx context.Context, ce CompanyEntry, jobURL string) (Job, bool) {
	root, err := s.http.GetHTML(ctx, jobURL)
	if err != nil {
		return Job{}, false
	}
	if !onstriderPositionOpen(root) {
		return Job{}, false // closed vacancy, or a page shape that no longer states it
	}
	var p onstriderPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false // no JobPosting markup to map
	}
	if p.Identifier.Value == "" {
		return Job{}, false // no native id → would collide on the dedup key; skip it
	}
	remote := strings.EqualFold(p.JobLocationType, "TELECOMMUTE")
	return Job{
		ExternalID:     p.Identifier.Value,
		URL:            s.applyURL(jobURL),
		Title:          p.Title,
		Company:        ce.Company,
		Location:       p.location(),
		Description:    sanitizeHTML(p.Description),
		Remote:         remote,
		WorkMode:       workModeFromRemote(remote),
		EmploymentType: schemaEmploymentType(p.firstEmploymentType()),
		PostedAt:       parseDate(p.DatePosted),
	}, true
}

// onstriderVacancyURLPattern matches a canonical vacancy URL
// (https://www.onstrider.com/jobs/<slug>-<8hex>). Anchoring /jobs/ directly after the host
// excludes the /pt/ + /en/ localizations and the /preview-slug-<uuid>/… duplicate pages;
// requiring the trailing 8-hex id excludes the /jobs listing root and marketing pages.
var onstriderVacancyURLPattern = regexp.MustCompile(`^https?://www\.onstrider\.com/jobs/[a-z0-9-]+-[0-9a-f]{8}/?$`)

// onstriderVacancyURL reports whether u is a canonical vacancy URL.
func onstriderVacancyURL(u string) bool { return onstriderVacancyURLPattern.MatchString(u) }

// onstriderPositionOpen reports whether the vacancy page states the position is still taking
// applications, reading the hero's data-position-open attribute ("1" open, "0" closed). It is
// the only place the page states this: both status labels are hard-coded into the markup and
// CSS picks one off this same attribute, so the visible text says "Closed" on an open vacancy.
//
// A page carrying no such attribute reads as CLOSED, because its absence means the page shape
// moved and the status is no longer stated — admitting an unknown-status vacancy is what put a
// year of dead links in the catalogue. Fetch already fails the board when the sitemap lists
// vacancies and none map, so a site-wide template change stops the run loudly rather than
// closing the catalogue behind it.
func onstriderPositionOpen(root *html.Node) bool {
	open, done := false, false
	walk(root, func(n *html.Node) bool {
		if done {
			return false
		}
		if n.Type == html.ElementNode {
			if v := attr(n, "data-position-open"); v != "" {
				open, done = v == "1", true
				return false
			}
		}
		return true
	})
	return open
}

// onstriderReferralHandlePattern is the shape a Strider referral handle may take. The handle is
// interpolated into a URL path, so anything else is rejected at construction rather than
// escaped: a handle is copied from a Strider profile by hand, and a malformed one is a
// misconfiguration to notice, not input to sanitise.
var onstriderReferralHandlePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// applyURL wraps a canonical vacancy URL in the configured Strider referral link, the shape
// Strider's own share links use: /r/<handle> on the app host, carrying the destination
// base64-encoded in ?job=. Verified live — landing on it records the referral as the visit's
// first touch. The plainer www.onstrider.com/jobs/<slug>?referral=<handle> form does NOT: that
// page sets no attribution cookie of its own, so only the /r/ route earns the referral.
//
// An unset handle (or a URL this cannot take apart) leaves the canonical vacancy URL alone.
func (s onstrider) applyURL(jobURL string) string {
	if s.referral == "" {
		return jobURL
	}
	_, slug, ok := strings.Cut(jobURL, "/jobs/")
	slug = strings.TrimSuffix(slug, "/")
	if !ok || slug == "" || strings.Contains(slug, "/") {
		return jobURL
	}
	dest := base64.StdEncoding.EncodeToString([]byte(slug + "?referral=" + s.referral))
	return "https://app.onstrider.com/r/" + s.referral + "?job=" + dest
}

// onstriderPosting is the schema.org JobPosting decoded from a vacancy page's ld+json. identifier
// is a PropertyValue whose value is the vacancy UUID; applicantLocationRequirements is an array
// of Country nodes; employmentType is an array; jobLocationType is the remote signal.
type onstriderPosting struct {
	Title           string `json:"title"`
	Description     string `json:"description"`
	DatePosted      string `json:"datePosted"`
	JobLocationType string `json:"jobLocationType"`
	// EmploymentType is raw because schema.org permits either a single string ("FULL_TIME") or
	// an array (["PART_TIME","CONTRACTOR"]); decoding it into a fixed []string would fail on the
	// scalar form and — since ldJobPosting drops a posting whose decode errors — silently discard
	// an otherwise-open vacancy. firstEmploymentType parses both shapes.
	EmploymentType json.RawMessage `json:"employmentType"`
	Identifier     struct {
		Value string `json:"value"`
	} `json:"identifier"`
	ApplicantLocationRequirements []struct {
		Name string `json:"name"`
	} `json:"applicantLocationRequirements"`
}

// location joins the applicant-location-requirement countries (the hidden employer means there is
// no city, only the countries a candidate may apply from), expanding each ISO 3166-1 alpha-2 code
// to its English country name. The expansion matters: the location parser deliberately reads a
// bare 2-letter token as a US subdivision before an ISO country code (so "CO"→Colorado, "AR"→
// Arkansas, "PA"→Pennsylvania), which would mis-place these LATAM postings in the US. The full
// name resolves unambiguously to the country. An unmapped code falls back to itself.
func (p onstriderPosting) location() string {
	var out []string
	for _, c := range p.ApplicantLocationRequirements {
		if c.Name == "" {
			continue
		}
		if name, ok := onstriderCountryNames[strings.ToUpper(c.Name)]; ok {
			out = append(out, name)
		} else {
			out = append(out, c.Name)
		}
	}
	return strings.Join(out, ", ")
}

// onstriderCountryNames expands the ISO 3166-1 alpha-2 country codes onstrider emits into the
// English names the location parser resolves without the US-subdivision collision. Strider is a
// Latin-America talent marketplace, so this covers Latin America; an out-of-region code (rare for
// this source) falls back to the raw code, which the parser handles directly when unambiguous.
var onstriderCountryNames = map[string]string{
	"AR": "Argentina", "BO": "Bolivia", "BR": "Brazil", "CL": "Chile",
	"CO": "Colombia", "CR": "Costa Rica", "CU": "Cuba", "DO": "Dominican Republic",
	"EC": "Ecuador", "GT": "Guatemala", "HN": "Honduras", "MX": "Mexico",
	"NI": "Nicaragua", "PA": "Panama", "PE": "Peru", "PR": "Puerto Rico",
	"PY": "Paraguay", "SV": "El Salvador", "UY": "Uruguay", "VE": "Venezuela",
}

// firstEmploymentType returns the first schema.org employmentType enum, accepting either shape
// the spec permits: a JSON array (["PART_TIME","CONTRACTOR"]) or a bare string ("FULL_TIME").
// It returns "" when the field is absent or unparseable, leaving the value to the parser.
func (p onstriderPosting) firstEmploymentType() string {
	if len(p.EmploymentType) == 0 {
		return ""
	}
	var arr []string
	if json.Unmarshal(p.EmploymentType, &arr) == nil {
		if len(arr) > 0 {
			return arr[0]
		}
		return ""
	}
	var one string
	_ = json.Unmarshal(p.EmploymentType, &one)
	return one
}
