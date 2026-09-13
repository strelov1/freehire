package talentnetwork

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/candidate/resumeextract"
)

// leak is a token that appears nowhere in any dictionary, so if it shows up in a
// marshalled card it can only have come through from the CV.
const leak = "umbrellacorp"

// cvWithLeakIn returns a CV whose ONLY occurrence of `leak` is in the named place. Each
// case is a separate path a real employer name travels: masking the company column and
// calling it done is what this test exists to disprove.
func cvWithLeakIn(where string) resumeextract.Structured {
	s := resumeextract.Structured{
		FullName:   "Ivan Strelov",
		Headline:   "Senior Backend Engineer",
		TotalYears: 8,
		Skills:     []string{"Go", "PostgreSQL"},
		Experience: []resumeextract.Experience{{
			Title:   "Senior Backend Engineer",
			Company: "Acme",
			Start:   date(2024, 1),
			Current: true,
			Stack:   []string{"Go", "Kubernetes"},
		}},
		Education: []resumeextract.Education{{Degree: "BSc Computer Science", Institution: "Some University"}},
		Projects:  []resumeextract.Project{{Name: "Billing rewrite", Link: "github.com/ivan"}},
	}
	switch where {
	case "company":
		s.Experience[0].Company = leak
	case "summary":
		s.Summary = "Led the platform team at " + leak
	case "role summary":
		s.Experience[0].Summary = "Rebuilt " + leak + "'s billing pipeline"
	case "highlights":
		s.Experience[0].Highlights = []string{"Cut " + leak + " infra spend by 40%"}
	case "title":
		s.Experience[0].Title = "Backend Engineer @ " + leak
	case "headline":
		s.Headline = "Backend Engineer at " + leak
	case "project name":
		s.Projects[0].Name = leak + " billing rewrite"
	case "project highlights":
		s.Projects[0].Highlights = []string{"Shipped for " + leak}
	case "institution":
		s.Education[0].Institution = leak + " University"
	case "degree":
		s.Education[0].Degree = "BSc, " + leak + " Institute"
	case "location":
		s.Location = leak + " Tower, Berlin"
	case "certifications":
		s.Certifications = []string{leak + " Certified Engineer"}
	case "languages":
		s.Languages = []string{"English", leak}
	case "skills":
		s.Skills = append(s.Skills, leak)
	case "stack":
		s.Experience[0].Stack = append(s.Experience[0].Stack, leak)
	default:
		panic("unknown place: " + where)
	}
	return s
}

// The invariant this whole feature rests on: nothing a candidate typed reaches the
// public card unless a dictionary resolved it. Masking named fields is not enough — the
// employer's name is usually sitting in the prose beside the field that was masked.
func TestProjectCard_LeaksNothingFromTheCV(t *testing.T) {
	places := []string{
		"company", "summary", "role summary", "highlights", "title", "headline",
		"project name", "project highlights", "institution", "degree", "location",
		"certifications", "languages", "skills", "stack",
	}
	for _, where := range places {
		t.Run(where, func(t *testing.T) {
			out, err := json.Marshal(ProjectCard(cvWithLeakIn(where)))
			if err != nil {
				t.Fatalf("marshal card: %v", err)
			}
			if strings.Contains(strings.ToLower(string(out)), leak) {
				t.Errorf("card carries %q from %s:\n%s", leak, where, out)
			}
		})
	}
}

// The candidate's own name and contact details are the other half of the promise.
func TestProjectCard_CarriesNoIdentity(t *testing.T) {
	s := resumeextract.Structured{
		FullName: "Ivan Strelov",
		Email:    "ivan@example.com",
		Phone:    "+49 170 1234567",
		Location: "Berlin, Germany",
		Links:    []string{"github.com/ivan"},
	}
	out, err := json.Marshal(ProjectCard(s))
	if err != nil {
		t.Fatalf("marshal card: %v", err)
	}
	for _, forbidden := range []string{"Ivan", "Strelov", "example.com", "1234567", "github.com"} {
		if strings.Contains(string(out), forbidden) {
			t.Errorf("card carries %q:\n%s", forbidden, out)
		}
	}
}

func TestProjectCard_CarriesTheProfessionalSignal(t *testing.T) {
	card := ProjectCard(resumeextract.Structured{
		TotalYears: 8,
		Skills:     []string{"Go", "PostgreSQL", "Not A Real Skill"},
		Experience: []resumeextract.Experience{{
			Title:   "Senior Backend Engineer",
			Company: "Acme",
			Start:   date(2024, 1),
			Current: true,
			Stack:   []string{"Go", "Kubernetes"},
		}},
	})

	if card.Category != "backend" || card.Seniority != "senior" {
		t.Errorf("card heading = %q/%q, want senior/backend", card.Seniority, card.Category)
	}
	if card.TotalYears != 8 {
		t.Errorf("total years = %d, want 8", card.TotalYears)
	}
	if len(card.Roles) != 1 {
		t.Fatalf("roles = %d, want 1", len(card.Roles))
	}
	if !card.Roles[0].Current || card.Roles[0].Start == nil {
		t.Error("the role lost its period")
	}
	if len(card.Roles[0].Stack) == 0 {
		t.Error("the role lost its stack")
	}
}

