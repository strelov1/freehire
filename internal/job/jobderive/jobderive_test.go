package jobderive

import (
	"reflect"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/dict/vocab"
)

func TestDerive_SlugsAndFacets(t *testing.T) {
	got := Derive(Input{
		Title:       "Senior Go Developer",
		Company:     "Acme",
		Source:      "manual",
		ExternalID:  "https://acme.example/jobs/1",
		Location:    "Remote - Germany",
		Description: "We use Golang and PostgreSQL.",
	})

	wantSlug := normalize.JobSlug("Senior Go Developer", "Acme", "manual", "https://acme.example/jobs/1")
	if got.PublicSlug != wantSlug {
		t.Errorf("PublicSlug = %q, want %q", got.PublicSlug, wantSlug)
	}
	if got.CompanySlug != normalize.CompanySlug("Acme") {
		t.Errorf("CompanySlug = %q", got.CompanySlug)
	}
	if len(got.Countries) == 0 || got.Countries[0] != "de" {
		t.Errorf("Countries = %v, want [de ...]", got.Countries)
	}
	if !reflect.DeepEqual(got.Skills, []string{"go", "postgresql"}) {
		t.Errorf("Skills = %v, want [go postgresql]", got.Skills)
	}
	// No structured work mode supplied → the parser's hint (remote) is used.
	if got.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote (parsed)", got.WorkMode)
	}
}

// A structured work-mode signal from the caller (e.g. an ATS workplace-type enum)
// beats the free-text parser hint.
//
// The synthetic enrichment facets (posting language, employment type, education
// level, minimum experience) are derived from the title/description.
func TestDerive_SyntheticFacets(t *testing.T) {
	got := Derive(Input{
		Title:      "Backend Engineering Intern",
		Company:    "Acme",
		Source:     "manual",
		ExternalID: "1",
		Description: "We are looking for a motivated engineer to join our backend team " +
			"and build scalable services. A Bachelor's degree in Computer Science is " +
			"required, along with at least 3 years of hands-on programming experience. " +
			"Fluent English is required.",
	})
	if got.PostingLanguage != "en" {
		t.Errorf("PostingLanguage = %q, want en", got.PostingLanguage)
	}
	if got.EmploymentType != "internship" {
		t.Errorf("EmploymentType = %q, want internship", got.EmploymentType)
	}
	if got.EducationLevel != "bachelor" {
		t.Errorf("EducationLevel = %q, want bachelor", got.EducationLevel)
	}
	if got.EnglishLevel != "c1" {
		t.Errorf("EnglishLevel = %q, want c1", got.EnglishLevel)
	}
	if got.ExperienceYearsMin == nil || *got.ExperienceYearsMin != 3 {
		t.Errorf("ExperienceYearsMin = %v, want 3", got.ExperienceYearsMin)
	}
}

// A description stating none of the synthetic facets leaves them empty/nil — the
// derivation never guesses.
func TestDerive_SyntheticFacetsSilent(t *testing.T) {
	got := Derive(Input{
		Title:       "Engineer",
		Company:     "Acme",
		Source:      "manual",
		ExternalID:  "2",
		Description: "Join us.",
	})
	if got.EmploymentType != "" || got.EducationLevel != "" || got.EnglishLevel != "" || got.ExperienceYearsMin != nil {
		t.Errorf("expected silent facets, got type=%q edu=%q eng=%q exp=%v",
			got.EmploymentType, got.EducationLevel, got.EnglishLevel, got.ExperienceYearsMin)
	}
}

// A structured employment_type from the adapter takes precedence over the free-text
// jobfacts parse, mirroring the seniority/category precedence — the source's own
// value wins even when the description text would parse to something else.
func TestDerive_StructuredEmploymentTypeWins(t *testing.T) {
	got := Derive(Input{
		Title:          "Backend Engineering Intern", // jobfacts would parse "internship"
		Company:        "Acme",
		Source:         "manual",
		ExternalID:     "3",
		Description:    "An internship opportunity.",
		EmploymentType: "contract", // structured signal from the source
	})
	if got.EmploymentType != "contract" {
		t.Errorf("EmploymentType = %q, want contract (structured wins over jobfacts internship)", got.EmploymentType)
	}
}

