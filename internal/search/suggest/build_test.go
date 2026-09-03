package suggest

import (
	"slices"
	"strings"
	"testing"
)

func slugs(docs []Document) []string {
	out := make([]string, 0, len(docs))
	for _, d := range docs {
		out = append(out, string(d.Kind)+":"+d.Slug)
	}
	return out
}

func texts(docs []Document) []string {
	out := make([]string, 0, len(docs))
	for _, d := range docs {
		out = append(out, d.Text)
	}
	return out
}

// Titles are the whole reason this dictionary exists: they are the vocabulary the
// market actually writes, and the facet dictionaries do not carry them.
func TestBuild_Titles(t *testing.T) {
	docs := Build(Input{
		Titles: map[string]int{
			"Product Owner": 5, "product owner": 4, "PRODUCT OWNER": 3,
			"Java Developer": 9,
			"One Off Role":   1,
		},
		TitleFloor: 10,
	})

	// The three spellings are one job. Merging them is what a floor of 10 has to see:
	// each spelling alone is under it, and together they are 12.
	if !slices.Contains(texts(docs), "Product Owner") {
		t.Fatalf("merged spellings must clear the floor together, got %v", texts(docs))
	}
	if slices.Contains(texts(docs), "Java Developer") {
		t.Error("a title under the floor must not be offered")
	}
	if slices.Contains(texts(docs), "One Off Role") {
		t.Error("a one-off title is noise, not a suggestion")
	}

	for _, d := range docs {
		if d.Text == "Product Owner" && d.Jobs != 12 {
			t.Errorf("merged count = %d, want 12", d.Jobs)
		}
	}
}

// A mined title is normalised to lowercase so its spellings merge. That is the KEY,
// not the label: "product owner" in a dropdown among "Backend Engineer" and "Google"
// reads as a bug. The document carries the display form and merges on the normalised
// one.
func TestBuild_ATitleIsOfferedTheWayItIsWritten(t *testing.T) {
	docs := Build(Input{Titles: map[string]int{"PRODUCT OWNER": 20}, TitleFloor: 1})
	if len(docs) != 1 {
		t.Fatalf("got %v", texts(docs))
	}
	if docs[0].Text != "Product Owner" {
		t.Errorf("text = %q, want %q", docs[0].Text, "Product Owner")
	}
}

func TestBuild_DropsATitleThatNamesNoCraft(t *testing.T) {
	docs := Build(Input{
		Titles:     map[string]int{"Manager": 500, "Engineering Manager": 500},
		TitleFloor: 10,
	})
	if slices.Contains(texts(docs), "Manager") {
		t.Error("a bare generic clears any floor and answers nothing")
	}
	if !slices.Contains(texts(docs), "Engineering Manager") {
		t.Error("the same noun qualified by a craft is a real suggestion")
	}
}

// A bare-category role and its category select the same postings — measured on the
// live catalogue, role `devops` counts 53,250 against category `devops` at 53,251.
// Offering both puts one filter in the dropdown twice, which is the confusion this
// feature exists to remove.
func TestBuild_ACategoryLosesToTheRoleThatSharesItsSlug(t *testing.T) {
	docs := Build(Input{
		Roles:      map[string]int{"devops": 53250},
		RoleLabels: map[string]string{"devops": "DevOps Engineer"},
		Categories: map[string]int{"devops": 53251, "healthcare": 77787},
	})
	got := slugs(docs)
	if !slices.Contains(got, "role:devops") {
		t.Error("the role names a job and wins")
	}
	if slices.Contains(got, "category:devops") {
		t.Error("the category is the same postings under a department's name")
	}
	if !slices.Contains(got, "category:healthcare") {
		t.Error("a category with no matching role survives")
	}
}

