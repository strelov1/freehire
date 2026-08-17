package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
)

// remotedotcom adapts remote.com's public board, a curated remote-jobs aggregator (~5.7k live
// postings). Boardless (one public listing, no per-tenant board) and multi-company, so it stays
// in the source facet and takes each posting's employer from the listing.
//
// remote.com is a Next.js App Router app, so there is no REST API and no Pages-Router
// _next/data endpoint — the data is the RSC flight. Sending "RSC: 1" makes both the listing and
// a posting page answer text/x-component instead of HTML, which is both the only machine
// readable shape and much the cheaper one (a listing page measured 33 KB against 389 KB of
// equivalent HTML, a posting page 20-60 KB against 144 KB).
//
// The two shapes carry different halves of a posting and are read differently:
//
//   - the LISTING inlines a "jobsData" object as plain JSON — every posting's identity,
//     employer, geography, seniority and compensation, and the totalPages that bounds the walk —
//     but no body at all.
//   - a POSTING PAGE carries the body, as a schema.org JobPosting in a flight TEXT ROW (not an
//     application/ld+json <script>, which the RSC response has no HTML to hold).
//
// That split is why this is a HydratingSource: a body costs one request per posting, and a
// posting with no body is excluded from the search index (search.DescriptionMissing), so the
// listing alone would ingest a catalogue nobody can find. Fetching the body only for postings
// the catalogue does not already have makes an ordinary run cost a request per NEW posting.
type remotedotcom struct {
	http HeaderTextGetter
}

const (
	// remotedotcomListURL is the board's listing, paginated by ?page=N (20 postings a page).
	remotedotcomListURL = "https://remote.com/jobs/all?page=%d"
	// remotedotcomJobURL is both the public posting page and, with the RSC header, its data
	// endpoint. It takes the company slug then the posting slug.
	remotedotcomJobURL = "https://remote.com/jobs/%s/%s"
	// remotedotcomMaxPages bounds the walk against a listing that stops honouring totalPages.
	// The walk normally ends on totalPages (288 at 5,745 postings, confirmed live 2026-08-16);
	// this is the backstop that keeps a broken response from paging forever, sized with room
	// for the board to grow several times over.
	remotedotcomMaxPages = 2000
)

// remotedotcomRSC is the header that makes remote.com answer with its RSC flight rather than
// the rendered page. Next-Router-State-Tree is NOT sent: the flight comes back complete
// without it (confirmed live across listing and posting pages, 2026-08-16), and a router state
// tree is a client-side cache key we would only be guessing at.
var remotedotcomRSC = map[string]string{"RSC": "1"}

// NewRemotedotcom builds the remote.com adapter over the given HTTP client.
func NewRemotedotcom(c HeaderTextGetter) Source { return remotedotcom{http: c} }

func (remotedotcom) Provider() string { return "remotedotcom" }

// remote.com is one public listing with no per-tenant board id.
func (remotedotcom) boardless() {}

// remote.com aggregates postings from many employers, so it stays in the source facet.
func (remotedotcom) aggregator() {}

// remotedotcomListing is the listing's "jobsData" object. totalPages bounds the walk exactly,
// so the adapter never guesses a page budget.
type remotedotcomListing struct {
	Jobs       []remotedotcomPosting `json:"jobs"`
	TotalPages int                   `json:"totalPages"`
}

// remotedotcomPlace is one named place in the listing: a country (code is ISO alpha-3) or a
// multi-country region such as "north-america", whose code is remote.com's own vocabulary and
// resolves to no country.
type remotedotcomPlace struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

// remotedotcomIncludedLocation is one entry of a hiring location's explicit set: a country
// (Value.Code is ISO alpha-3) or a region, whose Code is remote.com's own vocabulary.
type remotedotcomIncludedLocation struct {
	Type  string            `json:"type"`
	Value remotedotcomPlace `json:"value"`
}