// A token outside the dictionary is dropped, and its resolved neighbours survive. That
// is skilltag's own "never guess" rule doing the whitelisting for us.
func TestProjectCard_KeepsOnlyResolvedSkills(t *testing.T) {
	card := ProjectCard(resumeextract.Structured{
		Skills: []string{"Go", "PostgreSQL", "Vibes", "Synergy"},
	})
	if len(card.Skills) == 0 {
		t.Fatal("every skill was dropped")
	}
	for _, s := range card.Skills {
		if s == "Vibes" || s == "Synergy" {
			t.Errorf("card carries the unresolved skill %q", s)
		}
	}
}

// A title the dictionary cannot place still yields a role. Dropping it would make a work
// history look shorter than it is, which is a lie of a different kind.
func TestProjectCard_KeepsARoleWithNoResolvedTitle(t *testing.T) {
	// "Lighthouse Keeper" resolves to neither field. "Chief Vibes Officer" would not do:
	// classify reads "Chief ... Officer" as the c_level grade, correctly, which would
	// make this test pass for the wrong reason.
	card := ProjectCard(resumeextract.Structured{
		Experience: []resumeextract.Experience{{
			Title: "Lighthouse Keeper",
			Start: date(2022, 5),
			End:   date(2024, 4),
			Stack: []string{"Go"},
		}},
	})
	if len(card.Roles) != 1 {
		t.Fatalf("roles = %d, want the unresolved role kept", len(card.Roles))
	}
	if card.Roles[0].Category != "" || card.Roles[0].Seniority != "" {
		t.Error("an unresolved title must not be guessed at")
	}
	if card.Roles[0].Start == nil || card.Roles[0].End == nil {
		t.Error("the unresolved role lost its period")
	}
}

// A resolvable degree carries its level and year, and nothing else from the entry —
// no institution, in any field.
func TestProjectCard_CarriesResolvedEducation(t *testing.T) {
	card := ProjectCard(resumeextract.Structured{
		Education: []resumeextract.Education{
			{Degree: "BSc Computer Science", Institution: "Some University", Year: date(2019, 6)},
		},
	})
	if len(card.Education) != 1 {
		t.Fatalf("education = %d, want 1", len(card.Education))
	}
	if card.Education[0].Level != "bachelor" {
		t.Errorf("education level = %q, want bachelor", card.Education[0].Level)
	}
	if card.Education[0].Year == nil || card.Education[0].Year.Year != 2019 {
		t.Error("the resolved education entry lost its year")
	}
}

// An education entry whose degree resolves to nothing is dropped entirely — unlike an
// unresolved role, there is no "gap reads worse than absence" argument for education.
func TestProjectCard_DropsUnresolvedEducation(t *testing.T) {
	card := ProjectCard(resumeextract.Structured{
		Education: []resumeextract.Education{
			{Degree: "Certificate in Project Management", Institution: "Some Institute"},
			{Degree: "MSc Data Science", Year: date(2021, 1)},
		},
	})
	if len(card.Education) != 1 {
		t.Fatalf("education = %d, want the unresolvable entry dropped", len(card.Education))
	}
	if card.Education[0].Level != "master" {
		t.Errorf("education level = %q, want the resolvable entry's master", card.Education[0].Level)
	}
}

// A degree with no stated year still resolves — Year is optional on the entry, not a
// condition for keeping it.
func TestProjectCard_CarriesResolvedEducationWithNoYear(t *testing.T) {
	card := ProjectCard(resumeextract.Structured{
		Education: []resumeextract.Education{{Degree: "PhD in Physics"}},
	})
	if len(card.Education) != 1 {
		t.Fatalf("education = %d, want 1", len(card.Education))
	}
	if card.Education[0].Level != "phd" {
		t.Errorf("education level = %q, want phd", card.Education[0].Level)
	}
	if card.Education[0].Year != nil {
		t.Errorf("education year = %v, want nil (none was stated)", card.Education[0].Year)
	}
}

// A certification the dictionary resolves is canonicalized; one it does not is dropped.
func TestProjectCard_KeepsOnlyResolvedCertifications(t *testing.T) {
	card := ProjectCard(resumeextract.Structured{
		Certifications: []string{"PMP", "Certified Underwater Basket Weaver"},
	})
	if len(card.Certifications) != 1 || card.Certifications[0] != "pmp" {
		t.Errorf("certifications = %v, want only [pmp]", card.Certifications)
	}
}

// Multiple resolved certifications all survive, sorted.
func TestProjectCard_CarriesMultipleResolvedCertifications(t *testing.T) {
	card := ProjectCard(resumeextract.Structured{
		Certifications: []string{"CISSP", "CKAD", "PMP"},
	})
	want := []string{"cissp", "ckad", "pmp"}
	if len(card.Certifications) != len(want) {
		t.Fatalf("certifications = %v, want %v", card.Certifications, want)
	}
	for i, c := range want {
		if card.Certifications[i] != c {
			t.Errorf("certifications = %v, want %v", card.Certifications, want)
			break
		}
	}
}
