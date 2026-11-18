// Package jobview defines the single public wire shape of a job, served by the list,
// detail and search endpoints and stored in the search index. One type keeps the
// surfaces from drifting.
package jobview

import (
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/dict/location"
	"github.com/strelov1/freehire/internal/dict/vocab"
	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/outboundurl"
	"github.com/strelov1/freehire/internal/job/salaryextract"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// Job is the public wire shape of a job. It carries public_slug and deliberately omits
// the numeric id, which is enumerable and its growth leaks inventory size and fill rate.
//
// Enrichment is nested and typed; an unenriched job serializes it as `{}`. Timestamps
// are RFC3339 UTC strings (or null), which sort chronologically as the index relies on.
type Job struct {
	PublicSlug string `json:"public_slug"`
	Source     string `json:"source"`
	// ManuallyAdded marks a hand-curated posting added by a moderator (created_by set),
	// decoupled from Source. The authoring user id is never exposed.
	ManuallyAdded bool   `json:"manually_added"`
	ExternalID    string `json:"external_id"`
	URL           string `json:"url"`
	// SourceURL is the original posting when URL points to a separately verified
	// application destination. Omitted when both links are the same.
	SourceURL   string `json:"source_url,omitempty"`
	Title       string `json:"title"`
	Company     string `json:"company"`
	CompanySlug string `json:"company_slug"`
	Location    string `json:"location"`
	Description string `json:"description"`
	// DirectApply marks an employer vacancy that takes applications on freehire instead
	// of linking out (job.IsDirectApply); its URL is empty. Omitted rather than served
	// false, to keep the search index small.
	DirectApply bool `json:"direct_apply,omitempty"`
	// WorkMode/Skills are served from the jobs dictionary columns ONLY; the LLM's values
	// stay raw in the stored enrichment JSONB, so the LLM can run free as a discovery
	// signal without corrupting production data.
	//
	// Countries/Regions are a hybrid facet, like Cities (see geoFacet): the dictionary
	// columns win whenever they pinned a place; otherwise the LLM's enrichment.countries/
	// regions fill in, catching a restriction stated only in prose ("Remote (SPAIN only)").
	//
	// All four are served top-level and folded out of the nested Enrichment.
	Countries []string `json:"countries"`
	Regions   []string `json:"regions"`
	WorkMode  string   `json:"work_mode,omitempty"`
	Skills    []string `json:"skills"`
	// Cities is a hybrid facet (the one deliberate dict+LLM exception): the dictionary
	// column when it resolved a beacon city, otherwise a normalized fallback to the LLM's
	// enrichment.cities, since the dictionary is only a beacon list and would be too
	// sparse. Values are canonical display names (Title case).
	Cities []string `json:"cities"`
	// Metros is dict-only: the metro areas (GHS-FUA) the location line places the posting in, "Sacramento (US)".
	Metros []string `json:"metros"`
	// Collections is the curated-collection slugs (e.g. yc, bigtech) of the job's
	// company, denormalized onto the job. A deterministic source fact; an untagged job
	// serializes it as [].
	Collections []string `json:"collections"`
	// Perks are the benefits the description states it offers, from vocab.PerkValues in
	// its order (internal/dict/perks, never the LLM). Always an array: [] means the
	// description names none, not that the job offers none.
	Perks []string `json:"perks"`
	// Engagement names the legal terms the posting states it engages its worker on, from
	// vocab.EngagementValues (internal/dict/engagement, never the LLM), each in its
	// market's own term (ir35_outside, w2, b2b, clt, gph...). Always an array: [] means
	// the posting does not say.
	Engagement []string `json:"engagement"`
	// AIInterviewReports is how many people reported that this job's company screens with
	// an AI interviewer (internal/engage/processreport), denormalized from the company.
	// Omitted at zero, so "no reports" is an absent field rather than a zero a badge
	// could render.
	AIInterviewReports int32 `json:"ai_interview_reports,omitempty"`
	// IsTech is the deterministic "tech" or "non_tech" facet, derived from title +
	// category (jobderive), never the LLM. Omitted when unknown (jobs.is_tech NULL) so it
	// filters as empty.
	IsTech string `json:"is_tech,omitempty"`
	// RequiresClearance marks a posting that states a government security-clearance
	// requirement (UK SC/DV, US Secret/TS-SCI, AU NV1), derived from the description
	// (internal/dict/location), never the LLM.
	//
	// A plain bool with omitempty: the column only holds true or NULL, and a serialized
	// false would read as "this job needs no clearance".
	RequiresClearance bool `json:"requires_clearance,omitempty"`
	// ViaRecruiter marks a posting published by an intermediary on a client's behalf
	// ("on behalf of our client"), from the description (internal/job/jobfacts).
	// True-or-absent like RequiresClearance: an absent key does not mean "posted by the
	// employer".
	ViaRecruiter bool `json:"via_recruiter,omitempty"`
	// Equity is what the posting states about equity compensation: "salary_and_equity"
	// or "equity_only", from the title and description (internal/job/jobfacts). Absent
	// means "not stated", never "no equity".
	Equity string `json:"equity,omitempty"`
	// ClearanceLevels names the clearance levels the posting states, from
	// vocab.ClearanceLevelValues (the US ladder, interim, and other). Present exactly when
	// RequiresClearance is true; an empty list would claim levels the posting never made.
	ClearanceLevels []string `json:"clearance_levels,omitempty"`
	// AutoApplyAvailable marks a posting whose ATS provider (Source) internal/api/atsapply
	// can attempt (see AutoApplyProviders): a provider-level signal, not a guarantee.
	// True-or-absent: omitted must not be read as "checked and ineligible".
	AutoApplyAvailable bool `json:"auto_apply_available,omitempty"`
	// ApplicationDeadline is the source-stated last application day (YYYY-MM-DD), inclusive.
	ApplicationDeadline *string `json:"application_deadline,omitempty"`
	// Team is the department or team the source ATS files the posting under (Greenhouse,
	// Lever and Ashby state one), verbatim. Omitted when the source states none.
	Team string `json:"team,omitempty"`
	// ApplyQuestions is how many questions the application form REQUIRES beyond the
	// standard fields (name, contact details, CV), counted from the captured form
	// (applyform.Form.RequiredQuestions); it feeds the `apply_questions_max` filter.
	//
	// nil and omitted when no form was captured (most postings). 0 is a real answer and is
	// serialized; the two must never read alike. Not set by FromRow, which sees only the
	// jobs row: the search-index writers and the detail endpoint attach it from apply_forms.
	ApplyQuestions *int    `json:"apply_questions,omitempty"`
	PostedAt       *string `json:"posted_at"`
	CreatedAt      *string `json:"created_at"`
	UpdatedAt      *string `json:"updated_at"`
	// LastSeenAt is when a re-crawl last confirmed the posting live (see
	// docs/agents/job-lifecycle.md). The SPA uses it to estimate a rolling
	// JobPosting.validThrough (seo.ts) for sources with no expiry date.
	LastSeenAt *string `json:"last_seen_at"`
	// ClosedAt is non-null when the posting is no longer open. Lists and the search index
	// never contain closed jobs; only the detail endpoint serves them.
	ClosedAt          *string           `json:"closed_at"`
	Enrichment        enrich.Enrichment `json:"enrichment"`
	EnrichedAt        *string           `json:"enriched_at"`
	EnrichmentVersion int32             `json:"enrichment_version"`
	// ViewCount/AppliedCount are the materialized counters of distinct signed-in users who
	// viewed and who marked applied, served straight from the jobs columns.
	ViewCount    int32 `json:"view_count"`
	AppliedCount int32 `json:"applied_count"`
	// UpvoteCount/DownvoteCount are the materialized public thumbs counters, served
	// straight from the jobs columns.
	UpvoteCount   int32 `json:"upvote_count"`
	DownvoteCount int32 `json:"downvote_count"`
	// MyVote is the caller's own vote (-1, 0, 1). Caller-scoped: only auth-aware detail
	// reads populate it; list, search and index-build leave it 0.
	MyVote int32 `json:"my_vote"`
	// Reality is the job-reality signal (fresh/stale/likely-evergreen + evidence), attached
	// by ClassifyReality, never stored. Nil when not computed.
	Reality *Reality `json:"reality,omitempty"`
	// Ghost is the ghost-job signal, computed at read time by ClassifyGhost, never stored
	// and never present on a closed job. Nil at level `none`, most of the catalogue.
	Ghost *Ghost `json:"ghost,omitempty"`
	// ReferralAvailable is true when the job's company has an approved employee referrer.
	// Attached only by the detail handler; list rows leave it false.
	ReferralAvailable bool `json:"referral_available,omitempty"`
	// AutoApplyStatus is the caller's own auto-apply status for this job: "queued" or
	// "declined" (the candidate's permanent decision), nil for no attempt or an anonymous
	// caller. Caller-scoped like MyVote: only the detail handler overlays it, keyed by
	// (caller_id, job_id), so the shared job query stays free of it.
	AutoApplyStatus *string `json:"auto_apply_status,omitempty"`
}

// FromRow maps a database job row to the public wire shape. It hydrates the row into
// the Job aggregate via job.FromRow and delegates to FromDomain, for callers that hold
// a db.Job.
func FromRow(j db.Job) (Job, error) {
	domain, extras, err := job.FromRow(j)
	if err != nil {
		return Job{}, err
	}
	return FromDomain(domain, extras)
}

// FromDomain projects the Job aggregate (plus its read-only Extras) to the public wire
// shape; it is the projection source of truth. Dictionary columns are served, the
// LLM's copies are folded out of the nested enrichment, and countries/regions/cities
// stay dict-then-LLM hybrids.
func FromDomain(j job.Job, x job.Extras) (Job, error) {
	f := j.Fields()
	// The projection is read-time, so the clock is a parameter, like ClassifyReality's now.
	now := time.Now()
	// e is the raw decoded LLM enrichment; the dictionary columns are folded over it
	// below (dict wins).
	e := f.Enrichment

	countries, regions := geoFacet(f.Countries, f.Regions, e.Countries, e.Regions)
	workMode := f.WorkMode
	// Seniority and the synthetic facets are always the dictionary column value, kept nested
	// under enrichment. Category is the column too, unless the column is empty: then the
	// model's answer files the posting, when it names a real category (#4412).
	e.Seniority = f.Seniority
	if f.Category != "" || e.Category == "other" || !slices.Contains(vocab.CategoryValues, e.Category) {
		e.Category = f.Category
	}
	e.PostingLanguage = f.PostingLanguage
	e.EmploymentType = f.EmploymentType
	e.EducationLevel = f.EducationLevel
	e.EnglishLevel = f.EnglishLevel
	e.ExperienceYearsMin = f.ExperienceYearsMin
	// requirements has two producers, met here on the read path: the model's list wins
	// (it reads prose postings the deterministic extractor cannot), and the derivation
	// fills in otherwise.
	//
	// The fold must happen here, not only in SetJobEnrichment's overlay: that runs only
	// when the model runs, so postings the model has not reached would serve nothing.
	if len(e.Requirements) == 0 {
		e.Requirements = f.RequirementsDerived
	}
	// Salary has four producers, folded here for the same reason: the overlay copies the
	// ATS's and the moderator's ranges only when the model runs. Manual beats source beats
	// model, and a stated layer replaces the whole range rather than lending one bound to
	// another layer's currency. The description's labeled pay line is read only when none
	// of those stated a bound.
	for _, s := range []*job.Salary{f.SourceSalary, f.ManualSalary} {
		if s != nil {
			e.SalaryMin, e.SalaryMax, e.SalaryCurrency, e.SalaryPeriod = s.Min, s.Max, s.Currency, s.Period
		}
	}
	if e.SalaryMin == nil && e.SalaryMax == nil {
		if r, found := salaryextract.Parse(f.Description, f.Countries); found {
			e.SalaryMin, e.SalaryMax, e.SalaryCurrency, e.SalaryPeriod = r.Min, r.Max, r.Currency, r.Period
		}
	}
	skills := normalizeSet(f.Skills)
	collections := normalizeSet(x.Collections)
	perks := f.Perks
	if perks == nil {
		perks = []string{}
	}
	engagement := f.Engagement
	if engagement == nil {
		engagement = []string{}
	}
	cities := cityFacet(f.Cities, e.Cities)
	// Canonical "Name (CC)" labels: normalizeSet would lowercase them. Non-nil so the
	// facet serializes as [] rather than null.
	metros := f.Metros
	if metros == nil {
		metros = []string{}
	}
	e.Countries, e.Regions, e.WorkMode = nil, nil, ""
	e.Skills = nil
	e.Cities = nil

	return Job{
		PublicSlug:          f.PublicSlug,
		Source:              f.Source,
		ManuallyAdded:       f.ManuallyAdded,
		ExternalID:          f.ExternalID,
		URL:                 outboundurl.Tag(applicationDestination(f)),
		SourceURL:           originalPosting(f),
		DirectApply:         job.IsDirectApply(f.Source, f.ExternalID),
		Title:               f.Title,
		Company:             f.Company,
		CompanySlug:         f.CompanySlug,
		Location:            f.Location,
		Description:         f.Description,
		Countries:           countries,
		Regions:             regions,
		WorkMode:            workMode,
		Skills:              skills,
		Cities:              cities,
		Metros:              metros,
		Collections:         collections,
		Perks:               perks,
		Engagement:          engagement,
		Equity:              f.Equity,
		ClearanceLevels:     f.ClearanceLevels,
		AIInterviewReports:  x.AIInterviewReports,
		IsTech:              isTechFacet(f.IsTech),
		RequiresClearance:   f.RequiresClearance != nil && *f.RequiresClearance,
		ViaRecruiter:        f.ViaRecruiter != nil && *f.ViaRecruiter,
		AutoApplyAvailable:  AutoApplyProviders[f.Source],
		PostedAt:            rfc3339Ptr(effectivePosted(f.PostedAt, f.CreatedAt, now)),
		CreatedAt:           rfc3339Ptr(f.CreatedAt),
		UpdatedAt:           rfc3339Ptr(f.UpdatedAt),
		LastSeenAt:          rfc3339Ptr(f.LastSeenAt),
		ClosedAt:            rfc3339Ptr(f.ClosedAt),
		ApplicationDeadline: dateString(f.ApplicationDeadline),
		Team:                f.Team,
		Enrichment:          e,
		EnrichedAt:          rfc3339Ptr(f.EnrichedAt),
		EnrichmentVersion:   f.EnrichmentVersion,
		ViewCount:           x.ViewCount,
		AppliedCount:        x.AppliedCount,
		UpvoteCount:         x.UpvoteCount,
		DownvoteCount:       x.DownvoteCount,
	}, nil
}

// isTechFacet renders the tri-state is_tech as its facet value: "tech", "non_tech", or
// "" (omitted) for unknown, so an unclassified job filters as empty.
func isTechFacet(isTech *bool) string {
	if isTech == nil {
		return ""
	}
	if *isTech {
		return "tech"
	}
	return "non_tech"
}

// effectivePosted is EffectivePostedAt over domain *time.Time. The clock is a
// parameter so the rule is testable.
func effectivePosted(posted, created *time.Time, now time.Time) *time.Time {
	if posted == nil || posted.After(now) {
		return created
	}
	return posted
}

// rfc3339Ptr renders a nullable domain timestamp as an RFC3339 UTC string, or nil.
func rfc3339Ptr(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339)
	return &s
}