// remotedotcomPosting is one posting as the listing states it. Both location fields and
// compensation are nullable, and describe different things — see location and applySalary.
type remotedotcomPosting struct {
	Status         string   `json:"status"`
	Slug           string   `json:"slug"`
	Title          string   `json:"title"`
	PublishedAt    string   `json:"publishedAt"`
	EmploymentType string   `json:"employmentType"`
	Seniority      []string `json:"seniority"`
	CompanyProfile struct {
		Name string `json:"name"`
		Slug string `json:"slug"`
	} `json:"companyProfile"`
	// WorkplaceLocation is where the desk is: type remote/hybrid/on_site, with country and
	// city filled for the latter two.
	WorkplaceLocation *struct {
		Type    string             `json:"type"`
		City    string             `json:"city"`
		Country *remotedotcomPlace `json:"country"`
	} `json:"workplaceLocation"`
	// HiringLocation is where the candidate may live: type global (anywhere), location (an
	// explicit set of countries or regions) or timezone (a named zone plus a ± hour range).
	HiringLocation *struct {
		Type     string `json:"type"`
		Timezone *struct {
			Name string `json:"name"`
		} `json:"timezone"`
		IncludedLocations []remotedotcomIncludedLocation `json:"includedLocations"`
	} `json:"hiringLocation"`
	Compensation *struct {
		Minimum   float64 `json:"minimum"`
		Maximum   float64 `json:"maximum"`
		Frequency string  `json:"frequency"`
		Currency  struct {
			Code string `json:"code"`
		} `json:"currency"`
	} `json:"compensation"`
}

// remotedotcomJobPosting is the subset of the posting page's schema.org JobPosting the adapter
// reads. Everything else it states (title, employer, salary, dates) the listing already carries
// structurally, so only the body — which the listing omits entirely — is taken from here.
type remotedotcomJobPosting struct {
	Description string `json:"description"`
}

// Fetch is the list-only fallback used when the pipeline cannot supply a seen set. It yields
// every posting without a body; FetchNew is the crawl that hydrates.
func (s remotedotcom) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	postings, err := s.crawl(ctx)
	if err != nil {
		return nil, err
	}
	jobs := make([]Job, 0, len(postings))
	for _, p := range postings {
		if job, ok := p.toJob(); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// FetchNew is the hydrating crawl: it lists the whole board, but fetches a posting page only
// for a slug the catalogue does not already have. A seen posting is emitted as a liveness
// refresh — no request, and no content rewrite, which would wipe the description and the facets
// derived from it.
func (s remotedotcom) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.crawl(ctx)
	if err != nil {
		return nil, err
	}
	return fetchDetails(postings, defaultDetailWorkers, func(p remotedotcomPosting) (Job, bool) {
		base, ok := p.toJob()
		if !ok {
			return Job{}, false // unusable posting — dropped, as in Fetch
		}
		if seen(p.Slug) {
			base.SeenRefresh = true
			return base, true
		}
		body, ok := s.body(ctx, p)
		if !ok {
			// The listing is authoritative for the posting existing, so a failed body keeps
			// it; pipeline.HydrationRetryWindow re-offers the row for hydration later.
			log.Printf("remotedotcom: body %q failed; ingesting list-only", p.Slug)
			return base, true
		}
		base.Description = sanitizeHTML(body)
		return base, true
	}), nil
}

// crawl walks the listing and returns every posting. It obeys the repo-wide paginated-walk
// rule: the FIRST page failing is a board-level error, a LATER page failing ends the walk with
// what it has, so a mid-listing hiccup costs a slice of the board rather than the whole run.
func (s remotedotcom) crawl(ctx context.Context) ([]remotedotcomPosting, error) {
	var out []remotedotcomPosting
	for page := 1; page <= remotedotcomMaxPages; page++ {
		listing, err := s.listing(ctx, page)
		if err != nil {
			if page == 1 {
				return nil, fmt.Errorf("remotedotcom: page %d: %w", page, err)
			}
			log.Printf("remotedotcom: page %d: %v; keeping %d postings", page, err, len(out))
			break
		}
		// A page past the end answers 200 with an empty jobs array rather than an error, so
		// this is the honest stop even when totalPages is missing or stale.
		if len(listing.Jobs) == 0 {
			break
		}
		out = append(out, listing.Jobs...)
		if listing.TotalPages > 0 && page >= listing.TotalPages {
			break
		}
	}
	return out, nil
}

