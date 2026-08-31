package jobview

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/jobderive"
)

// The facet is served only when it is true. An unknown posting carries no key at
// all, so a consumer cannot mistake "we did not detect a requirement" for "this job
// needs no clearance" — a distinction the dictionary cannot make and must not imply.
func TestFromDomain_RequiresClearanceFacet(t *testing.T) {
	tests := []struct {
		name    string
		desc    string
		want    bool
		wantKey bool
	}{
		{
			name:    "stated requirement is served as true",
			desc:    "You must hold or be eligible for SC clearance.",
			want:    true,
			wantKey: true,
		},
		{
			name:    "unknown is omitted entirely",
			desc:    "We use Go and Kubernetes.",
			want:    false,
			wantKey: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j, err := job.New(job.Draft{Input: jobderive.Input{
				Source:      "greenhouse",
				ExternalID:  "acme:1",
				Title:       "Backend Engineer",
				Company:     "Acme",
				Description: tt.desc,
			}})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			v, err := FromDomain(j, job.Extras{})
			if err != nil {
				t.Fatalf("FromDomain: %v", err)
			}
			if v.RequiresClearance != tt.want {
				t.Errorf("RequiresClearance = %v, want %v", v.RequiresClearance, tt.want)
			}

			b, err := json.Marshal(v)
			if err != nil {
				t.Fatalf("Marshal: %v", err)
			}
			if gotKey := strings.Contains(string(b), `"requires_clearance"`); gotKey != tt.wantKey {
				t.Errorf("requires_clearance key present = %v, want %v", gotKey, tt.wantKey)
			}
		})
	}
}
