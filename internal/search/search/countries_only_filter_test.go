package search

import (
	"net/url"
	"reflect"
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

// countries_only keeps postings whose geography is one place, so a worldwide remote posting
// that happens to list the selected country drops out. multi_country is true-or-absent, so the
// filter negates the positive and reaches documents indexed before the attribute existed.
func TestFilterFromValues_CountriesOnly(t *testing.T) {
	tests := []struct {
		name  string
		query string
		want  [][]string
	}{
		{
			name:  "true keeps one-country postings beside the country facet",
			query: "countries=ch&countries_only=true",
			// normalizeGroups sorts groups: "NOT …" sorts before "countries …".
			want: [][]string{{`NOT multi_country = true`}, {`countries = "ch"`}},
		},
		{
			name:  "true alone still narrows to one-country postings",
			query: "countries_only=true",
			want:  [][]string{{`NOT multi_country = true`}},
		},
		{
			name:  "anything else is off",
			query: "countries_only=false",
			want:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := url.ParseQuery(tt.query)
			if err != nil {
				t.Fatalf("ParseQuery: %v", err)
			}
			got := normalizeGroups(t, FilterFromValues(v))
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filter = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestFromJob_MultiCountry(t *testing.T) {
	one, err := FromJob(db.Job{ID: 1, PublicSlug: "ch", Countries: []string{"ch"}}, "")
	if err != nil {
		t.Fatalf("FromJob: %v", err)
	}
	if one.MultiCountry {
		t.Error("a one-country posting must not be multi_country")
	}
	many, err := FromJob(db.Job{ID: 2, PublicSlug: "eu", Countries: []string{"ch", "de"}}, "")
	if err != nil {
		t.Fatalf("FromJob: %v", err)
	}
	if !many.MultiCountry {
		t.Error("a two-country posting must be multi_country")
	}
}

func TestFacetSettings_MultiCountryIsFilterable(t *testing.T) {
	if !contains(facetSettings().FilterableAttributes, MultiCountryAttr) {
		t.Errorf("%s must be filterable for countries_only", MultiCountryAttr)
	}
}