// listing fetches one listing page and decodes its embedded jobsData object. The object sits
// inline in the flight rather than as a document of its own, so it is isolated by a
// string-aware brace scan — a literal brace inside a title must not be counted as structure.
func (s remotedotcom) listing(ctx context.Context, page int) (remotedotcomListing, error) {
	flight, err := s.http.GetTextWithHeaders(ctx, fmt.Sprintf(remotedotcomListURL, page), remotedotcomRSC)
	if err != nil {
		return remotedotcomListing{}, err
	}
	raw, ok := bracketSlice(flight, `"jobsData":`, '{', '}')
	if !ok {
		// A markup change must surface loudly rather than silently empty the board.
		return remotedotcomListing{}, fmt.Errorf("jobsData not found")
	}
	var out remotedotcomListing
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return remotedotcomListing{}, fmt.Errorf("decode jobsData: %w", err)
	}
	return out, nil
}

// body fetches a posting page's flight and returns the description its schema.org JobPosting
// carries. ok=false covers both a failed request and a page with no JobPosting, and neither
// drops the posting — the caller ingests it list-only.
func (s remotedotcom) body(ctx context.Context, p remotedotcomPosting) (string, bool) {
	flight, err := s.http.GetTextWithHeaders(ctx, p.url(), remotedotcomRSC)
	if err != nil {
		return "", false
	}
	var posting remotedotcomJobPosting
	if !flightJobPosting(flight, &posting) {
		return "", false
	}
	return posting.Description, posting.Description != ""
}

// url is the posting's public page, which is also its data endpoint under the RSC header.
func (p remotedotcomPosting) url() string {
	return fmt.Sprintf(remotedotcomJobURL, p.CompanyProfile.Slug, p.Slug)
}

// toJob maps a listed posting to a Job, without a body. ok=false for a posting that is not
// published, or that lacks the slug (the dedup key) or the employer (which the slug is built
// from).
func (p remotedotcomPosting) toJob() (Job, bool) {
	if p.Status != "published" || p.Slug == "" || p.CompanyProfile.Name == "" {
		return Job{}, false
	}
	job := Job{
		ExternalID:     p.Slug,
		URL:            p.url(),
		Title:          p.Title,
		Company:        p.CompanyProfile.Name,
		Location:       p.location(),
		WorkMode:       remotedotcomWorkMode(p.workplaceType()),
		Seniority:      remotedotcomSeniority(p.Seniority),
		EmploymentType: remotedotcomEmploymentType(p.EmploymentType),
		Countries:      countriesFromCodes(p.countryCodes()),
		PostedAt:       parseRFC3339(p.PublishedAt),
	}
	job.Remote = job.WorkMode == "remote"
	p.applySalary(&job)
	return job, true
}

// workplaceType reports the workplace arrangement remote.com states, or "" when it states none.
func (p remotedotcomPosting) workplaceType() string {
	if p.WorkplaceLocation == nil {
		return ""
	}
	return p.WorkplaceLocation.Type
}

// location renders the posting's place as free text. Two independent fields describe it and
// they answer different questions: the WORKPLACE is where the desk is, the HIRING LOCATION is
// where the candidate may live. A hybrid or on-site posting is described by its desk, so that
// wins where it is stated; a remote one has no desk and is described by who may apply.
func (p remotedotcomPosting) location() string {
	if w := p.WorkplaceLocation; w != nil && w.Type != "" && w.Type != "remote" {
		country := ""
		if w.Country != nil {
			country = w.Country.Name
		}
		if s := joinNonEmpty(w.City, country); s != "" {
			return s
		}
	}
	h := p.HiringLocation
	if h == nil {
		return ""
	}
	switch h.Type {
	case "global":
		// The location dictionary reads "worldwide" as the global region, so saying so puts
		// an anywhere-posting in that bucket instead of leaving it placeless.
		return "Worldwide"
	case "location":
		// Both country and region entries are rendered — a role open across "Latin America"
		// says something a country list cannot, even though only countries resolve to codes.
		return distinctJoin(h.IncludedLocations, ", ", func(l remotedotcomIncludedLocation) string {
			return l.Value.Name
		})
	case "timezone":
		if h.Timezone != nil && h.Timezone.Name != "" {
			// The zone is named after a city ("Chicago" for america-chicago), and that name
			// alone is a claim the posting does not make: the location dictionary reads a bare
			// "Chicago" as the city Chicago in the US, filing a work-from-anywhere-in-this-zone
			// posting under a place nobody has to be. Naming it a timezone is both what the
			// posting says and, because the dictionary matches whole phrases, what stops the
			// city from being invented.
			return h.Timezone.Name + " timezone"
		}
	}
	return ""
}