// The catalogue carries every grade as its own role slug and graded slugs outnumber
// ungraded ones about six to one, so offering each would spend a whole dropdown on
// one role's grades.
func TestBuild_OneRowPerBaseRole(t *testing.T) {
	docs := Build(Input{
		Roles: map[string]int{
			"data_analytics": 77367, "senior_data_analytics": 20000,
			"junior_data_analytics": 5000, "data_engineering": 42072,
		},
		RoleLabels: map[string]string{
			"data_analytics": "Data Analyst", "senior_data_analytics": "Senior Data Analyst",
			"junior_data_analytics": "Junior Data Analyst", "data_engineering": "Data Engineer",
		},
	})
	got := slugs(docs)
	if !slices.Contains(got, "role:data_analytics") || !slices.Contains(got, "role:data_engineering") {
		t.Fatalf("both base roles must survive, got %v", got)
	}
	if slices.Contains(got, "role:senior_data_analytics") {
		t.Error("a graded slug is the base role again, under a longer name")
	}
}

func TestBuild_CompaniesAndSkillsCarryTheirCounts(t *testing.T) {
	docs := Build(Input{
		Companies: []Company{{Slug: "google", Name: "Google", Jobs: 3187}},
		Skills:    map[string]int{"java": 83676},
	})
	var company, skill *Document
	for i := range docs {
		switch docs[i].Kind {
		case KindCompany:
			company = &docs[i]
		case KindSkill:
			skill = &docs[i]
		}
	}
	if company == nil || company.Text != "Google" || company.Jobs != 3187 {
		t.Errorf("company doc = %+v", company)
	}
	if skill == nil || skill.Jobs != 83676 {
		t.Errorf("skill doc = %+v", skill)
	}
}

// Nothing may carry a zero: a suggestion with no postings behind it leads to an empty
// page, which is worse than no suggestion.
func TestBuild_NothingWithoutPostings(t *testing.T) {
	docs := Build(Input{
		Roles:      map[string]int{"backend": 0},
		RoleLabels: map[string]string{"backend": "Backend Engineer"},
		Categories: map[string]int{"qa": 0},
		Skills:     map[string]int{"cobol": 0},
		Companies:  []Company{{Slug: "gone", Name: "Gone", Jobs: 0}},
	})
	if len(docs) != 0 {
		t.Errorf("nothing measured at zero may be offered, got %v", slugs(docs))
	}
}

// The document id is what Meilisearch dedupes on, so two kinds sharing a slug must
// not collide: `backend` is a plausible role AND a plausible category.
func TestBuild_IdsAreUniqueAcrossKinds(t *testing.T) {
	docs := Build(Input{
		Roles:      map[string]int{"backend": 100},
		RoleLabels: map[string]string{"backend": "Backend Engineer"},
		Skills:     map[string]int{"backend": 200},
		Titles:     map[string]int{"backend": 300},
		TitleFloor: 1,
	})
	seen := map[string]bool{}
	for _, d := range docs {
		if seen[d.ID] {
			t.Fatalf("duplicate id %q", d.ID)
		}
		seen[d.ID] = true
	}
	if len(docs) != 3 {
		t.Errorf("want three documents sharing a slug, got %v", slugs(docs))
	}
}

// Demand is what the endpoint ranks by first: what people actually ask for beats what
// merely exists a lot of. The join is on the NORMALISED phrase, which is the only
// reason a typed query and a mined title can meet at all.
func TestBuild_CarriesRecordedDemand(t *testing.T) {
	docs := Build(Input{
		Roles:      map[string]int{"backend": 8000},
		RoleLabels: map[string]string{"backend": "Backend Engineer"},
		Skills:     map[string]int{"cobol": 12},
		Searches:   map[string]int{"backend engineer": 40},
	})
	for _, d := range docs {
		switch d.Kind {
		case KindRole:
			if d.Searches != 40 {
				t.Errorf("role searches = %d, want 40", d.Searches)
			}
		case KindSkill:
			// Nobody having asked for it is not evidence against it — the posting count
			// still orders it, so it is offered with a zero rather than withheld.
			if d.Searches != 0 {
				t.Errorf("unsearched skill searches = %d, want 0", d.Searches)
			}
		}
	}
	if len(docs) != 2 {
		t.Errorf("an unsearched suggestion must still be offered, got %v", slugs(docs))
	}
}

