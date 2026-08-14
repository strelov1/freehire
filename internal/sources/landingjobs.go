package sources

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// landingjobs adapts landing.jobs, an EU tech job board. Boardless (one public API, no
// per-tenant board) and multi-company, so it stays in the source facet and takes each
// posting's employer from the posting rather than from the config entry.
//
// Two shapes make it unlike its neighbours.
//
// **The employer is not in the payload.** There is no company field, so it comes from the
// posting URL, `https://landing.jobs/at/<company-slug>/<job-slug>`, humanized from the first
// segment. A posting whose URL does not have that shape is skipped rather than ingested under a
// guessed employer, which would break its company slug. The posting's own numeric `id` is the
// external id — the slug pair was used for that in the first draft and is not durable enough:
// live slugs bake in a year (`…-in-lisbon-2025`), so a slug the board regenerates would
// silently duplicate the posting instead of updating it.
//
// **One request, and no pagination.** `?page=N` is ignored by the endpoint — pages 1 and 2 come
// back byte-for-byte identical (verified live 2026-08-13, issue #1627) — so a page walk would
// re-fetch the same postings until its own cap and dedup them all away, paying N requests for
// one page of data. The single response is therefore taken as the whole feed. Whether the 50
// items it returns ARE the whole active catalogue is unconfirmed; if a parameter controlling
// depth is found, this is where the walk goes, and `internal/sources/AGENTS.md`'s first-page
// rule applies to it.
//
// Note: landing.jobs' robots.txt disallows /api/ for every listed user agent. The endpoint is
// public and unauthenticated, and the crawl is a scheduled job rather than a search-engine
// crawler, but this is recorded here — as it is on remotli, the other adapter in this position —
// because it was raised in the adapter's tracking issue as a conscious call rather than a silent
// one.
type landingjobs struct {
	http JSONGetter
}

const landingjobsListURL = "https://landing.jobs/api/v1/jobs"

// landingjobsPathMarker precedes the "<company-slug>/<job-slug>" pair in a posting URL.
const landingjobsPathMarker = "/at/"

// NewLandingJobs builds the landing.jobs adapter over the given HTTP client.
func NewLandingJobs(c JSONGetter) Source { return landingjobs{http: c} }

func (landingjobs) Provider() string { return "landingjobs" }

// landing.jobs is one global feed, so its config entry carries no board.
func (landingjobs) boardless() {}

// landing.jobs carries postings from many companies, so it stays in the source facet.
func (landingjobs) aggregator() {}

// landingjobsPosting is one posting, body inline (no detail call).
//
// The feed also carries `tags`, `gross_salary_low`/`gross_salary_high` and `currency_code`,
// none of which are declared. Salary has no home on Job (enrichment owns it), and the element
// shape of `tags` was not confirmed — declaring a wrong Go type for it would fail the decode of
// the WHOLE feed rather than that one field, so the skills dictionary mines the description
// instead. Adding it is a small change once the shape is verified.
type landingjobsPosting struct {
	ID               int64              `json:"id"`
	Title            string             `json:"title"`
	URL              string             `json:"url"`
	Locations        []landingjobsPlace `json:"locations"`
	Remote           bool               `json:"remote"`
	Type             string             `json:"type"`
	PublishedAt      string             `json:"published_at"`
	CreatedAt        string             `json:"created_at"`
	RoleDescription  string             `json:"role_description"`
	MainRequirements string             `json:"main_requirements"`
	NiceToHave       string             `json:"nice_to_have"`
	Perks            string             `json:"perks"`
}

// landingjobsPlace is one entry of a posting's locations. The array is null for a fully-remote
// role, and carries SEVERAL entries for a role open in more than one city — 28% of a live
// sample — so neither reader may stop at the first.
type landingjobsPlace struct {
	City        string `json:"city"`
	CountryCode string `json:"country_code"`
}

// label renders one place as "City, CC", or whichever half the entry actually carries.
func (p landingjobsPlace) label() string {
	return joinNonEmpty(strings.TrimSpace(p.City), strings.TrimSpace(p.CountryCode))
}

