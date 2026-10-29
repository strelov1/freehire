package location

import (
	"slices"
	"testing"
)

func TestRestrictionFromTitle(t *testing.T) {
	tests := []struct {
		name          string
		title         string
		wantCountries []string
		wantRegions   []string
	}{
		{
			"bracketed remote-US suffix",
			"Senior Back End Engineer [Remote-US]",
			[]string{"us"}, []string{"north_america"},
		},
		{
			"parenthetical location suffix with two countries",
			"Senior Backend Engineer (Go) (Location - Australia or New Zealand)",
			[]string{"au", "nz"}, []string{"apac"},
		},
		{
			"non-anchor bracket is ignored",
			"Frontend Engineer (React)",
			nil, nil,
		},
		{
			"non-anchor bracket with employment type is ignored",
			"Product Manager (Contract)",
			nil, nil,
		},
		{
			"anchor word with no resolvable place yields nothing",
			"Engineer [Remote-EMEA-ish]",
			nil, nil,
		},
		{
			"title with no bracket at all yields nothing",
			"Senior Software Developer",
			nil, nil,
		},
		{
			"closing bracket that is only a region",
			"Staff Product Engineer, Frontend (LATAM)",
			nil, []string{"latam"},
		},
		{
			"closing bracket that is only a country",
			"Senior Software Engineer, Applied AI (Netherlands)",
			[]string{"nl"}, []string{"eu"},
		},
		{
			"trailing dash segment that is only a country",
			"Senior. QA Automation Engineer (Java-Playwright)-India",
			[]string{"in"}, []string{"apac"},
		},
		{
			"leading macro-region word",
			"LATAM Senior QA Analyst (Data & ETL)",
			nil, []string{"latam"},
		},
		{
			// A bracket inside the title names a language's variety, not the role's place.
			"inner bracket qualifying a language is ignored",
			"Spanish (Latin America) Language Data Contributor",
			nil, nil,
		},
		{
			// A leading country is as often the market a role sells into.
			"leading country word is ignored",
			"Germany Sales Manager",
			nil, nil,
		},
		{
			"closing open-anywhere bracket without an anchor restricts nothing",
			"Backend Engineer (Worldwide)",
			nil, nil,
		},
		{
			"trailing segment that is not only a place is ignored",
			"Data Engineer - Germany or Remote EU",
			nil, nil,
		},
		{
			"empty title yields nothing",
			"",
			nil, nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotCountries, gotRegions := RestrictionFromTitle(tt.title)
			if !slices.Equal(gotCountries, tt.wantCountries) {
				t.Errorf("countries = %v, want %v", gotCountries, tt.wantCountries)
			}
			if !slices.Equal(gotRegions, tt.wantRegions) {
				t.Errorf("regions = %v, want %v", gotRegions, tt.wantRegions)
			}
		})
	}
}