func TestBuild_DemandJoinsAMinedTitle(t *testing.T) {
	docs := Build(Input{
		Titles:     map[string]int{"Product Owner": 20},
		TitleFloor: 1,
		Searches:   map[string]int{"product owner": 99},
	})
	if len(docs) != 1 || docs[0].Searches != 99 {
		t.Errorf("a typed query must reach the title it names, got %+v", docs)
	}
}

// Meilisearch accepts only letters, digits, hyphens and underscores in a document
// identifier. The first production build got as far as writing and then failed on the
// very first document:
//
//	Document identifier `"company:01-tech"` is invalid.
//
// The colon was a readable namespace separator and an illegal character, and the
// existing tests could not see it: they asserted ids were UNIQUE, which is a property
// of the set, while validity is a property the engine alone defines. This asserts the
// engine's rule against every kind, including the two that produced it — a company
// slug and a mined title, which carry the widest characters of anything here.
func TestBuild_IdsAreAcceptableToTheEngine(t *testing.T) {
	docs := Build(Input{
		Titles:     map[string]int{"Node.js Developer (m/f/d)": 50, "C++ Engineer": 50},
		TitleFloor: 1,
		Roles:      map[string]int{"senior_backend": 10},
		RoleLabels: map[string]string{"senior_backend": "Senior Backend Engineer"},
		Categories: map[string]int{"ml_ai": 10},
		Skills:     map[string]int{"ci-cd": 10, "node.js": 10},
		Companies: []Company{
			{Slug: "01-tech", Name: "01 Tech", Jobs: 5},
			{Slug: "at&t", Name: "AT&T", Jobs: 5},
		},
	})
	if len(docs) == 0 {
		t.Fatal("nothing built")
	}
	for _, d := range docs {
		for _, r := range d.ID {
			ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
				(r >= '0' && r <= '9') || r == '-' || r == '_'
			if !ok {
				t.Errorf("id %q carries %q, which Meilisearch refuses", d.ID, r)
				break
			}
		}
		if len(d.ID) > 511 {
			t.Errorf("id %q is %d bytes, over the 511 the engine allows", d.ID, len(d.ID))
		}
	}
}

// Namespacing is what the id is FOR — `backend` is a plausible role, skill and
// category — so whatever encoding keeps it legal must keep it distinct too.
func TestBuild_IdsStayDistinctAcrossKindsAfterEncoding(t *testing.T) {
	docs := Build(Input{
		Roles:      map[string]int{"backend": 100},
		RoleLabels: map[string]string{"backend": "Backend"},
		Skills:     map[string]int{"backend": 200},
		Categories: map[string]int{"backend": 300},
		Titles:     map[string]int{"backend": 400},
		TitleFloor: 1,
	})
	seen := map[string]bool{}
	for _, d := range docs {
		if seen[d.ID] {
			t.Fatalf("duplicate id %q", d.ID)
		}
		seen[d.ID] = true
	}
}

// Hex doubles the length, so the engine's 511-byte id ceiling becomes a ~255-byte
// ceiling on the value. A mined title has no such bound of its own — `Title` cuts at
// the first separator, so a long run without one survives whole — and the id would be
// rejected at write time, killing the build over one absurd posting.
//
// The judgement is the same one the demand path already makes: what is worth offering
// is a search PHRASE, and a phrase is short. So `Recordable` gates both.
func TestBuild_ARunawayTitleIsNotVocabulary(t *testing.T) {
	long := strings.Repeat("engineering ", 40) // no separator, ~480 bytes
	docs := Build(Input{
		Titles:     map[string]int{long: 500, "Data Engineer": 500},
		TitleFloor: 1,
	})
	if len(docs) != 1 || docs[0].Text != "Data Engineer" {
		t.Fatalf("got %v", texts(docs))
	}
}