// countryCodes returns the ISO alpha-3 codes the posting states structurally. A hybrid or
// on-site posting states one country (where the desk is); a remote one states the set the
// candidate may live in. Region entries ("north-america") carry remote.com's own vocabulary
// rather than a country code and are left to the location dictionary, which reads the region
// out of the free-text location instead.
func (p remotedotcomPosting) countryCodes() []string {
	if w := p.WorkplaceLocation; w != nil && w.Type != "" && w.Type != "remote" && w.Country != nil {
		return []string{w.Country.Code}
	}
	h := p.HiringLocation
	if h == nil {
		return nil
	}
	var codes []string
	for _, l := range h.IncludedLocations {
		if l.Type == "country" {
			codes = append(codes, l.Value.Code)
		}
	}
	return codes
}

// applySalary maps remote.com's structured compensation onto the job's salary fields.
//
// The bounds are MINOR units. The listing states one posting as 1000000-1500000 USD/yearly
// while that same posting's schema.org baseSalary states 10000-15000 (confirmed live
// 2026-08-16), so reading the listing verbatim would inflate every salary a hundredfold —
// which is also why the salary is not folded into the description as text: this is a
// structured field, and Job's structured salary is where a structured field belongs.
func (p remotedotcomPosting) applySalary(job *Job) {
	c := p.Compensation
	if c == nil || c.Currency.Code == "" {
		return
	}
	period := remotedotcomSalaryPeriod(c.Frequency)
	if period == "" {
		return
	}
	min, max := roundSalaryPart(c.Minimum/100), roundSalaryPart(c.Maximum/100)
	if min == nil && max == nil {
		return
	}
	job.SalaryMin, job.SalaryMax = min, max
	job.SalaryCurrency, job.SalaryPeriod = c.Currency.Code, period
}

// remotedotcomWorkMode maps remote.com's workplace type onto freehire's work-mode vocabulary.
func remotedotcomWorkMode(t string) string {
	switch t {
	case "remote":
		return "remote"
	case "hybrid":
		return "hybrid"
	case "on_site":
		return "onsite"
	default:
		return ""
	}
}

// remotedotcomSalaryPeriod maps remote.com's compensation frequency onto
// vocab.SalaryPeriodValues. Only yearly, monthly and hourly were observed live; daily is
// mapped because freehire names it, and anything else — a weekly rate, which freehire's
// vocabulary cannot express — yields "" and drops the whole salary rather than filing an
// amount under a period it does not have.
func remotedotcomSalaryPeriod(frequency string) string {
	switch frequency {
	case "yearly":
		return "year"
	case "monthly":
		return "month"
	case "daily":
		return "day"
	case "hourly":
		return "hour"
	default:
		return ""
	}
}

// remotedotcomEmploymentType maps remote.com's employment type onto
// vocab.EmploymentTypeValues. Only full_time and contractor were observed live.
func remotedotcomEmploymentType(t string) string {
	switch t {
	case "full_time":
		return "full_time"
	case "part_time":
		return "part_time"
	case "contractor":
		return "contract"
	case "internship":
		return "internship"
	default:
		return ""
	}
}

// remotedotcomSeniority maps remote.com's seniority onto vocab.SeniorityValues. The field is
// an array but every posting observed live carries at most one value, so the first recognized
// one is taken.
//
// The management rungs need a decision freehire's ladder does not make for us: it names lead,
// staff and principal, of which only lead is a management rank. classify's own title dictionary
// already answers it the same way — it reads "head of", "vice president" and "директор" as
// c_level — so director and executive land there and manager lands on lead. These come from a
// STRUCTURED rank field, not from guessing at a title, which is what makes the mapping sound
// where reading "Manager" out of a job title would not be.
func remotedotcomSeniority(values []string) string {
	for _, v := range values {
		switch v {
		case "entry_level":
			return "junior"
		case "mid_level":
			return "middle"
		case "senior":
			return "senior"
		case "manager":
			return "lead"
		case "director", "executive":
			return "c_level"
		}
	}
	return ""
}
