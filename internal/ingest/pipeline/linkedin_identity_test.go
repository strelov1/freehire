package pipeline

import (
	"testing"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

func TestLinkedInIdentityDoesNotDependOnSearchKeyword(t *testing.T) {
	for _, keyword := range []string{"software engineer", "data engineer", ""} {
		source, id := jobIdentity(sources.CompanyEntry{Provider: "linkedin", Board: keyword}, sources.Job{ExternalID: "4490679811", URL: "https://www.linkedin.com/jobs/view/4490679811/"})
		if source != "linkedin" || id != "4490679811" {
			t.Errorf("keyword %q: identity=%s/%s, want linkedin/4490679811", keyword, source, id)
		}
	}
	for _, provider := range []string{"greenhouse", "himalayas"} {
		source, id := jobIdentity(sources.CompanyEntry{Provider: provider, Board: "acme"}, sources.Job{ExternalID: "4490679811", URL: "https://www.linkedin.com/jobs/view/4490679811/"})
		if source != provider || id != "acme:4490679811" {
			t.Errorf("%s identity=%s/%s, want original board namespace", provider, source, id)
		}
	}
}

func TestLinkedInIdentityRejectsConflictingNativeID(t *testing.T) {
	for _, posting := range []sources.Job{
		{ExternalID: "invalid", URL: "https://www.linkedin.com/jobs/view/4490679811/"},
		{ExternalID: "4490679812", URL: "https://www.linkedin.com/jobs/view/4490679811/"},
		{ExternalID: "4490679811", URL: "https://example.test/jobs/4490679811/"},
	} {
		_, id := jobIdentity(sources.CompanyEntry{Provider: "linkedin", Board: "software engineer"}, posting)
		if id != "" {
			t.Errorf("conflicting posting received writable identity %q", id)
		}
	}
}
