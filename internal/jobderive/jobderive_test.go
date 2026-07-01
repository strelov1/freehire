package jobderive

import (
	"reflect"
	"testing"

	"github.com/strelov1/freehire/internal/normalize"
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
	if got.CompanySlug != normalize.Slug("Acme") {
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

func TestDerive_LocationWorkModeBeatsDescription(t *testing.T) {
	got := Derive(Input{
		Title:       "Dev",
		Company:     "Acme",
		Source:      "greenhouse",
		ExternalID:  "board:1",
		Location:    "Remote - Germany",       // location marker → remote
		Description: "This is a hybrid role.", // description → hybrid, but loses
	})
	if got.WorkMode != "remote" {
		t.Errorf("WorkMode = %q, want remote (location beats description)", got.WorkMode)
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
