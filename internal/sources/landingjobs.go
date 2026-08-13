package sources

import (
	"context"
	"fmt"
	"strings"
	"unicode"
)

// landingjobs adapts landing.jobs, an EU tech job board. Boardless (one public API, no
// per-tenant board) and multi-company, so it stays in the source facet and takes each
// posting's employer from the posting rather than from the config entry.
//
// Two shapes make it unlike its neighbours.
//
// **Neither the company nor a posting id is in the payload.** Both are derived from the
// posting URL, which is `https://landing.jobs/at/<company-slug>/<job-slug>`: the employer from
// the first segment, humanized, and the external id from both segments joined — the only stable
// identity the feed offers. A posting whose URL does not have that shape is skipped rather than
// ingested under a guessed identity, since the id is the dedup key.
//
// **One request, and no pagination.** `?page=N` is ignored by the endpoint — pages 1 and 2 come
// back byte-for-byte identical (verified 2026-08-08, issue #1627) — so a page walk would
// re-fetch the same postings until its own cap and dedup them all away, paying N requests for
// one page of data. The single response is therefore taken as the whole feed. Whether the ~50
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
// The feed also carries `tags`, `type`, `gross_salary_low`/`gross_salary_high` and
// `currency_code`, none of which are declared here. Salary has no home on Job (enrichment owns
// it), and the element shape of `tags` and the vocabulary of `type` are undocumented and were
// not verified live — declaring a wrong Go type for either would fail the decode of the WHOLE
// feed, not just that field, so both are left to the pipeline's own dictionaries. Adding them is
// a small change once the shapes are confirmed against the live endpoint.
type landingjobsPosting struct {
	Title            string             `json:"title"`
	URL              string             `json:"url"`
	Locations        []landingjobsPlace `json:"locations"`
	Remote           bool               `json:"remote"`
	PublishedAt      string             `json:"published_at"`
	CreatedAt        string             `json:"created_at"`
	RoleDescription  string             `json:"role_description"`
	MainRequirements string             `json:"main_requirements"`
	NiceToHave       string             `json:"nice_to_have"`
	Perks            string             `json:"perks"`
}

// landingjobsPlace is one entry of a posting's locations. The array is null for a fully-remote
// role, so every read of it goes through the empty check in landingjobsLocation.
type landingjobsPlace struct {
	City        string `json:"city"`
	CountryCode string `json:"country_code"`
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

// toJob maps a posting to a Job, returning ok=false when the URL does not yield an identity —
// without it there is no dedup key and no employer, and a posting ingested under a fabricated
// one would be re-inserted on every crawl.
func (p landingjobsPosting) toJob() (Job, bool) {
	company, id, ok := landingjobsIdentity(p.URL)
	if !ok || p.Title == "" {
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
		ExternalID:  id,
		URL:         p.URL,
		Title:       p.Title,
		Company:     company,
		Location:    landingjobsLocation(p),
		Description: landingjobsDescription(p),
		Remote:      p.Remote,
		WorkMode:    mode,
		Countries:   landingjobsCountries(p.Locations),
		PostedAt:    parseRFC3339(firstNonEmpty(p.PublishedAt, p.CreatedAt)),
	}, true
}

// landingjobsIdentity splits a posting URL into its employer and its stable id.
//
// `https://landing.jobs/at/acme-corp/senior-go-engineer` yields ("Acme Corp",
// "acme-corp/senior-go-engineer"). Both segments go into the id because the job slug alone is
// not unique across employers — two companies may both post "backend-engineer" — and the id is
// the dedup key. ok is false for any URL without both segments.
func landingjobsIdentity(rawURL string) (company, id string, ok bool) {
	_, after, found := strings.Cut(rawURL, landingjobsPathMarker)
	if !found {
		return "", "", false
	}
	// Trim a query/fragment before splitting so neither lands inside the job slug.
	after = strings.TrimSpace(after)
	if i := strings.IndexAny(after, "?#"); i >= 0 {
		after = after[:i]
	}
	parts := strings.Split(strings.Trim(after, "/"), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return landingjobsCompany(parts[0]), parts[0] + "/" + parts[1], true
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

// landingjobsLocation renders the posting's place as free text for the location dictionary:
// the first entry as "City, CC", with "Remote" appended when the posting is flagged remote and
// standing alone when there is no place at all (the array is null for fully-remote roles).
func landingjobsLocation(p landingjobsPosting) string {
	place := ""
	if len(p.Locations) > 0 {
		place = strings.TrimSpace(strings.Trim(
			p.Locations[0].City+", "+p.Locations[0].CountryCode, " ,"))
	}
	if !p.Remote {
		return place
	}
	if place == "" {
		return "Remote"
	}
	return place + ", Remote"
}

// landingjobsCountries normalizes the first location's country code into Job.Countries. It is a
// structured field rather than a token mined from the location text, which is what licenses
// setting it at all; an unresolved or absent code yields nil so the dictionary decides instead.
func landingjobsCountries(places []landingjobsPlace) []string {
	if len(places) == 0 {
		return nil
	}
	return countryFromCode(places[0].CountryCode)
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
