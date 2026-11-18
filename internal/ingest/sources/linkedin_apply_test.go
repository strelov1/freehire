package sources

import (
	"strings"
	"testing"
)

func TestLinkedInPostingDistinguishesNativeAndExternalApplications(t *testing.T) {
	const old = `<button data-modal="job-details-topcard-apply-modal">Apply</button>`
	for _, tc := range []struct{ name, control, method, target string }{
		{"native", `<button data-tracking-control-name="public_jobs_apply-link-onsite">Apply</button>`, "onsite", ""},
		{"external hidden", `<button data-modal="job-details-topcard-apply-modal"><icon data-svg-class-name="apply-button__offsite-apply-icon-svg"></icon>Apply</button>`, "external", ""},
		{"external URL", `<a data-tracking-control-name="public_jobs_apply-link-offsite" href="https://boards.greenhouse.io/acme/jobs/123">Apply</a>`, "external", "https://boards.greenhouse.io/acme/jobs/123"},
		{"unknown", old, "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := parseHTML(t, strings.Replace(linkedInOpenPage, old, tc.control, 1))
			j, err := LinkedInPosting(root, "https://www.linkedin.com/jobs/view/123/", "123")
			if err != nil || string(j.ApplyMethod) != tc.method || j.ExternalApplyURL != tc.target {
				t.Fatalf("application method = %+v, %v", j, err)
			}
		})
	}
}