// normalizeSet returns the sorted, deduplicated, lowercased form of a facet column.
// Always non-nil so it serializes as [] rather than null.
func normalizeSet(a []string) []string {
	set := make(map[string]struct{}, len(a))
	for _, v := range a {
		set[strings.ToLower(v)] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for v := range set {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// nonCityFallback are LLM-emitted "city" values that are really work-mode or
// open-anywhere markers. The dictionary never produces them; they leak in only through
// the enrichment.cities fallback and are dropped there. Bare-remote geography is
// carried by regions=global (see location.Parse).
var nonCityFallback = map[string]struct{}{
	"remote": {}, "remote-first": {}, "fully remote": {}, "worldwide": {},
	"anywhere": {}, "global": {}, "distributed": {}, "hybrid": {},
	"onsite": {}, "on-site": {}, "work from home": {}, "wfh": {},
}

// geoFacet builds the served country/region facets. The dictionary columns win
// whenever they pinned a place (a country, or a region more specific than "global").
// Only when geography is unpinned does it fall back wholesale to the LLM's
// enrichment.countries/regions, which catch restrictions stated only in prose
// ("Remote (SPAIN only)"). A pinned dictionary place is never overridden. Both outputs
// are lowercased, sorted, deduped and non-nil.
func geoFacet(dictCountries, dictRegions, llmCountries, llmRegions []string) (countries, regions []string) {
	countries = normalizeSet(dictCountries)
	regions = normalizeSet(dictRegions)
	if geoPinned(countries, regions) {
		return countries, regions
	}
	// The LLM writes regions as free text ("us", "emea", "gmt+1 to gmt+5"), so they
	// are folded into the region vocabulary before they can reach the facet.
	llmC := normalizeSet(llmCountries)
	llmR := location.NormalizeRegions(llmRegions)
	if llmR == nil {
		llmR = []string{}
	}
	if len(llmC) == 0 && len(llmR) == 0 {
		return countries, regions // the LLM knows no more than the dict — keep it
	}
	return llmC, llmR
}

// geoPinned reports whether the dictionary resolved a concrete place: a country, or a
// region more specific than the open-anywhere "global" bucket.
func geoPinned(countries, regions []string) bool {
	if len(countries) > 0 {
		return true
	}
	for _, r := range regions {
		if r != "global" {
			return true
		}
	}
	return false
}

// cityFacet builds the served city facet: the dictionary cities when present,
// otherwise a normalized fallback to the LLM's enrichment.cities (trimmed, ", Country"
// suffix dropped, deduped). Case is preserved: the value is its own display label.
func cityFacet(dict, llm []string) []string {
	if len(dict) > 0 {
		return dedupeSortedPreserveCase(dict)
	}
	cleaned := make([]string, 0, len(llm))
	for _, c := range llm {
		c = strings.TrimSpace(c)
		if i := strings.IndexByte(c, ','); i >= 0 {
			c = strings.TrimSpace(c[:i])
		}
		if c == "" {
			continue
		}
		if _, bad := nonCityFallback[strings.ToLower(c)]; bad {
			continue
		}
		cleaned = append(cleaned, c)
	}
	return dedupeSortedPreserveCase(cleaned)
}

// dedupeSortedPreserveCase sorts and dedupes case-insensitively, keeping the first-seen
// spelling. Non-nil so the facet serializes as [] not null; used for display-name cities.
func dedupeSortedPreserveCase(a []string) []string {
	seen := make(map[string]struct{}, len(a))
	out := make([]string, 0, len(a))
	for _, v := range a {
		k := strings.ToLower(v)
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// FromRows maps a batch of database rows to the public wire shape.
func FromRows(jobs []db.Job) ([]Job, error) {
	out := make([]Job, len(jobs))
	for i, j := range jobs {
		v, err := FromRow(j)
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

// EffectivePostedAt is the timestamp a job reads as "posted" for display and the
// freshest-first sort: the source's posted_at when present and not in the future,
// otherwise the ingest time (created_at). A missing or future posted_at (e.g. a Workday
// startDate set to a future go-live) would otherwise leave the job undated or sort it
// above genuinely recent postings. The raw created_at stays exposed separately.
//
// Exported so the search document's numeric posted_ts uses the same rule as the
// display posted_at. The clock is the caller's (now), like ClassifyReality.
func EffectivePostedAt(posted, created pgtype.Timestamptz, now time.Time) pgtype.Timestamptz {
	return pgconv.Timestamptz(effectivePosted(pgconv.TimePtr(posted), pgconv.TimePtr(created), now))
}

// dateString preserves a source's calendar day rather than serializing an instant.
func dateString(t *time.Time) *string {
	if t == nil {
		return nil
	}
	day := t.Format(time.DateOnly)
	return &day
}