func TestRetainEmploymentType(t *testing.T) {
	const disclaimer = "Salaries for part-time roles will be pro-rated based on number of hours regularly worked."
	cases := []struct {
		name, stored, title, desc, want string
	}{
		{"ats full time survives the pay policy", "full_time", "Manager", disclaimer, "full_time"},
		{"ats part time with no phrase is kept", "part_time", "Analyst", "Build the risk platform.", "part_time"},
		{"real part-time sentence is kept", "part_time", "Barista", "This is a part-time role, 20h/week. " + disclaimer, "part_time"},
		{"disclaimer-only part time is dropped", "part_time", "Manager", disclaimer, ""},
		{"empty stays empty", "", "Manager", disclaimer, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := RetainEmploymentType(c.stored, c.title, c.desc); got != c.want {
				t.Errorf("RetainEmploymentType(%q) = %q, want %q", c.stored, got, c.want)
			}
		})
	}
}

// Education and English level were the last two facets with no structured tier: both
// were always read out of the description by English-language matchers, so a platform
// that STATES them — Profession.hu publishes each as a closed picklist — had nowhere to
// put the answer. They now follow the same precedence as the four facets above.
func TestDerive_StructuredEducationAndEnglishLevelWin(t *testing.T) {
	got := Derive(Input{
		Title:      "Fejlesztő",
		Company:    "Acme",
		Source:     "profession",
		ExternalID: "4",
		// The description would parse to bachelor and c1 on its own.
		Description:    "Bachelor's degree required. Fluent English.",
		EducationLevel: "master",
		EnglishLevel:   "b2",
	})
	if got.EducationLevel != "master" {
		t.Errorf("EducationLevel = %q, want master (structured wins over the bachelor in the text)", got.EducationLevel)
	}
	if got.EnglishLevel != "b2" {
		t.Errorf("EnglishLevel = %q, want b2 (structured wins over the fluent in the text)", got.EnglishLevel)
	}
}

// The description stays the fallback: a source that states neither leaves both to the
// matchers, which is every adapter but one.
func TestDerive_EducationAndEnglishLevelFallBackToDescription(t *testing.T) {
	got := Derive(Input{
		Title:       "Developer",
		Company:     "Acme",
		Source:      "greenhouse",
		ExternalID:  "5",
		Description: "Bachelor's degree required. English at B2 level.",
	})
	if got.EducationLevel != "bachelor" {
		t.Errorf("EducationLevel = %q, want bachelor from the description", got.EducationLevel)
	}
	if got.EnglishLevel != "b2" {
		t.Errorf("EnglishLevel = %q, want b2 from the description", got.EnglishLevel)
	}
}

func TestDerive_StructuredWorkModeWins(t *testing.T) {
	got := Derive(Input{
		Title:      "Dev",
		Company:    "Acme",
		Source:     "greenhouse",
		ExternalID: "board:1",
		Location:   "Remote - Germany",
		WorkMode:   "hybrid",
	})
	if got.WorkMode != "hybrid" {
		t.Errorf("WorkMode = %q, want hybrid (structured wins over parsed remote)", got.WorkMode)
	}
}

// Work-mode source precedence: structured → location → description. The
// description is the lowest-priority source and only fills a value the structured
// signal and the location marker both left empty.
func TestDerive_DescriptionFillsWorkModeWhenLocationSilent(t *testing.T) {
	got := Derive(Input{
		Title:       "Dev",
		Company:     "Acme",
		Source:      "greenhouse",
		ExternalID:  "board:1",
		Location:    "Berlin", // no work-mode marker
		Description: "This is a fully remote position open to the EU.",
	})
	if got.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote (description fills when location silent)", got.WorkMode)
	}
	// The beacon city is forwarded from location.Parse to the cities facet.
	if len(got.Cities) != 1 || got.Cities[0] != "Berlin" {
		t.Errorf("Cities = %v, want [Berlin]", got.Cities)
	}
}

