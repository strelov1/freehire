package jobderive

// Input is the raw job content the derivation reads. Source and ExternalID are the
// already-resolved storage identity (the caller decides how to namespace them); they
// feed the public slug so it is stable with the dedup key.
//
// WorkMode, Seniority, Category, Skills, and ExperienceYearsMin are the caller's
// STRUCTURED signals — values the source states explicitly (an enum, a grade, a tag
// list, a numeric field), already mapped into freehire's vocabularies. Each takes
// precedence over the dictionary derivation: the scalar signals win when present and
// the dictionary fills only when they are empty/nil; the multi-valued Skills are
// unioned with the dictionary skills. They carry structured signal only, never a
// heuristic an adapter inferred from free text; an adapter with no signal leaves them
// empty/nil so the dictionary decides.
//
// Countries, Regions and Cities are the caller's structured GEOGRAPHY signal (a country
// code the platform states directly, macro-region codes, and city names the
// submitter/moderator stated by hand). Unlike the scalars they fully replace the
// location-dictionary derivation for that facet when present — an explicit place is
// authoritative, not a hint the dictionary refines — and when empty the dictionary
// derives as before. The one exception is a Countries signal beside a location line that
// states a scope at least as broad, which the line outranks (lineOutranksCountries).
type Input struct {
	Title       string
	Company     string
	Source      string
	ExternalID  string
	Location    string
	Description string
	WorkMode    string
	Countries   []string
	Regions     []string
	Cities      []string
	// FallbackCountries is a WEAKER structured geography signal: a country the platform
	// attaches to the posting without saying the work is there (Greenhouse's offices,
	// which are sometimes the employer's headquarters rather than the hiring scope).
	// Unlike Countries it never overrides anything — it is the last step of the geography
	// chain, consulted only when the location, the title marker and both description
	// rescues all stated nothing at all, and it pulls its region along like Countries.
	FallbackCountries []string
	Seniority         string
	Category          string
	EmploymentType    string
	// Engagement is the caller's structured signal for the legal terms
	// (vocab.EngagementValues): a picklist an adapter read, or — for backfill-derive —
	// the row's stored value, which holds whatever a picklist once said and the text
	// alone cannot recover. It is unioned with the text's reading, never replaced by it.
	Engagement         []string
	Skills             []string
	ExperienceYearsMin *int
	// EducationLevel and EnglishLevel are the caller's structured signal for the two
	// facets a platform states as a picklist rather than in prose (Profession.hu
	// publishes both, on every posting). They obey the same contract as the scalars
	// above — a member of vocab.EducationLevelValues / vocab.EnglishLevelValues, set
	// only from a stated field and never from an adapter's own reading of the text,
	// so an adapter with nothing to say leaves them empty and the matchers decide.
	EducationLevel string
	EnglishLevel   string
	// SalaryMin/SalaryMax/SalaryCurrency/SalaryPeriod are the caller's structured
	// salary signal — an adapter sets them only when the ATS states a salary in its
	// own structured field (never a description-text guess: unlike the scalars
	// above, there is no dictionary fallback for salary, so this is pure passthrough).
	SalaryMin      *int
	SalaryMax      *int
	SalaryCurrency string
	SalaryPeriod   string
	// IsTechHint is the caller's structured signal that a posting is confirmed
	// technical — e.g. a source that crawls only a platform's own dedicated IT
	// category board, rather than a general-population board the title
	// dictionary must judge on its own. Unlike Category it asserts IT-ness
	// without naming a subcategory, so it feeds is_tech directly. It obeys the
	// same contract as the scalars above: a fact the source states, never a
	// heuristic an adapter infers from free text. When true it takes
	// precedence over BOTH the tech and non-tech title/category evidence
	// below — a source that crawled a posting specifically because it filed
	// it as IT is more authoritative than a generic dictionary.
	IsTechHint bool
}

// Derived is the set of facets computed from an Input.
type Derived struct {
	CompanySlug string
	PublicSlug  string
	Countries   []string
	Regions     []string
	Cities      []string
	Metros      []string
	WorkMode    string
	Skills      []string
	Seniority   string
	Category    string
	// IsTech is the tri-state technical/non-technical signal: non-nil true when the
	// derived category is a recognized technical category, non-nil false when it is a
	// known non-technical category or the title states a confident non-tech role, and
	// nil when neither resolves (unknown — never coerced, so the coverage gap stays
	// measurable).
	IsTech *bool
	// RequiresClearance marks a posting that states a government security-clearance
	// requirement (internal/dict/location). Tri-state in shape but only ever true or
	// nil: the dictionary cannot tell a posting that promises no clearance from one
	// that is silent, so a denial produces the absence of a true, not a false.
	RequiresClearance *bool
	// ViaRecruiter marks a posting that states it is published by an intermediary on a
	// client's behalf (internal/job/jobfacts). True or nil, never false, for the same
	// reason as RequiresClearance: a recruiter who never says so is not detected.
	ViaRecruiter *bool
	// Equity is what the posting states about equity compensation (jobfacts.Equity): a
	// vocab.EquityValues member, or "" when it states nothing the matcher reads. There
	// is no "no equity" value, for the same reason as RequiresClearance has no false.
	Equity string
	// ClearanceLevels names the clearance levels the posting states, in
	// vocab.ClearanceLevelValues order. Non-empty exactly when RequiresClearance is
	// set: both come from one reading of the description, so they cannot disagree.
	ClearanceLevels []string
	// Perks are the benefits the description states it offers, in vocab.PerkValues
	// order (internal/dict/perks); nil when it names none.
	Perks []string
	// Engagement names the legal terms the title or description states the worker is
	// engaged on, in vocab.EngagementValues order (internal/dict/engagement); nil when
	// they state none.
	Engagement []string
	// Synthetic enrichment facets (category B): deterministic stand-ins for fields
	// the LLM also emits, served dict-only like the facets above. ExperienceYearsMin
	// is nil when the description states no figure.
	PostingLanguage    string
	EmploymentType     string
	EducationLevel     string
	EnglishLevel       string
	ExperienceYearsMin *int
	// SalaryMin/SalaryMax/SalaryCurrency/SalaryPeriod pass Input's structured salary
	// signal through unchanged — see Input's doc for why there is no dictionary
	// fallback here.
	SalaryMin      *int
	SalaryMax      *int
	SalaryCurrency string
	SalaryPeriod   string
}
