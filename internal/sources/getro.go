package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"slices"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/enrich"
	"github.com/strelov1/freehire/internal/skilltag"
)

// getro adapts Getro-powered job boards (<label>.getro.com), the platform many VC/accelerator
// portfolios use to publish their companies' openings (e.g. jobsinvc.getro.com). The board is
// the numeric network id — the search API rejects the human-readable subdomain and demands an
// integer collection id. It is an aggregator, not a single company: each posting carries its own
// employer (organization.name) and its url points at the company's first-party ATS
// (greenhouse/lever/workday/…), so it stays in the source facet and its copies are suppressed in
// favour of the first-party posting when freehire also crawls that ATS directly.
//
// The search listing carries rich STRUCTURED facets (work_mode, seniority, skills, locations) but
// no description; the description lives only in the SSR detail page's __NEXT_DATA__. So — like
// justjoin/nofluffjobs — getro is a HydratingSource: FetchNew fetches a posting's detail only for
// postings the catalogue does not already have, and re-lists the rest as liveness refreshes.
type getro struct {
	http getroHTTP
}

// getroHTTP is the transport getro needs: the POST search listing (JSONPoster), the collection
// metadata GET that resolves the board's subdomain (JSONGetter), and the SSR detail page whose
// __NEXT_DATA__ carries the description (HTMLGetter).
type getroHTTP interface {
	JSONPoster
	JSONGetter
	HTMLGetter
}

// NewGetro builds the Getro adapter over the given HTTP client.
func NewGetro(c getroHTTP) Source { return getro{http: c} }

func (getro) Provider() string { return "getro" }

// getro aggregates postings from many companies (company per posting), so it stays in the source
// facet and its ATS-duplicate copies are suppressed by the cross-source dedup pass. It is NOT
// boardless — the board is the numeric network id the search API requires.
func (getro) aggregator() {}

const (
	// getroSearchURL is the public job-search endpoint; the path segment is the numeric network
	// (collection) id. It is POST-only and paginates by a 0-based page with a hits_per_page size.
	getroSearchURL = "https://api.getro.com/api/v2/collections/%s/search/jobs"
	// getroMetaURL is the collection metadata; its data.attributes.label is the board subdomain
	// (<label>.getro.com) the SSR detail page is served from.
	getroMetaURL = "https://api.getro.com/api/v2/collections/%s"
	// getroDetailURL is the SSR job page carrying the description. The company path segment is
	// cosmetic — the page resolves off the job slug (which embeds the job id) — but the real
	// organization slug is used when the listing provides it.
	getroDetailURL = "https://%s.getro.com/companies/%s/jobs/%s"
	// getroPageSize is the listing page size. The API caps the returned page but honours a large
	// hits_per_page; 100 keeps each response modest while bounding the page count.
	getroPageSize = 100
	// getroMaxPages bounds pagination so a server that mis-reports count (never emptying its data)
	// cannot loop forever; the count/empty-page check ends it sooner.
	getroMaxPages = 200
)

// getroSearchResponse is one search page: the postings plus the network-wide total (count), used
// to stop paging once every posting has been collected.
type getroSearchResponse struct {
	Results struct {
		Jobs  []getroPosting `json:"jobs"`
		Count int            `json:"count"`
	} `json:"results"`
}

// getroPosting is one listing entry. url is already the first-party ATS apply link; created_at is
// epoch seconds; slug embeds the job id and drives the detail URL.
type getroPosting struct {
	ID           int64    `json:"id"`
	Slug         string   `json:"slug"`
	Title        string   `json:"title"`
	URL          string   `json:"url"`
	WorkMode     string   `json:"work_mode"`
	Seniority    string   `json:"seniority"`
	Skills       []string `json:"skills"`
	Locations    []string `json:"locations"`
	CreatedAt    int64    `json:"created_at"`
	Organization struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	} `json:"organization"`
}

// Fetch is the list-only crawl (no description): kept as the fallback for callers that do not
// drive hydration. FetchNew is the hydrating path used by ingest.
func (s getro) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	postings, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	for _, p := range postings {
		if job, ok := p.toJob(); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// FetchNew is the hydrating crawl: it lists the same board, but fetches a posting's detail (the
// description the listing omits) only for a posting the catalogue does not already have — seen
// reports whether a posting's id is already ingested. A seen posting yields the list-only job as a
// liveness refresh (no detail request); an unseen posting is hydrated with its description; a
// single detail failure is isolated (logged, falling back to list-only so the posting is still
// ingested). Detail fetches run under the shared bounded worker pool.
func (s getro) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	// Resolve the board's subdomain once — every detail page is served from it. If it cannot be
	// resolved the crawl degrades to list-only (detail returns ok=false for an empty label), so a
	// metadata hiccup never fails the board.
	label := s.label(ctx, e.Board)
	return fetchDetails(postings, defaultDetailWorkers, func(p getroPosting) (Job, bool) {
		base, ok := p.toJob()
		if !ok {
			return Job{}, false // unusable posting — dropped, as in Fetch
		}
		if seen(base.ExternalID) {
			// Already ingested: refresh liveness only, no detail request. The pipeline must not
			// re-upsert content — an empty description would re-derive its facets to empty. base
			// carries just the identity fields toJob set.
			base.SeenRefresh = true
			return base, true
		}
		desc, ok := s.detail(ctx, label, p)
		if !ok {
			log.Printf("getro: detail %d failed; ingesting list-only", p.ID)
			return base, true
		}
		base.Description = desc
		return base, true
	}), nil
}

