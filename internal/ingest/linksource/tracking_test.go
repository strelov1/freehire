package linksource

import (
	"slices"
	"testing"
)

// The vocabulary is shared with the database's normalize_job_url, so the names it exports
// are a contract: every one must be a parameter that never vetoes a link match here.
func TestTrackingParamNamesNeverVetoALinkMatch(t *testing.T) {
	const posting = "https://acme.example/careers/job?jobid=42"
	names := TrackingParamNames()
	if len(names) == 0 {
		t.Fatal("TrackingParamNames is empty")
	}
	if !slices.IsSorted(names) {
		t.Errorf("TrackingParamNames is not sorted: %v", names)
	}
	for _, k := range names {
		link := posting + "&" + k + "=x"
		if !sameURL(link, posting) || !sameURL(posting, posting+"&"+k+"=y") {
			t.Errorf("tracking param %q vetoed a match", k)
		}
	}
}

func TestTrackingParamNamesCoverTheWellKnownTags(t *testing.T) {
	names := TrackingParamNames()
	for _, k := range []string{"gclid", "fbclid", "lever-source", "refid", "trackingid", "mc_cid", "_hsenc"} {
		if !slices.Contains(names, k) {
			t.Errorf("TrackingParamNames lacks %q", k)
		}
	}
	// The utm_ family is matched by prefix, not listed.
	if !trackingParam("UTM_Campaign") {
		t.Error("utm_ prefix not recognised case-insensitively")
	}
}