func (s landingjobs) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	var postings []landingjobsPosting
	if err := s.http.GetJSON(ctx, landingjobsListURL, &postings); err != nil {
		return nil, fmt.Errorf("landingjobs: list: %w", err)
	}
	jobs := make([]Job, 0, len(postings))
	for _, p := range postings {
		if job, ok := p.toJob(); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// toJob maps a posting to a Job, returning ok=false without a native id, a title, or a
// URL-derivable employer — the id is the dedup key and the employer backs the company slug, so
// a posting missing either would be re-inserted or filed under a fabricated company.
func (p landingjobsPosting) toJob() (Job, bool) {
	company, ok := landingjobsCompanyFromURL(p.URL)
	if !ok || p.ID == 0 || p.Title == "" {
		return Job{}, false
	}
	// remote is a structured boolean, so it may set WorkMode — but only in the direction the
	// field actually states. false means "not flagged remote", which is not the same as onsite
	// (the board carries hybrid roles too), so the mode is left for the pipeline to derive.
	mode := ""
	if p.Remote {
		mode = "remote"
	}
	return Job{
		ExternalID:  strconv.FormatInt(p.ID, 10),
		URL:         p.URL,
		Title:       p.Title,
		Company:     company,
		Location:    landingjobsLocation(p),
		Description: landingjobsDescription(p),
		Remote:      p.Remote,
		WorkMode:    mode,
		Countries:   landingjobsCountries(p.Locations),
		// The board states the employment type in a structured field ("Full-time"), which the
		// shared schema.org mapper resolves once the separator matches its vocabulary — the same
		// hyphen-to-underscore step remotli applies to the identical case format.
		EmploymentType: schemaEmploymentType(strings.ReplaceAll(p.Type, "-", "_")),
		PostedAt:       parseRFC3339(firstNonEmpty(p.PublishedAt, p.CreatedAt)),
	}, true
}

// landingjobsCompanyFromURL humanizes the employer out of a posting URL.
//
// `https://landing.jobs/at/acme-corp/senior-go-engineer` yields "Acme Corp". Both path segments
// must be present: `/at/acme-corp` alone is the company's own page rather than a posting, and
// treating it as one would file a job under a URL that never named it. ok is false otherwise.
//
// The last check is on the humanized NAME rather than on the slug it came from. A slug that is
// all separators ("---") is non-empty and clears every path check, but humanizes to nothing —
// and ok=true with no name is the one outcome this function exists to prevent, since the caller
// reads it as "the employer resolved" and would emit a posting with no company to slug.
func landingjobsCompanyFromURL(rawURL string) (company string, ok bool) {
	_, after, found := strings.Cut(rawURL, landingjobsPathMarker)
	if !found {
		return "", false
	}
	// Trim a query/fragment before splitting so neither lands inside a path segment.
	after = strings.TrimSpace(after)
	if i := strings.IndexAny(after, "?#"); i >= 0 {
		after = after[:i]
	}
	parts := strings.Split(strings.Trim(after, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", false
	}
	company = landingjobsCompany(parts[0])
	return company, company != ""
}

// landingjobsCompany humanizes a company slug into a display name: hyphens and underscores
// become spaces and each word is capitalized ("acme-corp" → "Acme Corp").
//
// Only the first rune of a word is touched. Upper-casing the whole word would shout every name,
// and lower-casing the tail would flatten the ones the slug preserves ("gitHub" → "Github"),
// so a word already carrying internal capitals keeps them.
func landingjobsCompany(slug string) string {
	words := strings.FieldsFunc(slug, func(r rune) bool {
		return r == '-' || r == '_' || unicode.IsSpace(r)
	})
	for i, w := range words {
		runes := []rune(w)
		runes[0] = unicode.ToUpper(runes[0])
		words[i] = string(runes)
	}
	return strings.Join(words, " ")
}

// landingjobsLocation renders every place the posting states, not just the first: a role open in
// Munich, Lisbon and Cologne says so in the feed, and keeping one city would hide it from a
// search for the others. "Remote" is appended when the posting is flagged remote, and stands
// alone when there is no place at all (the array is null for fully-remote roles).
func landingjobsLocation(p landingjobsPosting) string {
	places := distinctJoin(p.Locations, "; ", landingjobsPlace.label)
	if !p.Remote {
		return places
	}
	if places == "" {
		return "Remote"
	}
	return places + "; Remote"
}

// landingjobsCountries normalizes EVERY location's country code into Job.Countries. They are
// structured fields rather than tokens mined from location text, which is what licenses setting
// them at all; unresolved and duplicate codes drop out, and nothing resolvable yields nil so the
// dictionary decides instead.
func landingjobsCountries(places []landingjobsPlace) []string {
	codes := make([]string, 0, len(places))
	for _, p := range places {
		codes = append(codes, p.CountryCode)
	}
	return countriesFromCodes(codes)
}

// landingjobsDescription stitches the posting's HTML sections into one body, heading each named
// section and dropping the empty ones, then sanitizes the result. The role description leads
// unheaded — it is the body proper, and a "Description" heading above the first paragraph reads
// as chrome.
func landingjobsDescription(p landingjobsPosting) string {
	var b strings.Builder
	section := func(heading, body string) {
		if strings.TrimSpace(body) == "" {
			return
		}
		if heading != "" {
			b.WriteString("<h3>")
			b.WriteString(heading)
			b.WriteString("</h3>")
		}
		b.WriteString(body)
	}
	section("", p.RoleDescription)
	section("Requirements", p.MainRequirements)
	section("Nice to have", p.NiceToHave)
	section("Perks", p.Perks)
	return sanitizeHTML(b.String())
}
