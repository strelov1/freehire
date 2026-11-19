package kit

import (
	"time"

	"github.com/strelov1/freehire/internal/ingest/applyform"
)

// Job is a raw posting as an adapter yields it, before the pipeline normalizes it
// into the catalogue. ExternalID carries the platform's native posting id; the
// pipeline namespaces it by board before persisting.
type Job struct {
	ExternalID string
	URL        string
	Title      string
	Company    string
	// CompanyKey, when set, is what the company slug derives from instead of Company: for a
	// posting shown under a neutral label shared by many boards (PlaceholderLabel), so each
	// board stays its own company. Empty for every other posting.
	CompanyKey  string
	Location    string
	Description string
	// ApplyMethod and ExternalApplyURL are discovery evidence from the provider's
	// primary application control. They are not a catalogue destination override.
	// An unknown method must not be treated as a provider-native application.
	ApplyMethod      ApplicationMethod
	ExternalApplyURL string
	Remote           bool
	PostedAt         *time.Time
	// ApplicationDeadline is the source's inclusive last application date, in UTC.
	// Nil means the source did not provide a deadline.
	ApplicationDeadline *time.Time
	// Team is the department or team the platform files the posting under, in a STRUCTURED
	// field, verbatim (the adapter trims it; Greenhouse, Lever and Ashby set it). It is the
	// ATS's own label, not a taxonomy; empty when the platform states none.
	Team string
	// WorkMode is the work arrangement when the platform states it in a STRUCTURED
	// field (a workplace-type enum or an explicit remote flag) — "remote",
	// "hybrid", or "onsite", else "". It is left empty for adapters that only
	// expose free-text location; the pipeline then falls back to parsing the
	// location string. Provenance stays clean: this carries structured signal only,
	// never the location heuristic.
	WorkMode string
	// Seniority, Category, EmploymentType, Skills, and ExperienceYearsMin are the
	// platform's STRUCTURED facet signals, already mapped into freehire's controlled
	// vocabularies (vocab.SeniorityValues / vocab.CategoryValues /
	// vocab.EmploymentTypeValues / canonical skill names). They mirror WorkMode: an
	// adapter sets them only when the platform states the value in a structured field
	// (e.g. an ATS timeType / typeOfEmployment enum), never a heuristic inferred from
	// free text, and leaves them empty/nil otherwise so the pipeline's dictionaries
	// decide. The pipeline gives a set value precedence over the dictionary (Skills are unioned).
	Seniority          string
	Category           string
	EmploymentType     string
	Skills             []string
	ExperienceYearsMin *int
	// Engagement is the legal terms the platform states in a structured field
	// (vocab.EngagementValues) — a Brazilian board's "Tipo de contrato" picklist naming
	// CLT or PJ. The pipeline unions it with what internal/dict/engagement reads from the
	// text, since a picklist and a description can each name a term the other omits.
	Engagement []string
	// IsTechHint is a structured "confirmed technical" signal, set only by an adapter
	// whose crawl scope itself guarantees a posting is technical — e.g. a source that
	// crawls exclusively a platform's own dedicated IT category board, rather than a
	// general-population board the title dictionary must judge unassisted. Unlike
	// Category it asserts IT-ness without naming a subcategory: a source sets it when
	// it KNOWS a posting is IT but not which kind, so the dictionary is left to try
	// (and may still fail, leaving the category empty). It is a fact the adapter
	// states about its own crawl, never a heuristic inferred from title or
	// description text.
	IsTechHint bool
	// EducationLevel and EnglishLevel follow the same structured-only contract, for the
	// platforms that state them as a picklist instead of leaving them in prose
	// (profession.hu publishes both on every posting, as "Angol középfok, Egyetem").
	// Members of vocab.EducationLevelValues / vocab.EnglishLevelValues. They matter more
	// than the other scalars on a non-English board: the fallback matchers
	// (internal/job/jobfacts) read English prose only, so without a structured value a
	// Hungarian or Polish posting yields nothing at all rather than something weaker.
	EducationLevel string
	EnglishLevel   string
	// Countries mirrors the same contract for geography: an adapter sets it only when
	// the platform states the country in a STRUCTURED field (not the free-text location
	// the description/location string carries), normalized through
	// internal/dict/location.NormalizeCountry into freehire's canonical lowercase alpha-2
	// codes. Left nil for adapters that expose only free-text location, so the
	// pipeline's location dictionary derives it instead — the same fallback WorkMode
	// and the other structured facets already follow.
	Countries []string
	// FallbackCountries is the weaker sibling of Countries: a country the platform
	// attaches to the posting in a structured field WITHOUT stating that the work is
	// there — Greenhouse's offices, which on a bare-"Remote" posting are usually the
	// hiring country but sometimes only the headquarters. It never overrides anything:
	// jobderive consults it last, only when the location, the title and the description
	// stated no geography at all. Canonical alpha-2 codes, nil when there is no signal.
	FallbackCountries []string
	// SalaryMin/SalaryMax/SalaryCurrency/SalaryPeriod mirror the same structured-only
	// contract for compensation: an adapter sets them only when the platform states a
	// salary in its own STRUCTURED field (e.g. Lever's salaryRange, Ashby's
	// compensationTiers, Recruitee's salary), currency as ISO 4217 and period as one of
	// vocab.SalaryPeriodValues ("year"/"month"/"day"/"hour"), never inferred from the
	// description. Left nil/empty otherwise, so the enrichment pass's own LLM-derived
	// guess decides — the pipeline gives a set value precedence over that guess.
	SalaryMin      *int
	SalaryMax      *int
	SalaryCurrency string
	SalaryPeriod   string
	// CompanyWebsite is the EMPLOYER's own website, set only when the platform states it in a
	// structured employer field (EURES's employer.website). It is never a posting URL and never the
	// job board's own host. It is a fact about the company rather than the posting, so the pipeline
	// writes it to the company row (company_info.website, only where none is known yet) and keeps it
	// out of the job aggregate and its content hash. A shared website is the strongest evidence that
	// two spellings are one employer (docs/agents/company-identity.md).
	CompanyWebsite string
	// CompanyProfileURL is the provider's own employer profile, used only to
	// discover a structured employer website when an external apply URL is hidden.
	CompanyProfileURL string
	// Removed marks a posting the source reports as taken down (e.g. an item flagged
	// removed in JobStream's incremental feed). A streaming, self-closing source emits
	// these so the pipeline closes the job by identity instead of upserting it; all other
	// adapters leave it false and only ever emit live postings.
	Removed bool
	// SeenRefresh marks a posting a HydratingSource re-listed but did NOT fetch fresh
	// content for (it was already ingested, so detail is skipped). The pipeline refreshes
	// the row's liveness (last_seen_at, reopen) by identity WITHOUT rewriting its content,
	// so the description and facets hydrated when it was new are preserved — a content-less
	// re-upsert would re-derive the facets from an empty description and wipe them. Only a
	// HydratingSource sets it (carrying just Title/Company/URL/ExternalID for the identity);
	// all other adapters leave it false. Mutually exclusive with Removed.
	SeenRefresh bool
	// Unreadable marks a posting the board's listing NAMED but whose detail request could not
	// be READ — a timeout, a refusal, a 5xx, an undecodable body, or a page that answered
	// carrying no posting at all. It is the opposite of the platform stating the posting is
	// GONE (404/410), which is information and is simply not yielded: this is the absence of
	// information, and the difference decides whether a run may read "we did not see it" as
	// evidence that it is no longer there.
	//
	// The pipeline writes nothing for it — there is nothing to write — and counts it in
	// Stats.Unreadable, which withholds the run's close scopes for that board rather than
	// failing it. Failing the board would turn a per-posting hole into a whole-board outage,
	// cool the board to its 24-hour ceiling and freeze its entire catalogue.
	//
	// An adapter sets it INSTEAD of dropping the posting, carrying whatever identity it holds
	// without the detail (ExternalID/URL/Company), so the employer whose close scope is
	// withheld can be named. Only a buffered adapter whose detail is a posting's ONLY source
	// sets it: a HydratingSource re-lists a stored posting as SeenRefresh with no detail
	// request at all, so the case does not arise there, and ingestStream does not read this
	// marker. Mutually exclusive with Removed and SeenRefresh.
	//
	// EVERY OTHER READER OF A Fetch RESULT MUST SKIP IT. It carries identity without detail,
	// so it satisfies an id or URL predicate while holding no title — and unlike SeenRefresh,
	// which a link resolver can never match because its seen-predicate hydrates exactly the
	// posting being asked about, this marker fires on precisely that posting. linksource's
	// pickPosting skips it for that reason; a future reader that does not will turn "we could
	// not read that page" into an invalid draft and a 500.

	Unreadable bool
	// ApplyForm is the application form the platform published for this posting, set ONLY
	// by an adapter whose list endpoint already carries one. It is nil for every other
	// adapter, and nil is not a failure — most platforms do not describe their form in a
	// listing, and the two that describe it per posting are captured after ingest by
	// cmd/capture-apply-form rather than here.
	//
	// An adapter must not issue an extra request to fill this. That is the whole point of
	// the split: a form that costs a request would make a crawl's duration a function of
	// board size, and the adapter cannot tell which postings are new anyway — that answer
	// only exists after the upsert's ON CONFLICT resolves.
	ApplyForm *applyform.Form
}
