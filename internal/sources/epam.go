package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// epam adapts EPAM's careers site (careers.epam.com). EPAM's job-search API
// (www.epam.com/api/jobs/search) is behind a Cloudflare challenge, but the gzip sitemap
// enumerates every vacancy and each vacancy page server-renders a schema.org JobPosting
// ld+json block (not Cloudflare-gated), so this adapter is the SuccessFactors shape —
// sitemap to enumerate, per-job detail fetch for the posting — over the shared ld+json
// helper. The board is the career-site host.
type epam struct {
	http epamHTTP
}

// epamHTTP is the transport epam needs: the gzip XML sitemap (the Go transport
// transparently decodes the Content-Encoding: gzip response) plus HTML detail pages.
type epamHTTP interface {
	XMLGetter
	HTMLGetter
}

// NewEPAM builds the EPAM adapter over the given HTTP client.
func NewEPAM(c epamHTTP) Source { return epam{http: c} }

func (epam) Provider() string { return "epam" }

func (e epam) Fetch(ctx context.Context, ce CompanyEntry) ([]Job, error) {
	sitemapURL := fmt.Sprintf("https://%s/sitemap.xml.gz", ce.Board)
	// Keep only English vacancy pages (epamJobID is empty for the listing, language roots,
	// and the /uk//de/… localisations, so each vacancy is ingested once under its English url).
	urls, err := sitemapJobLocs(ctx, e.http, sitemapURL, epamJobID)
	if err != nil {
		return nil, fmt.Errorf("epam: sitemap %s: %w", ce.Board, err)
	}

	// Each job's posting comes from its own page fetch, fanned out under a bounded pool.
	return fetchDetails(urls, defaultDetailWorkers, func(u string) (Job, bool) {
		return e.detail(ctx, ce, u)
	}), nil
}

// detail fetches one vacancy page and maps its JobPosting ld+json to a Job, returning
// ok=false when the URL has no parseable id, the fetch fails, or the page carries no
// JobPosting, so the caller skips just that posting.
func (e epam) detail(ctx context.Context, ce CompanyEntry, jobURL string) (Job, bool) {
	id := epamJobID(jobURL)
	if id == "" {
		return Job{}, false // no native id → would collide on the dedup key; skip it
	}
	root, err := e.http.GetHTML(ctx, jobURL)
	if err != nil {
		return Job{}, false
	}
	var p epamPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}

	location := p.location()
	// jobLocationType is EPAM's only structured work-arrangement signal: TELECOMMUTE means
	// remote. Absent → leave WorkMode empty and fall back to the location text for remote.
	remote := p.JobLocationType == "TELECOMMUTE"
	workMode := ""
	if remote {
		workMode = "remote"
	}
	// The ld+json description is a flattened, structure-less blob (headings and bullet lists
	// collapsed into one run of text, benefits dropped). The page's __NEXT_DATA__ carries the
	// same content as structured sections, so prefer it and keep the ld+json as the fallback.
	description := sanitizeHTML(html.UnescapeString(p.Description))
	if structured := epamStructuredDescription(root); structured != "" {
		description = structured
	}
	return Job{
		ExternalID:  id,
		URL:         jobURL,
		Title:       p.Title,
		Company:     ce.Company,
		Location:    location,
		Description: description,
		Remote:      remote || isRemote(location),
		WorkMode:    workMode,
		PostedAt:    parseDate(p.DatePosted),
	}, true
}

// epamJobIDPattern captures the Contentstack vacancy uid (e.g. "blt01b3u51rnautbmxq") from
// an /en/vacancy/<slug>-<uid>_en URL. Restricting to /en/vacancy/ and the _en suffix both
// filters the sitemap to English vacancies and yields the dedup id in one match.
var epamJobIDPattern = regexp.MustCompile(`/en/vacancy/[^/?#]*-(blt[a-z0-9]+)_en(?:[/?#]|$)`)

// epamJobID extracts the vacancy uid from an English vacancy URL, returning "" for the
// listing, language roots, and non-English vacancy localisations.
func epamJobID(u string) string {
	return firstSubmatch(epamJobIDPattern, u)
}

