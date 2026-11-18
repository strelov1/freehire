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
// marketplace. The sitemap lists every vacancy URL and each page server-renders a schema.org
// JobPosting ld+json block, so it enumerates the sitemap and maps details over fetchDetails +
// ldJobPosting. The real employer is hidden (hiringOrganization is always "Strider"), so it is
// boardless single-company.
//
// Open/closed is read from the hero's data-position-open attribute, NOT from the presence of the
// JobPosting markup, which stays on closed pages. The visible text cannot tell either: the page
// ships BOTH the "Accepting Applications" and "Closed" spans and CSS picks one off that attribute.
type onstrider struct {
	http onstriderHTTP
	// referral is the Strider referral handle (ONSTRIDER_REFERRAL_HANDLE, see registry.go) that
	// outbound links are attributed to. Empty means unattributed: URL stays the canonical page.
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

	// The sitemap also carries blog, marketing, localized and preview duplicate URLs.
	var urls []string
	for _, u := range sitemap.URLs {
		if onstriderVacancyURL(u.Loc) {
			urls = append(urls, u.Loc)
		}
	}

	jobs := fetchDetails(urls, defaultDetailWorkers, func(u string) (Job, bool) {
		return s.detail(ctx, ce, u)
	})
	// No detail mapped (the Cloudflare edge can 403 datacenter IPs on detail pages while the
	// sitemap still serves). Fail rather than return an empty success, which the unseen sweep
	// would read as "all vacancies gone".
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

// onstriderVacancyURLPattern matches https://www.onstrider.com/jobs/<slug>-<8hex>. Anchoring /jobs/
// after the host excludes the /pt/, /en/ and preview duplicates; the 8-hex id excludes the listing root.
var onstriderVacancyURLPattern = regexp.MustCompile(`^https?://www\.onstrider\.com/jobs/[a-z0-9-]+-[0-9a-f]{8}/?$`)

// onstriderVacancyURL reports whether u is a canonical vacancy URL.
func onstriderVacancyURL(u string) bool { return onstriderVacancyURLPattern.MatchString(u) }

// onstriderPositionOpen reports whether the vacancy page states the position is still taking
// applications, reading the hero's data-position-open attribute ("1" open, "0" closed). The
// visible text is useless: both labels are in the markup.
//
// A page without the attribute reads as CLOSED: its absence means the page shape moved, and
// admitting unknown-status vacancies fills the catalogue with dead links. Fetch fails the board
// when none map, so a site-wide template change stops loudly.
func onstriderPositionOpen(root *html.Node) bool {
	open, done := false, false
	walk(root, func(n *html.Node) bool {
		if done {
			return false
		}
		if n.Type == html.ElementNode {
			if v := Attr(n, "data-position-open"); v != "" {
				open, done = v == "1", true
				return false
			}
		}
		return true
	})
	return open
}

// onstriderReferralHandlePattern is the shape a referral handle may take. It is interpolated into
// a URL path, so anything else is rejected at construction rather than escaped.
var onstriderReferralHandlePattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// applyURL wraps a vacancy URL in Strider's share-link shape: /r/<handle> on the app host with
// the destination base64-encoded in ?job=. Only that route records the referral; the plain
// ?referral=<handle> form sets no attribution cookie. An unset handle (or an unparseable URL)
// leaves the canonical URL alone.
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
// is a PropertyValue holding the vacancy UUID; jobLocationType is the remote signal.
type onstriderPosting struct {
	Title           string `json:"title"`
	Description     string `json:"description"`
	DatePosted      string `json:"datePosted"`
	JobLocationType string `json:"jobLocationType"`
	// EmploymentType is raw because schema.org permits a string or an array; a fixed []string
	// would fail on the scalar and ldJobPosting would drop the posting. See firstEmploymentType.
	EmploymentType json.RawMessage `json:"employmentType"`
	Identifier     struct {
		Value string `json:"value"`
	} `json:"identifier"`
	ApplicantLocationRequirements []struct {
		Name string `json:"name"`
	} `json:"applicantLocationRequirements"`
}

// location joins the applicant-location countries (there is no city), expanding each ISO alpha-2
// code to its English name: the location parser reads a bare 2-letter token as a US subdivision
// first ("CO" is Colorado, "PA" Pennsylvania). An unmapped code falls back to itself.
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

// onstriderCountryNames covers the Latin American ISO codes Strider emits; others fall back to the raw code.
var onstriderCountryNames = map[string]string{
	"AR": "Argentina", "BO": "Bolivia", "BR": "Brazil", "CL": "Chile",
	"CO": "Colombia", "CR": "Costa Rica", "CU": "Cuba", "DO": "Dominican Republic",
	"EC": "Ecuador", "GT": "Guatemala", "HN": "Honduras", "MX": "Mexico",
	"NI": "Nicaragua", "PA": "Panama", "PE": "Peru", "PR": "Puerto Rico",
	"PY": "Paraguay", "SV": "El Salvador", "UY": "Uruguay", "VE": "Venezuela",
}

// firstEmploymentType returns the first schema.org employmentType, accepting a JSON array or a
// bare string. It returns "" when the field is absent or unparseable.
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
