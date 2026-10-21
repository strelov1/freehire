package jobview_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
)

// An employer vacancy taking applications on freehire is flagged on the wire and links
// nowhere; every other posting omits the flag entirely, so the field costs the search
// index nothing for the catalogue it does not describe.
func TestFromDomain_DirectApply(t *testing.T) {
	cases := map[string]struct {
		row      db.Job
		wantFlag bool
	}{
		"on-site employer vacancy": {
			row:      db.Job{Source: job.EmployerSource, ExternalID: job.DirectApplyPrefix + "0b6f", Title: "Dev", Company: "Acme", CreatedBy: pgtype.Int8{Int64: 9, Valid: true}},
			wantFlag: true,
		},
		"employer vacancy linking out": {
			row: db.Job{Source: job.EmployerSource, ExternalID: "https://acme.test/1", URL: "https://acme.test/1", Title: "Dev", Company: "Acme"},
		},
		"crawled posting": {
			row: db.Job{Source: "greenhouse", ExternalID: "acme:1", URL: "https://acme.test/1", Title: "Dev", Company: "Acme"},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			j, x, err := job.FromRow(tc.row)
			if err != nil {
				t.Fatalf("job.FromRow: %v", err)
			}
			got, err := jobview.FromDomain(j, x)
			if err != nil {
				t.Fatalf("FromDomain: %v", err)
			}
			if got.DirectApply != tc.wantFlag {
				t.Errorf("DirectApply = %v, want %v", got.DirectApply, tc.wantFlag)
			}
			raw, _ := json.Marshal(got)
			if has := strings.Contains(string(raw), `"direct_apply"`); has != tc.wantFlag {
				t.Errorf("direct_apply present in JSON = %v, want %v: %s", has, tc.wantFlag, raw)
			}
			if tc.wantFlag && got.URL != "" {
				t.Errorf("URL = %q, want empty for an on-site vacancy", got.URL)
			}
		})
	}
}
