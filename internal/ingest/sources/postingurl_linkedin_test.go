package sources

import "testing"

func TestCanonicalPostingURLLinkedInUsesNativeIdentity(t *testing.T) {
	for _, raw := range []string{
		"https://www.linkedin.com/jobs/view/senior-software-engineer-at-acme-4490679811/?trk=public_jobs",
		"https://nl.linkedin.com/jobs/view/software-engineer-4490679811",
		"https://www.linkedin.com/jobs/view/4490679811/?utm_source=freehire.me#apply",
		"https://www.linkedin.com/jobs/search/?currentJobId=4490679811&keywords=go&location=Berlin",
		"https://www.linkedin.com/jobs/collections/recommended/?currentJobId=4490679811",
		"https://de.linkedin.com/jobs/search-results/?currentJobId=4490679811",
	} {
		if got := CanonicalPostingURL(raw); got != linkedinCanonicalURLForTest {
			t.Errorf("canonical(%q)=%q, want %q", raw, got, linkedinCanonicalURLForTest)
		}
	}
	for _, raw := range []string{
		"https://linkedin.com.example.test/jobs/view/4490679811/",
		"https://www.linkedin.com/jobs/search/?keywords=4490679811",
		"https://www.linkedin.com/jobs/view/not-a-native-id/",
		"https://linkedin.com.example.test/jobs/search/?currentJobId=4490679811",
		"https://www.linkedin.com/feed/?currentJobId=4490679811",
		"https://www.linkedin.com/jobs/search/?currentJobId=abc",
		"http://www.linkedin.com/jobs/search/?currentJobId=4490679811",
	} {
		if got := CanonicalPostingURL(raw); got != raw {
			t.Errorf("unrecognized LinkedIn identity changed: %q -> %q", raw, got)
		}
	}
}

const linkedinCanonicalURLForTest = "https://www.linkedin.com/jobs/view/4490679811/"