func TestDerive_DescriptionContradictsLocationRemoteWithHybrid(t *testing.T) {
	got := Derive(Input{
		Title:       "Dev",
		Company:     "Acme",
		Source:      "greenhouse",
		ExternalID:  "board:1",
		Location:    "Remote - Germany",       // location marker → remote
		Description: "This is a hybrid role.", // explicit arrangement contradicts the location bucket
	})
	if got.WorkMode != "hybrid" {
		t.Errorf("WorkMode = %q, want hybrid (explicit arrangement contradicts remote)", got.WorkMode)
	}
}

func TestDerive_StructuredBeatsDescription(t *testing.T) {
	got := Derive(Input{
		Title:       "Dev",
		Company:     "Acme",
		Source:      "greenhouse",
		ExternalID:  "board:1",
		Location:    "Berlin", // silent
		WorkMode:    "onsite", // structured
		Description: "This is a fully remote position.",
	})
	if got.WorkMode != "onsite" {
		t.Errorf("WorkMode = %q, want onsite (structured beats description)", got.WorkMode)
	}
}

// The contradiction tier sits ABOVE the precedence chain: a description that explicitly
// denies remote work overrules a remote work mode whichever source produced it. See
// location.RemoteContradicted.
func TestDerive_DescriptionContradictsLocationRemote(t *testing.T) {
	got := Derive(Input{
		Title:      "Senior Structural Test Engineer",
		Company:    "NVIDIA",
		Source:     "workday",
		ExternalID: "board:1",
		// The employer's own Workday location bucket, with remoteType unset (freehire#2555).
		Location: "US, TX, Remote",
		Description: "This position is 100% on-site based at either our Dallas or Houston " +
			"Contract Manufacturing (CM) facility.",
	})
	if got.WorkMode != "onsite" {
		t.Errorf("WorkMode = %q, want onsite (description contradicts the location's remote)", got.WorkMode)
	}
}

func TestDerive_DescriptionContradictsStructuredRemote(t *testing.T) {
	got := Derive(Input{
		Title:       "Executive Assistant",
		Company:     "Acme",
		Source:      "paycor",
		ExternalID:  "board:1",
		Location:    "Mentor, OH",
		WorkMode:    "remote", // the ATS's own field, and wrong
		Description: "Onsite - not a remote position. Join our team supporting leadership.",
	})
	if got.WorkMode != "onsite" {
		t.Errorf("WorkMode = %q, want onsite (description contradicts the ATS remote flag)", got.WorkMode)
	}
}

// Only a remote result is ever overruled. "Not a remote position" is a true statement about a
// hybrid job, so reading it as onsite would replace a right answer with a wrong one.
func TestDerive_DenialLeavesHybridAlone(t *testing.T) {
	got := Derive(Input{
		Title:       "Dev",
		Company:     "Acme",
		Source:      "greenhouse",
		ExternalID:  "board:1",
		Location:    "Berlin",
		WorkMode:    "hybrid",
		Description: "This is not a remote position; you will be in the office three days a week.",
	})
	if got.WorkMode != "hybrid" {
		t.Errorf("WorkMode = %q, want hybrid (a denial of remote says nothing against hybrid)", got.WorkMode)
	}
}

func TestDerive_GenuineRemoteSurvives(t *testing.T) {
	got := Derive(Input{
		Title:       "Dev",
		Company:     "Acme",
		Source:      "greenhouse",
		ExternalID:  "board:1",
		Location:    "Remote - Germany",
		Description: "Collaborate with a remote team; we are fully remote across the EU.",
	})
	if got.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote (no denial in the description)", got.WorkMode)
	}
}

func TestDerive_NoisyDescriptionYieldsNoWorkMode(t *testing.T) {
	got := Derive(Input{
		Title:       "Dev",
		Company:     "Acme",
		Source:      "greenhouse",
		ExternalID:  "board:1",
		Location:    "Berlin", // silent
		Description: "Experience with distributed systems and hybrid cloud.",
	})
	if got.WorkMode != "" {
		t.Errorf("WorkMode = %q, want empty (noisy description, no real phrase)", got.WorkMode)
	}
}

