package buddy

import (
	"reflect"
	"regexp"
	"testing"

	"github.com/strelov1/freehire/internal/candidate/resumeextract"
)

func cvWithTitle(title string) resumeextract.Structured {
	return resumeextract.Structured{Experience: []resumeextract.Experience{{Title: title, Current: true}}}
}

func TestRankingKeys(t *testing.T) {
	tests := []struct {
		name string
		in   Inputs
		want Keys
	}{
		{"category from title", Inputs{CV: cvWithTitle("Senior Backend Engineer")}, Keys{Category: "backend"}},
		{"title wins over specialization", Inputs{CV: cvWithTitle("Senior Backend Engineer"), Specializations: []string{"design"}}, Keys{Category: "backend"}},
		{"specialization fallback", Inputs{Specializations: []string{"data", "design"}}, Keys{Category: "data"}},
		{"nothing known", Inputs{}, Keys{}},
		{"cv geography wins", Inputs{Countries: []string{"de"}, Regions: []string{"eu"}, BaseCountry: "us"}, Keys{Countries: []string{"de"}, Regions: []string{"eu"}}},
		{"empty-but-known cv geography is kept", Inputs{Countries: []string{}, BaseCountry: "us"}, Keys{Countries: []string{}}},
		{"base country fallback", Inputs{BaseCountry: "us"}, Keys{Countries: []string{"us"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := rankingKeys(tt.in); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("rankingKeys = %+v, want %+v", got, tt.want)
			}
		})
	}
}

var handleShape = regexp.MustCompile(`^[a-z]+-[a-z]+-[1-9][0-9]{1,3}$`)

func TestGenerateHandleShape(t *testing.T) {
	for range 50 {
		h := GenerateHandle()
		if !handleShape.MatchString(h) {
			t.Fatalf("unexpected handle %q", h)
		}
	}
}
