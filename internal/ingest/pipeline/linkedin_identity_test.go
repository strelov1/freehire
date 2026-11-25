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

func TestIndeedIdentityIsTheGlobalJobKey(t *testing.T) {
	for _, keyword := range []string{"software engineer", "golang"} {
		e := sources.CompanyEntry{Provider: "indeed", Board: keyword}
		if source, id := jobIdentity(e, sources.Job{ExternalID: "b2d02bb2a34565d6"}); source != "indeed" || id != "b2d02bb2a34565d6" {
			t.Errorf("keyword %q: identity=%s/%s, want indeed/b2d02bb2a34565d6", keyword, source, id)
		}
		if seenScope(e) != "" || seenKey(e, "b2d02bb2a34565d6") != "b2d02bb2a34565d6" {
			t.Errorf("keyword %q: seen lookup scope %q key %q, want provider-wide by job key", keyword, seenScope(e), seenKey(e, "b2d02bb2a34565d6"))
		}
	}
	if _, id := jobIdentity(sources.CompanyEntry{Provider: "indeed", Board: "golang"}, sources.Job{ExternalID: "not-a-key"}); id != "" {
		t.Errorf("malformed key received writable identity %q", id)
	}
}
