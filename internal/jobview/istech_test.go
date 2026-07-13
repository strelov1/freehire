package jobview

import (
	"testing"

	"github.com/strelov1/freehire/internal/job"
)

func TestFromDomain_IsTechFacet(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string // "" = omitted (unknown)
	}{
		{"tech", "Senior Backend Developer", "tech"},
		{"non_tech", "Warehouse Janitorial Cleaner", "non_tech"},
		{"unknown", "Yard Coordinator", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j, err := job.New(job.Draft{Source: "src", ExternalID: "ext", Title: tt.title})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			v, err := FromDomain(j, job.Extras{})
			if err != nil {
				t.Fatalf("FromDomain: %v", err)
			}
			if v.IsTech != tt.want {
				t.Errorf("IsTech = %q, want %q", v.IsTech, tt.want)
			}
		})
	}
}
