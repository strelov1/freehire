package sources_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/sources"
)

// fakeJSON serves one canned JSON body per URL and records what was asked for.
type fakeJSON struct {
	bodies map[string]string
	asked  []string
}

func (f *fakeJSON) GetJSON(_ context.Context, url string, v any) error {
	f.asked = append(f.asked, url)
	body, ok := f.bodies[url]
	if !ok {
		return errors.New("not found")
	}
	return json.Unmarshal([]byte(body), v)
}

const blend360Publication = "https://api.smartrecruiters.com/v1/companies/Blend360/postings/59957d76-615a-4809-a282-bcee1120ca7d"

func TestPostingURLResolver_SmartRecruitersOneClick(t *testing.T) {
	http := &fakeJSON{bodies: map[string]string{
		blend360Publication: `{"id":"744000143615340","postingUrl":"https://jobs.smartrecruiters.com/Blend360/744000143615340-senior-ai-engineer"}`,
	}}
	r := sources.NewPostingURLResolver(http)

	const oneClick = "https://jobs.smartrecruiters.com/oneclick-ui/company/Blend360/publication/59957d76-615a-4809-a282-bcee1120ca7d?dcr_ci=Blend360"
	const want = "https://jobs.smartrecruiters.com/Blend360/744000143615340-senior-ai-engineer"

	if got := r.CanonicalPostingURL(context.Background(), oneClick); got != want {
		t.Errorf("CanonicalPostingURL(one-click)\n = %q\nwant %q", got, want)
	}
	if len(http.asked) != 1 || http.asked[0] != blend360Publication {
		t.Errorf("asked %v, want a single call to %q", http.asked, blend360Publication)
	}
}

// A publication SmartRecruiters will not answer for (withdrawn, or a company that
// renamed) leaves the URL alone: an unanswered lookup is not evidence about the page,
// and the caller's second tier still gets to try the URL as it stands.
func TestPostingURLResolver_UnresolvablePublicationIsLeftAlone(t *testing.T) {
	r := sources.NewPostingURLResolver(&fakeJSON{})

	const oneClick = "https://jobs.smartrecruiters.com/oneclick-ui/company/Blend360/publication/59957d76-615a-4809-a282-bcee1120ca7d"
	if got := r.CanonicalPostingURL(context.Background(), oneClick); got != oneClick {
		t.Errorf("CanonicalPostingURL = %q, want it unchanged", got)
	}
}

// Every other link is answered offline, by the pure rewrite — no ATS is called for a
// page whose posting URL the link already is.
func TestPostingURLResolver_OtherLinksNeverCallOut(t *testing.T) {
	http := &fakeJSON{}
	r := sources.NewPostingURLResolver(http)

	cases := map[string]string{
		// The pure apply-form rewrite still applies.
		"https://jobs.ashbyhq.com/truelogic/c6d2719d/application": "https://jobs.ashbyhq.com/truelogic/c6d2719d",
		// A SmartRecruiters detail page is already the posting URL.
		"https://jobs.smartrecruiters.com/Blend360/744000143615340-senior-ai-engineer": "https://jobs.smartrecruiters.com/Blend360/744000143615340-senior-ai-engineer",
		// The one-click path with something other than a publication uuid in it.
		"https://jobs.smartrecruiters.com/oneclick-ui/company/Blend360/publication/not-a-uuid": "https://jobs.smartrecruiters.com/oneclick-ui/company/Blend360/publication/not-a-uuid",
		"not a url": "not a url",
	}
	for raw, want := range cases {
		if got := r.CanonicalPostingURL(context.Background(), raw); got != want {
			t.Errorf("CanonicalPostingURL(%q)\n = %q\nwant %q", raw, got, want)
		}
	}
	if len(http.asked) != 0 {
		t.Errorf("called out for %v, want no calls", http.asked)
	}
}

// The zero resolver is the pure rewrite: a caller with no HTTP client (a test, a worker
// that has no business dialling out) still gets correct offline canonicalisation rather
// than a nil-pointer panic.
func TestPostingURLResolver_ZeroValueStaysOffline(t *testing.T) {
	var r sources.PostingURLResolver

	const oneClick = "https://jobs.smartrecruiters.com/oneclick-ui/company/Blend360/publication/59957d76-615a-4809-a282-bcee1120ca7d"
	if got := r.CanonicalPostingURL(context.Background(), oneClick); got != oneClick {
		t.Errorf("CanonicalPostingURL = %q, want it unchanged", got)
	}
	if got := r.CanonicalPostingURL(context.Background(), "https://jobs.lever.co/avara/3c71c090/apply"); got != "https://jobs.lever.co/avara/3c71c090" {
		t.Errorf("CanonicalPostingURL = %q, want the apply suffix dropped", got)
	}
}