// Seniority source precedence: title dictionary → description. The description
// only fills a grade the title left empty; category is unaffected.
func TestDerive_DescriptionFillsSeniorityWhenTitleSilent(t *testing.T) {
	got := Derive(Input{
		Title:       "Backend Developer", // no grade word
		Company:     "Acme",
		Source:      "greenhouse",
		ExternalID:  "board:1",
		Description: "We are looking for a senior engineer to own the platform.",
	})
	if got.Seniority != "senior" {
		t.Errorf("Seniority = %q, want senior (description fills when title silent)", got.Seniority)
	}
	if got.Category != "backend" {
		t.Errorf("Category = %q, want backend (unaffected)", got.Category)
	}
}

func TestDerive_TitleSeniorityBeatsDescription(t *testing.T) {
	got := Derive(Input{
		Title:       "Lead Backend Engineer", // title → lead
		Company:     "Acme",
		Source:      "greenhouse",
		ExternalID:  "board:1",
		Description: "You will work with a senior team.", // description → senior, but loses
	})
	if got.Seniority != "lead" {
		t.Errorf("Seniority = %q, want lead (title beats description)", got.Seniority)
	}
}

func TestDerive_NoisyDescriptionYieldsNoSeniority(t *testing.T) {
	got := Derive(Input{
		Title:       "Backend Developer", // no grade
		Company:     "Acme",
		Source:      "greenhouse",
		ExternalID:  "board:1",
		Description: "Collaborate with senior management and lead the team to success.",
	})
	if got.Seniority != "" {
		t.Errorf("Seniority = %q, want empty (noisy description, no anchored grade)", got.Seniority)
	}
}

// A structured seniority from the source (e.g. a marketplace's grade field) wins
// over both the title dictionary and the description.
func TestDerive_StructuredSeniorityWins(t *testing.T) {
	got := Derive(Input{
		Title:       "Lead Backend Engineer", // title → lead
		Company:     "Acme",
		Source:      "getmatch",
		ExternalID:  "1",
		Seniority:   "senior", // structured source signal
		Description: "We want a junior to grow.",
	})
	if got.Seniority != "senior" {
		t.Errorf("Seniority = %q, want senior (structured source wins)", got.Seniority)
	}
}

// When the source carries no structured seniority, the dictionary fills it.
func TestDerive_DictionaryFillsSeniorityWhenSourceSilent(t *testing.T) {
	got := Derive(Input{
		Title:      "Lead Backend Engineer", // title → lead
		Company:    "Acme",
		Source:     "getmatch",
		ExternalID: "1",
	})
	if got.Seniority != "lead" {
		t.Errorf("Seniority = %q, want lead (dictionary fills when source silent)", got.Seniority)
	}
}

// A structured category from the source wins over the title dictionary; when the
// source is silent the dictionary fills it.
func TestDerive_StructuredCategoryWinsAndDictionaryFills(t *testing.T) {
	withSource := Derive(Input{
		Title:      "Backend Developer", // title → backend
		Company:    "Acme",
		Source:     "getmatch",
		ExternalID: "1",
		Category:   "data_engineering", // structured source signal
	})
	if withSource.Category != "data_engineering" {
		t.Errorf("Category = %q, want data_engineering (structured source wins)", withSource.Category)
	}

	silent := Derive(Input{
		Title:      "Backend Developer", // title → backend
		Company:    "Acme",
		Source:     "getmatch",
		ExternalID: "2",
	})
	if silent.Category != "backend" {
		t.Errorf("Category = %q, want backend (dictionary fills when source silent)", silent.Category)
	}
}

// The description fills a non-technical category when the source and title are both
// silent — the lowest tier of the category precedence, feeding the enrichment gate.
func TestDerive_DescriptionFillsNonTechCategoryWhenTitleSilent(t *testing.T) {
	got := Derive(Input{
		Title:       "Join Our Team", // no category word
		Company:     "Acme",
		Source:      "greenhouse",
		ExternalID:  "board:1",
		Description: "We are hiring a sales representative for our growing team.",
	})
	if got.Category != "sales" {
		t.Errorf("Category = %q, want sales (description fills when title silent)", got.Category)
	}
}

