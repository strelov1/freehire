package sources

import "testing"

func TestCanonicalPostingURLLinkedInUsesNativeIdentity(t *testing.T) {
	for _, raw := range []string{
		"https://www.linkedin.com/jobs/view/senior-software-engineer-at-acme-4490679811/?trk=public_jobs",
		"https://nl.linkedin.com/jobs/view/software-engineer-4490679811",
		"https://www.linkedin.com/jobs/view/4490679811/?utm_source=freehire.me#apply",
	} {
		if got := CanonicalPostingURL(raw); got != linkedinCanonicalURLForTest {
			t.Errorf("canonical(%q)=%q, want %q", raw, got, linkedinCanonicalURLForTest)
		}
	}
	for _, raw := range []string{
		"https://linkedin.com.example.test/jobs/view/4490679811/",
		"https://www.linkedin.com/jobs/search/?keywords=4490679811",
		"https://www.linkedin.com/jobs/view/not-a-native-id/",
	} {
		if got := CanonicalPostingURL(raw); got != raw {
			t.Errorf("unrecognized LinkedIn identity changed: %q -> %q", raw, got)
		}
	}
}

const linkedinCanonicalURLForTest = "https://www.linkedin.com/jobs/view/4490679811/"