// epamPosting is the schema.org JobPosting decoded from an EPAM vacancy page's ld+json
// block. EPAM emits no jobLocation; the location is built from applicantLocationRequirements
// (an array of Country) and the work arrangement from jobLocationType.
type epamPosting struct {
	Title                         string        `json:"title"`
	Description                   string        `json:"description"`
	DatePosted                    string        `json:"datePosted"`
	JobLocationType               string        `json:"jobLocationType"`
	ApplicantLocationRequirements []epamCountry `json:"applicantLocationRequirements"`
}

type epamCountry struct {
	Name string `json:"name"`
}

// location joins the applicant-location countries (the only location signal EPAM exposes
// in the JobPosting), so a job open to several countries lists them all.
func (p epamPosting) location() string {
	names := make([]string, 0, len(p.ApplicantLocationRequirements))
	for _, c := range p.ApplicantLocationRequirements {
		names = append(names, c.Name)
	}
	return joinNonEmpty(names...)
}

// epamNextData is the slice of the vacancy page's __NEXT_DATA__ payload we read: the job's
// structured description. Only the fields we render are modelled; encoding/json ignores the
// rest, and absent/null fields decode to their zero value.
type epamNextData struct {
	Props struct {
		PageProps struct {
			Job epamJob `json:"job"`
		} `json:"pageProps"`
	} `json:"props"`
}

// epamJob is the structured description EPAM flattens away in its ld+json: an HTML intro, the
// three bullet-list categories, and the benefits blocks (already HTML). The category also
// exposes about_the_project/about_the_customer/technologies, but those are null across every
// sampled vacancy, so they are left as a seam rather than modelled speculatively.
type epamJob struct {
	Description string `json:"description"` // intro, already HTML
	Category    struct {
		Responsibilities []string `json:"responsibilities"`
		Requirements     []string `json:"requirements"`
		NiceToHave       []string `json:"nice_to_have"`
	} `json:"category"`
	Benefits []struct {
		Content string `json:"content"` // already HTML (<ul><li>…</li></ul>)
	} `json:"benefits"`
}

// epamStructuredDescription reconstructs a structured HTML description from the page's
// __NEXT_DATA__ payload, returning "" when the script is absent, unparseable, or carries no
// description content — so the caller falls back to the flat ld+json description.
func epamStructuredDescription(root *html.Node) string {
	raw := scriptTextByID(root, "__NEXT_DATA__")
	if raw == "" {
		return ""
	}
	var data epamNextData
	if json.Unmarshal([]byte(raw), &data) != nil {
		return ""
	}
	return sanitizeHTML(data.Props.PageProps.Job.descriptionHTML())
}

// descriptionHTML assembles the intro, the three bullet-list sections, and the benefits into a
// single HTML document ready for sanitizing. Empty sections are skipped so a job that omits,
// say, "Nice to have" carries no dangling heading.
func (j epamJob) descriptionHTML() string {
	var b strings.Builder
	b.WriteString(j.Description)
	epamListSection(&b, "Responsibilities", j.Category.Responsibilities)
	epamListSection(&b, "Requirements", j.Category.Requirements)
	epamListSection(&b, "Nice to have", j.Category.NiceToHave)

	var benefits strings.Builder
	for _, ben := range j.Benefits {
		if strings.TrimSpace(ben.Content) != "" {
			benefits.WriteString(ben.Content)
		}
	}
	if benefits.Len() > 0 {
		b.WriteString("<h3>Benefits</h3>")
		b.WriteString(benefits.String())
	}
	return b.String()
}

// epamSectionLabels are the section headings EPAM's data occasionally leaks as the last bullet
// of the preceding list; such items are dropped so they don't render as stray one-word bullets.
var epamSectionLabels = map[string]bool{
	"responsibilities": true,
	"requirements":     true,
	"nice to have":     true,
	"benefits":         true,
}

// epamListSection appends a "<h3>title</h3><ul><li>…</li></ul>" block for the non-empty items,
// or nothing when every item is blank. Items that are just a leaked section heading are skipped.
func epamListSection(b *strings.Builder, title string, items []string) {
	var lis strings.Builder
	for _, it := range items {
		s := strings.TrimSpace(it)
		if s == "" || epamSectionLabels[strings.ToLower(s)] {
			continue
		}
		lis.WriteString("<li>" + s + "</li>")
	}
	if lis.Len() == 0 {
		return
	}
	b.WriteString("<h3>" + title + "</h3><ul>")
	b.WriteString(lis.String())
	b.WriteString("</ul>")
}