// The title dictionary beats the description tier: a resolvable title is never
// overwritten by a non-tech phrase in the prose.
func TestDerive_TitleCategoryBeatsDescription(t *testing.T) {
	got := Derive(Input{
		Title:       "Backend Developer", // title → backend
		Company:     "Acme",
		Source:      "greenhouse",
		ExternalID:  "board:1",
		Description: "You will support our sales representative onboarding.",
	})
	if got.Category != "backend" {
		t.Errorf("Category = %q, want backend (title beats description)", got.Category)
	}
}

// The description tier resolves only non-technical categories: a title-silent job
// whose prose describes a technical role stays empty (never enriched-then-gated).
func TestDerive_TechDescriptionYieldsNoCategory(t *testing.T) {
	got := Derive(Input{
		Title:       "Join Our Team", // no category word
		Company:     "Acme",
		Source:      "greenhouse",
		ExternalID:  "board:1",
		Description: "Build resilient backend systems in Go and mentor the team.",
	})
	if got.Category != "" {
		t.Errorf("Category = %q, want empty (description resolves no technical category)", got.Category)
	}
}

// A structured category from the source wins over the description tier too.
func TestDerive_StructuredCategoryBeatsDescription(t *testing.T) {
	got := Derive(Input{
		Title:       "Join Our Team", // no category word
		Company:     "Acme",
		Source:      "getmatch",
		ExternalID:  "1",
		Category:    "frontend", // structured source signal
		Description: "We are hiring a sales representative.",
	})
	if got.Category != "frontend" {
		t.Errorf("Category = %q, want frontend (structured source wins over description)", got.Category)
	}
}

// A structured minimum-experience from the source wins over the jobfacts text
// parse; when the source is nil the text parse fills it.
func TestDerive_StructuredExperienceWinsAndTextFills(t *testing.T) {
	src := 7
	withSource := Derive(Input{
		Title:              "Dev",
		Company:            "Acme",
		Source:             "getmatch",
		ExternalID:         "1",
		ExperienceYearsMin: &src,
		Description:        "at least 3 years of experience required",
	})
	if withSource.ExperienceYearsMin == nil || *withSource.ExperienceYearsMin != 7 {
		t.Errorf("ExperienceYearsMin = %v, want 7 (structured source wins)", withSource.ExperienceYearsMin)
	}

	silent := Derive(Input{
		Title:       "Dev",
		Company:     "Acme",
		Source:      "getmatch",
		ExternalID:  "2",
		Description: "at least 3 years of experience required",
	})
	if silent.ExperienceYearsMin == nil || *silent.ExperienceYearsMin != 3 {
		t.Errorf("ExperienceYearsMin = %v, want 3 (text fills when source nil)", silent.ExperienceYearsMin)
	}
}

// Skills is a set: the structured source skills are UNIONED with the dictionary
// skills (deduped, sorted), neither replacing the other.
func TestDerive_SourceSkillsUnionWithDictionary(t *testing.T) {
	got := Derive(Input{
		Title:       "Dev",
		Company:     "Acme",
		Source:      "getmatch",
		ExternalID:  "1",
		Skills:      []string{"go"},              // structured source signal
		Description: "We use Kubernetes and Go.", // dictionary → go, kubernetes
	})
	if !reflect.DeepEqual(got.Skills, []string{"go", "kubernetes"}) {
		t.Errorf("Skills = %v, want [go kubernetes] (source ∪ dictionary, deduped+sorted)", got.Skills)
	}
}

// The job's resolved category is passed into skilltag.Parse's category-scoped
// acronym tier (refine-ai-role-classification), so a bare "RAG" mention resolves
// to the "rag" skill on an AI-category job — closing the under-tagging gap the
// résumé-only acronym left on job postings. Input.Category is deliberately left
// empty here: the resolved category must come from the title dictionary
// (class.Category), the common real-world path since most sources carry no
// structured category signal — passing the raw in.Category instead of the
// resolved local would compile fine and pass a test that set Category directly,
// but silently miss this path.
func TestDerive_CategoryScopedAcronymFillsSkills(t *testing.T) {
	got := Derive(Input{
		Title:       "AI Engineer", // resolves to category ai_engineering via the title dictionary
		Company:     "Acme",
		Source:      "getmatch",
		ExternalID:  "1",
		Description: "Built RAG pipelines over pgvector.",
	})
	if got.Category != "ai_engineering" {
		t.Fatalf("Category = %q, want ai_engineering (test setup precondition)", got.Category)
	}
	if !slices.Contains(got.Skills, "rag") {
		t.Errorf("Skills = %v, want to contain \"rag\" (category-scoped RAG acronym)", got.Skills)
	}
}

