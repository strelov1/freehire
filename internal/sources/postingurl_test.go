package sources_test

import (
	"testing"

	"github.com/strelov1/freehire/internal/sources"
)

func TestRefFromURL_Greenhouse(t *testing.T) {
	tests := []struct {
		name, url, wantExternalID string
	}{
		{
			// What an application form on job-boards.greenhouse.io looks like. The
			// job id rides `token`; `jr_id` is Greenhouse's internal requisition id,
			// which the catalog does not carry.
			name:           "embedded application form",
			url:            "https://job-boards.greenhouse.io/embed/job_app?for=stripe&jr_id=6a2444ad757ade085b6affd5&token=7826765",
			wantExternalID: "stripe:7826765",
		},
		{
			name:           "embedded form on the legacy boards host",
			url:            "https://boards.greenhouse.io/embed/job_app?for=stripe&token=7826765",
			wantExternalID: "stripe:7826765",
		},
		{
			name:           "job detail page",
			url:            "https://job-boards.greenhouse.io/stripe/jobs/7826765",
			wantExternalID: "stripe:7826765",
		},
		{
			name:           "job detail page on the legacy host, with tracking query",
			url:            "https://boards.greenhouse.io/stripe/jobs/7826765?gh_src=abc123",
			wantExternalID: "stripe:7826765",
		},
		{
			name:           "trailing slash",
			url:            "https://job-boards.greenhouse.io/stripe/jobs/7826765/",
			wantExternalID: "stripe:7826765",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, ok := sources.RefFromURL(tt.url)
			if !ok {
				t.Fatalf("RefFromURL(%q) did not resolve", tt.url)
			}
			if ref.Source != "greenhouse" {
				t.Errorf("source = %q, want greenhouse", ref.Source)
			}
			if ref.ExternalID != tt.wantExternalID {
				t.Errorf("external id = %q, want %q", ref.ExternalID, tt.wantExternalID)
			}
		})
	}
}

func TestRefFromURL_Unresolvable(t *testing.T) {
	tests := []struct{ name, url string }{
		{"empty", ""},
		{"not a url", "reCAPTCHA"},
		{"a board we do not recognise", "https://jobs.lever.co/acme/1234"},
		{"greenhouse board listing, no job", "https://job-boards.greenhouse.io/stripe"},
		{"embedded form without the job id", "https://job-boards.greenhouse.io/embed/job_app?for=stripe"},
		{"embedded form without the board", "https://job-boards.greenhouse.io/embed/job_app?token=7826765"},
		{"a non-numeric job id", "https://job-boards.greenhouse.io/stripe/jobs/not-an-id"},
		// The company's own careers page carries the job id but not the board
		// token, and the board is not derivable from the host — so this stays
		// unresolved rather than guessing a board from the domain.
		{"company careers page with gh_jid", "https://stripe.com/jobs/search?gh_jid=7954688"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if ref, ok := sources.RefFromURL(tt.url); ok {
				t.Fatalf("RefFromURL(%q) resolved to %+v, want no match", tt.url, ref)
			}
		})
	}
}

func TestRefFromURL_IsCaseInsensitiveAboutTheBoard(t *testing.T) {
	ref, ok := sources.RefFromURL("https://job-boards.greenhouse.io/embed/job_app?for=Stripe&token=7826765")
	if !ok {
		t.Fatal("did not resolve")
	}
	// Board tokens are lowercase in the catalog; a link that shouts still matches.
	if ref.ExternalID != "stripe:7826765" {
		t.Fatalf("external id = %q, want the lowercased board", ref.ExternalID)
	}
}

// The apply form is a different URL for the same posting, and it is where a
// candidate spends most of their time — so the extension asks about it more often
// than about the detail page. Every one of these is a real shape from the catalog.
func TestCanonicalPostingURL_DropsTheApplyForm(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{
			name: "ashby application",
			url:  "https://jobs.ashbyhq.com/truelogic/c6d2719d-3935-4e59-8446-26135d01957a/application",
			want: "https://jobs.ashbyhq.com/truelogic/c6d2719d-3935-4e59-8446-26135d01957a",
		},
		{
			name: "lever apply",
			url:  "https://jobs.eu.lever.co/avara/3c71c090-60d1-4563-b90f-6fba1a1f8419/apply",
			want: "https://jobs.eu.lever.co/avara/3c71c090-60d1-4563-b90f-6fba1a1f8419",
		},
		{
			name: "workable apply, trailing slash",
			url:  "https://apply.workable.com/1kosmos/j/435C7BA5E4/apply/",
			want: "https://apply.workable.com/1kosmos/j/435C7BA5E4",
		},
		{
			// CatsOne is multi-tenant per host (<tenant>.catsone.com), so this has to
			// match by domain label rather than a fixed hostname.
			name: "catsone apply, a tenant subdomain",
			url:  "https://emergitel.catsone.com/careers/7701/jobs/16841332-full-stack-software-engineer-react-nodejs-typescript/apply",
			want: "https://emergitel.catsone.com/careers/7701/jobs/16841332-full-stack-software-engineer-react-nodejs-typescript",
		},
		{
			name: "catsone apply, a different tenant",
			url:  "https://acme.catsone.com/careers/1/jobs/2-title/apply",
			want: "https://acme.catsone.com/careers/1/jobs/2-title",
		},
		{
			// The check that got us here is case-insensitive, so the cut must be too —
			// otherwise the suffix matches, nothing is removed, and the URL comes back
			// quietly altered instead of untouched.
			name: "a shouting suffix is still the apply form",
			url:  "https://jobs.ashbyhq.com/truelogic/c6d2719d/APPLICATION",
			want: "https://jobs.ashbyhq.com/truelogic/c6d2719d",
		},
		{
			name: "a query string survives — the caller's normalisation drops it",
			url:  "https://jobs.ashbyhq.com/truelogic/c6d2719d/application?utm_source=freehire.me",
			want: "https://jobs.ashbyhq.com/truelogic/c6d2719d?utm_source=freehire.me",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sources.CanonicalPostingURL(tt.url); got != tt.want {
				t.Errorf("CanonicalPostingURL(%q)\n = %q\nwant %q", tt.url, got, tt.want)
			}
		})
	}
}

// Only where the suffix is known to be the same posting. Elsewhere a path segment
// is a different page, and collapsing it would hand back the wrong vacancy.
func TestCanonicalPostingURL_LeavesEverythingElseAlone(t *testing.T) {
	for _, raw := range []string{
		"https://jobs.ashbyhq.com/truelogic/c6d2719d",
		"https://jobs.lever.co/avara/3c71c090",
		"https://careers.example.test/jobs/42/apply",
		"https://boards.greenhouse.io/stripe/jobs/7826765",
		"https://jobs.ashbyhq.com/truelogic",
		"https://acme.catsone.com/careers/1/jobs/2-title",
		// Ends in the apex's own label, not bracketed by it — must not match on a
		// coincidental hostname.
		"https://notcatsone.com/careers/1/jobs/2-title/apply",
		"https://catsone.com/careers/1/jobs/2-title/apply",
		"not a url",
		"",
	} {
		if got := sources.CanonicalPostingURL(raw); got != raw {
			t.Errorf("CanonicalPostingURL(%q) = %q, want it unchanged", raw, got)
		}
	}
}
