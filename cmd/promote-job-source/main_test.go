package main

import (
	"testing"

	"github.com/strelov1/freehire/internal/platform/db"
)

func TestRetryUsesThePromotedPostingAfterItsOverrideWasCleared(t *testing.T) {
	const native = "https://gistworld.mua.hrdepartment.com/hrsmart/ats/Posting/view/3735"
	if got := destinationURL(db.Job{Source: "hrsmart", URL: native}, ""); got != native {
		t.Fatalf("retry lost the native destination after transfer: %q", got)
	}
	if got := destinationURL(db.Job{Source: "linkedin", URL: "https://www.linkedin.com/jobs/view/4472812273/", ApplicationUrl: native}, ""); got != native {
		t.Fatalf("first transfer lost the verified destination: %q", got)
	}
	if got := destinationURL(db.Job{Source: "linkedin", URL: "https://www.linkedin.com/jobs/view/4472812273/"}, ""); got != "" {
		t.Fatalf("unverified LinkedIn URL passed as a native destination: %q", got)
	}
}