// The category-scoped acronym tier only fires for categories on its own
// allow-list — a category outside ai_engineering/ml_ai must not tag bare "RAG",
// same as today.
func TestDerive_CategoryScopedAcronymDoesNotFireOutsideAllowList(t *testing.T) {
	got := Derive(Input{
		Title:       "Backend Engineer",
		Company:     "Acme",
		Source:      "getmatch",
		ExternalID:  "1",
		Category:    "backend",
		Description: "We report RAG status weekly.",
	})
	if slices.Contains(got.Skills, "rag") {
		t.Errorf("Skills = %v, must not contain \"rag\" for category backend", got.Skills)
	}
}

// TestDerive_CompanySlugStripsLegalForms pins that the catalogue's company key is
// normalize.CompanySlug, not normalize.Slug. This one line is the whole of duplicate class
// 1a: 4,149 folded groups covering 229,004 open jobs on prod are one employer written with
// and without its corporate form.
func TestDerive_CompanySlugStripsLegalForms(t *testing.T) {
	for company, want := range map[string]string{
		"RingCentral, Inc.":     "ringcentral",
		"Accenture GmbH":        "accenture",
		"Sun Technologies,Inc.": "sun-technologies",
		"Acme":                  "acme",
	} {
		got := Derive(Input{Title: "Engineer", Company: company, Source: "manual", ExternalID: "1"})
		if got.CompanySlug != want {
			t.Errorf("Derive(company=%q).CompanySlug = %q, want %q", company, got.CompanySlug, want)
		}
	}
}

// TestDerive_IsPure guards the reason class 1b resolves in pipeline.Runner and not here.
//
// Derive is the one derivation every write path shares — ingest, moderator authoring and
// Telegram all call it, which is what stops the deterministic facets diverging between them.
// It can only hold that position while it is a function of its input alone. Handing it a
// context would mean handing it a database, and the next caller would have to be able to fail.
func TestDerive_IsPure(t *testing.T) {
	fn := reflect.TypeOf(Derive)
	if got := fn.NumIn(); got != 1 {
		t.Fatalf("Derive takes %d arguments, want exactly 1 (Input) — a second argument is "+
			"almost certainly a context, and Derive must not be able to do I/O", got)
	}
	if got := fn.In(0); got != reflect.TypeOf(Input{}) {
		t.Errorf("Derive's argument is %v, want jobderive.Input", got)
	}
	if got := fn.NumOut(); got != 1 {
		t.Fatalf("Derive returns %d values, want exactly 1 (Derived) — it cannot fail, "+
			"because it does nothing that can", got)
	}
	if got := fn.Out(0); got != reflect.TypeOf(Derived{}) {
		t.Errorf("Derive returns %v, want jobderive.Derived", got)
	}
}