// list pages the search endpoint (page size getroPageSize, 0-based) until the reported count is
// reached or a page returns no postings, whichever comes first. Each page carries the postings
// with their structured facets inline.
func (s getro) list(ctx context.Context, board string) ([]getroPosting, error) {
	url := fmt.Sprintf(getroSearchURL, board)
	var all []getroPosting
	for page := 0; page < getroMaxPages; page++ {
		var resp getroSearchResponse
		body := map[string]int{"hits_per_page": getroPageSize, "page": page}
		if err := s.http.PostJSON(ctx, url, body, &resp); err != nil {
			return nil, fmt.Errorf("getro: search page %d: %w", page, err)
		}
		all = append(all, resp.Results.Jobs...)
		if len(resp.Results.Jobs) == 0 || len(all) >= resp.Results.Count {
			break
		}
	}
	return all, nil
}

// label resolves the board's subdomain from the collection metadata (data.attributes.label). It
// returns "" on any failure, which degrades FetchNew to list-only rather than failing the board.
func (s getro) label(ctx context.Context, board string) string {
	var resp struct {
		Data struct {
			Attributes struct {
				Label string `json:"label"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := s.http.GetJSON(ctx, fmt.Sprintf(getroMetaURL, board), &resp); err != nil {
		return ""
	}
	return resp.Data.Attributes.Label
}

// getroNextData is the slice of the SSR detail page's __NEXT_DATA__ we read: the current job's
// description (HTML). Only the fields we render are modelled; encoding/json ignores the rest.
type getroNextData struct {
	Props struct {
		PageProps struct {
			InitialState struct {
				Jobs struct {
					CurrentJob struct {
						Description string `json:"description"`
					} `json:"currentJob"`
				} `json:"jobs"`
			} `json:"initialState"`
		} `json:"pageProps"`
	} `json:"props"`
}

// detail fetches a posting's SSR page and extracts its description from the __NEXT_DATA__ payload,
// returning ok=false when the label/slug is missing, the fetch fails, the script is absent or
// unparseable, or the job carries no description — so the caller falls back to the list-only job.
func (s getro) detail(ctx context.Context, label string, p getroPosting) (string, bool) {
	if label == "" || p.Slug == "" {
		return "", false
	}
	url := fmt.Sprintf(getroDetailURL, label, firstNonEmpty(p.Organization.Slug, "-"), p.Slug)
	root, err := s.http.GetHTML(ctx, url)
	if err != nil {
		return "", false
	}
	raw := scriptTextByID(root, "__NEXT_DATA__")
	if raw == "" {
		return "", false
	}
	var data getroNextData
	if json.Unmarshal([]byte(raw), &data) != nil {
		return "", false
	}
	desc := sanitizeHTML(data.Props.PageProps.InitialState.Jobs.CurrentJob.Description)
	if desc == "" {
		return "", false
	}
	return desc, true
}

// toJob maps a listing posting to a Job, returning ok=false for an unusable posting (no id to key
// on, no url to link to, or no company which would break the slug). The structured facets Getro
// states — work_mode, seniority, skills — are mapped into freehire's vocabularies; the description
// is left for detail hydration.
func (p getroPosting) toJob() (Job, bool) {
	if p.ID == 0 || p.URL == "" || p.Organization.Name == "" {
		return Job{}, false
	}
	location := distinctJoin(p.Locations, "; ", func(s string) string { return s })
	workMode := getroWorkMode(p.WorkMode)
	return Job{
		ExternalID: strconv.FormatInt(p.ID, 10),
		URL:        p.URL,
		Title:      strings.TrimSpace(p.Title),
		Company:    strings.TrimSpace(p.Organization.Name),
		Location:   location,
		WorkMode:   workMode,
		Remote:     workMode == "remote" || isRemote(location),
		Seniority:  getroSeniority(p.Seniority),
		Skills:     skilltag.Parse(strings.Join(p.Skills, " ")),
		PostedAt:   parseEpochSeconds(p.CreatedAt),
	}, true
}

// getroWorkMode maps Getro's work_mode enum into freehire's work-mode vocabulary. Getro spells the
// office value with an underscore ("on_site"); normalising it to a hyphen lets the shared
// workplaceTypeMode do the rest (remote/hybrid/on-site → remote/hybrid/onsite, unknown → "").
func getroWorkMode(m string) string {
	return workplaceTypeMode(strings.ReplaceAll(m, "_", "-"))
}

// getroSeniority maps Getro's LinkedIn-style seniority grade into freehire's vocabulary
// (enrich.SeniorityValues: intern/junior/middle/senior/lead/staff/principal/c_level). It maps to a
// candidate then checks vocabulary membership, so an unmapped or mis-mapped grade drops to "" and
// lets the title dictionary decide rather than being guessed.
func getroSeniority(level string) string {
	var l string
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "internship":
		l = "intern"
	case "entry_level":
		l = "junior"
	case "senior":
		l = "senior"
	case "cxo":
		l = "c_level"
		// Getro's remaining grades — "associate", "mid_senior", "director", "vice_president" — are
		// LinkedIn's cross-functional levels with no exact twin in freehire's IC-oriented ladder, so
		// they are deliberately left unmapped: the title dictionary classifies them from the job
		// title instead of this adapter guessing (per the dict-only "never guess unknowns" convention).
	}
	if slices.Contains(enrich.SeniorityValues, l) {
		return l
	}
	return ""
}
