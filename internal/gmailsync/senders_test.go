package gmailsync

import (
	"strings"
	"testing"
)

func TestIsATSSender(t *testing.T) {
	yes := []string{
		"no-reply@us.greenhouse-mail.io",
		"Sardine <no-reply@ashbyhq.com>",
		"Oowlish <no-reply@hire.lever.co>", // subdomain of lever.co
		"web@myworkday.com",
	}
	for _, s := range yes {
		if !IsATSSender(s) {
			t.Errorf("IsATSSender(%q) = false, want true", s)
		}
	}
	no := []string{"friend@gmail.com", "news@substack.com", "", "not-an-address"}
	for _, s := range no {
		if IsATSSender(s) {
			t.Errorf("IsATSSender(%q) = true, want false", s)
		}
	}
}

func TestBuildQuery(t *testing.T) {
	q := BuildQuery(1_700_000_000)
	if !strings.Contains(q, "from:(") {
		t.Errorf("query missing from:() clause: %q", q)
	}
	if !strings.Contains(q, "greenhouse-mail.io") || !strings.Contains(q, "ashbyhq.com") {
		t.Errorf("query missing ATS domains: %q", q)
	}
	if !strings.Contains(q, "after:1700000000") {
		t.Errorf("query missing after: watermark: %q", q)
	}
}

func TestBuildQueryNoWatermark(t *testing.T) {
	// A zero watermark (first run) omits the after: clause → full backfill.
	q := BuildQuery(0)
	if strings.Contains(q, "after:") {
		t.Errorf("zero watermark should omit after:, got %q", q)
	}
}