// Three producers write experience_years_min and each can overshoot on its own, so the
// bound sits where they converge rather than in any one of them. Both cases below are
// real prod rows from 2026-09-05.
func TestDerive_DropsImplausibleExperienceYears(t *testing.T) {
	years := func(n int) *int { return &n }

	// The description talks about the EMPLOYER's age. "N years of experience" is
	// syntactically identical whether the subject is the candidate, the company or the
	// product, so the text parse cannot tell them apart — only the ceiling can.
	t.Run("a company's age in the description is not read as a requirement", func(t *testing.T) {
		d := Derive(Input{
			Title:  "System Engineer",
			Source: "manual", ExternalID: "1",
			Description: "With over 40 years of semiconductor process control experience, " +
				"chipmakers around the world rely on KLA to solve really hard problems.",
		})
		if d.ExperienceYearsMin != nil {
			t.Errorf("ExperienceYearsMin = %d, want nil — 40 is the company's age, not a requirement",
				*d.ExperienceYearsMin)
		}
	})

	// A source adapter's structured field normally WINS over the text parse, so an
	// out-of-range value there reaches the column without the description saying
	// anything at all. Seven smartrecruiters postings stored 84 this way.
	t.Run("a structured signal past the ceiling is dropped too", func(t *testing.T) {
		d := Derive(Input{
			Title:  "Java Developer",
			Source: "smartrecruiters", ExternalID: "2",
			ExperienceYearsMin: years(84),
			Description:        "Build services.",
		})
		if d.ExperienceYearsMin != nil {
			t.Errorf("ExperienceYearsMin = %d, want nil — no career is 84 years long",
				*d.ExperienceYearsMin)
		}
	})

	// The whole tail above the ceiling is round — 30 (19,145 rows), 40, 50, 35, 45 —
	// because careers are not round and anniversaries are. Every sampled row at 30 is
	// the employer's age, on postings like these two.
	t.Run("an employer's anniversary is not a server's required experience", func(t *testing.T) {
		for _, tc := range []struct{ title, description string }{
			{"Dining Room Server", "As a family company, we can do things differently, and for " +
				"the past 30 years, our long-term commitment to team members has been unsurpassed."},
			{"Account Executive", "We have maintained an A+ rating from A.M. Best, the industry's " +
				"leading authority, for over 30 years."},
		} {
			d := Derive(Input{Title: tc.title, Source: "manual", ExternalID: "5", Description: tc.description})
			if d.ExperienceYearsMin != nil {
				t.Errorf("%s: ExperienceYearsMin = %d, want nil — that is the company's age",
					tc.title, *d.ExperienceYearsMin)
			}
		}
	})

	t.Run("a believable structured signal is kept", func(t *testing.T) {
		for _, n := range []int{0, 5, 18, 25, vocab.MaxExperienceYears} {
			d := Derive(Input{
				Title:  "Programme Director",
				Source: "manual", ExternalID: "3",
				ExperienceYearsMin: years(n),
				Description:        "Lead the programme.",
			})
			if d.ExperienceYearsMin == nil || *d.ExperienceYearsMin != n {
				t.Errorf("ExperienceYearsMin = %v for input %d, want it kept — postings really "+
					"do state 18 and 25", d.ExperienceYearsMin, n)
			}
		}
	})

	t.Run("a believable figure in the description is kept", func(t *testing.T) {
		d := Derive(Input{
			Title:  "Backend Engineer",
			Source: "manual", ExternalID: "4",
			Description: "We need 5+ years of Go experience.",
		})
		if d.ExperienceYearsMin == nil || *d.ExperienceYearsMin != 5 {
			t.Errorf("ExperienceYearsMin = %v, want 5", d.ExperienceYearsMin)
		}
	})
}

func TestDerive_ExplicitRemoteWorkFromHomeDenial(t *testing.T) {
	got := Derive(Input{Title: "Outside Sales", WorkMode: "remote", Description: "This is not a remote work-from-home role, and a reliable vehicle is required."})
	if got.WorkMode != "onsite" {
		t.Errorf("WorkMode = %q, want onsite", got.WorkMode)
	}
}

func TestDerive_StructuredGeographyShadow(t *testing.T) {
	for _, tc := range []struct {
		location                 string
		countries, regions, want []string
	}{
		{"Anywhere, USA", []string{"us"}, []string{"global", "north_america"}, []string{"north_america"}},
		{"Remote anywhere or On-site Prague", []string{"cz"}, []string{"global", "eu"}, []string{"eu", "global"}},
	} {
		t.Run(tc.location, func(t *testing.T) {
			got := Derive(Input{Title: "Backend Engineer", Location: tc.location, Countries: tc.countries, Regions: tc.regions})
			if !reflect.DeepEqual(got.Regions, tc.want) {
				t.Fatalf("regions = %v, want %v", got.Regions, tc.want)
			}
		})
	}
}
func TestDerive_NegativeStructuredExperience(t *testing.T) {
	n := -1
	got := Derive(Input{Title: "Backend Engineer", ExperienceYearsMin: &n})
	if got.ExperienceYearsMin != nil {
		t.Fatalf("experience = %v, want nil", *got.ExperienceYearsMin)
	}
}
