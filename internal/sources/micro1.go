package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/skilltag"
)

// micro1 adapts the micro1 job board (jobs.micro1.ai), a boardless single-company source.
// The board root redirects to a marketing listing, but jobs.micro1.ai/sitemap.xml enumerates
// every /post/<uuid> posting, and each posting page server-renders its full payload into the
// Next.js RSC flight stream (no standalone ld+json tag). This is the dataart shape — sitemap
// to enumerate, per-posting detail fetch — with the detail parsed out of the flight via the
// shared decodeNextFlight/bracketSlice/nextFlightTextRows primitives (as deel does).
type micro1 struct {
	http micro1HTTP
}

// micro1HTTP is the transport micro1 needs: the XML sitemap plus HTML detail pages.
type micro1HTTP interface {
	XMLGetter
	HTMLGetter
}

const micro1SitemapURL = "https://jobs.micro1.ai/sitemap.xml"

// NewMicro1 builds the micro1 adapter over the given HTTP client.
func NewMicro1(c micro1HTTP) Source { return micro1{http: c} }

func (micro1) Provider() string { return "micro1" }

// micro1 is single-company, so its config entries carry no board.
func (micro1) boardless() {}

// Fetch enumerates the sitemap, keeps the canonical /post/<uuid> URLs, and fetches each
// posting's detail concurrently.
func (m micro1) Fetch(ctx context.Context, ce CompanyEntry) ([]Job, error) {
	var sitemap struct {
		URLs []struct {
			Loc string `xml:"loc"`
		} `xml:"url"`
	}
	if err := m.http.GetXML(ctx, micro1SitemapURL, &sitemap); err != nil {
		return nil, fmt.Errorf("micro1: sitemap: %w", err)
	}

	var urls []string
	for _, u := range sitemap.URLs {
		if micro1PostID(u.Loc) != "" {
			urls = append(urls, u.Loc)
		}
	}

	return fetchDetails(urls, defaultDetailWorkers, func(u string) (Job, bool) {
		return m.detail(ctx, ce, u)
	}), nil
}

// detail fetches one posting page and maps its RSC-flight payload to a Job. ok is false when
// the URL is not a canonical post (would collide on the dedup key), the fetch/flight decode
// fails, the page carries no data object, or the posting is no longer open — the caller then
// skips just that posting.
func (m micro1) detail(ctx context.Context, ce CompanyEntry, jobURL string) (Job, bool) {
	if micro1PostID(jobURL) == "" {
		return Job{}, false // not a canonical /post/<uuid> URL → skip before fetching
	}
	root, err := m.http.GetHTML(ctx, jobURL)
	if err != nil {
		return Job{}, false
	}
	flight, err := decodeNextFlight(root)
	if err != nil {
		return Job{}, false
	}
	d, ok := extractMicro1Data(flight)
	if !ok || d.ClientJobID == "" {
		return Job{}, false
	}
	// The sitemap tracks live postings, but if a page still reports a non-open status skip it
	// so it is not ingested as a live job (a cheap freshness guard on the structured field).
	if d.JobStatus != "" && d.JobStatus != "open" {
		return Job{}, false
	}

	// The description is a "$N" reference into the flight's text rows; resolve it (an
	// unresolved reference degrades to an empty description rather than dropping the posting).
	desc := d.JobDescription
	if ref, isRef := strings.CutPrefix(desc, "$"); isRef {
		desc = nextFlightTextRows(flight)[ref]
	}

	workMode := micro1WorkMode(d.LocationType)
	location := strings.TrimSpace(d.LocationName)
	return Job{
		ExternalID:  d.ClientJobID,
		URL:         jobURL,
		Title:       strings.TrimSpace(d.JobRoleName),
		Company:     ce.Company,
		Location:    location,
		Description: sanitizeHTML(desc),
		Remote:      workMode == "remote" || isRemote(d.JobRoleName+" "+location),
		WorkMode:    workMode,
		Skills:      micro1Skills(d.RequiredSkills),
		PostedAt:    parseLayout("2006-01-02 15:04:05", d.CreateDatetime),
	}, true
}

// micro1WorkMode maps micro1's structured location_type enum ("remote"/"hybrid"/"onsite")
// to freehire's WorkMode, returning "" for any other value (e.g. a null type) so the pipeline
// falls back to parsing the location text — structured signal only, never a heuristic.
func micro1WorkMode(locationType string) string {
	switch mode := strings.ToLower(strings.TrimSpace(locationType)); mode {
	case "remote", "hybrid", "onsite":
		return mode
	default:
		return ""
	}
}

// micro1Skills canonicalizes micro1's free-form required_skills (e.g. "GoLang", "Typescript")
// through the shared skilltag dictionary, so the Skills facet carries canonical names rather
// than the platform's raw spellings.
func micro1Skills(skills []string) []string {
	return skilltag.Parse(strings.Join(skills, " "))
}

// micro1PostPattern captures the posting UUID from a canonical board posting URL
// (https://jobs.micro1.ai/post/<uuid>). Anchoring the host and requiring a UUID segment
// excludes the board root, non-UUID paths, deeper sub-paths, and other hosts.
var micro1PostPattern = regexp.MustCompile(
	`^https?://jobs\.micro1\.ai/post/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})/?$`)

// micro1PostID extracts the posting UUID from a canonical posting URL, returning "" for the
// board root, non-post URLs, and any URL on another host.
func micro1PostID(u string) string {
	if m := micro1PostPattern.FindStringSubmatch(u); m != nil {
		return m[1]
	}
	return ""
}

// micro1Data is the subset of a micro1 posting's RSC-flight `data` object the adapter maps.
// JobDescription is a "$N" reference into the flight's text rows (resolved via
// nextFlightTextRows), not inline HTML.
type micro1Data struct {
	ClientJobID    string   `json:"client_job_id"`
	JobRoleName    string   `json:"job_role_name"`
	JobDescription string   `json:"job_description"`
	JobStatus      string   `json:"job_status"`
	CreateDatetime string   `json:"create_datetime"`
	RequiredSkills []string `json:"required_skills"`
	LocationType   string   `json:"location_type"`
	LocationName   string   `json:"location_name"`
}

// extractMicro1Data decodes the posting `data` object out of the flight stream. It anchors on
// the payload's own opening — `"data":{"client_job_id"` — rather than a bare `"data":`, so a
// stray earlier `"data":` from unrelated component state on some pages cannot match the wrong
// object. ok is false when the page carries no such object.
func extractMicro1Data(flight string) (micro1Data, bool) {
	raw, ok := bracketSlice(flight, `"data":{"client_job_id"`, '{', '}')
	if !ok {
		return micro1Data{}, false
	}
	var d micro1Data
	if err := json.Unmarshal([]byte(raw), &d); err != nil {
		return micro1Data{}, false
	}
	return d, true
}
