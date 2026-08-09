package adzunadesc

import (
	"context"
	"errors"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

type fakeHTML struct {
	body   string
	err    error
	gotURL string
}

func (f *fakeHTML) GetHTML(_ context.Context, url string) (*html.Node, error) {
	f.gotURL = url
	if f.err != nil {
		return nil, f.err
	}
	return html.Parse(strings.NewReader(f.body))
}

const pageWithJobPosting = `<!DOCTYPE html><html><head>
<script type="application/ld+json">
{"@context":"http://schema.org/","@type":"JobPosting","title":"Solutions Architect","description":"<h2><strong>About MongoDB</strong></h2><p>Great role.</p>"}
</script>
</head><body></body></html>`

const pageWithoutJobPosting = `<!DOCTYPE html><html><head><title>Access Denied</title></head><body>blocked</body></html>`

const pageWithEmptyDescription = `<!DOCTYPE html><html><head>
<script type="application/ld+json">
{"@context":"http://schema.org/","@type":"JobPosting","title":"Solutions Architect","description":""}
</script>
</head><body></body></html>`

func TestFetchDescription(t *testing.T) {
	t.Run("extracts and sanitizes the JobPosting description", func(t *testing.T) {
		f := &fakeHTML{body: pageWithJobPosting}
		got, err := FetchDescription(context.Background(), f, "https://www.adzuna.co.uk/jobs/details/1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "<h2><strong>About MongoDB</strong></h2><p>Great role.</p>"
		if got != want {
			t.Errorf("got %q, want %q", got, want)
		}
		if f.gotURL != "https://www.adzuna.co.uk/jobs/details/1" {
			t.Errorf("fetched wrong URL: %q", f.gotURL)
		}
	})

	t.Run("no JobPosting block on the page (e.g. Access Denied)", func(t *testing.T) {
		f := &fakeHTML{body: pageWithoutJobPosting}
		_, err := FetchDescription(context.Background(), f, "https://www.adzuna.co.uk/jobs/land/ad/1")
		if err == nil {
			t.Fatal("expected an error, got none")
		}
	})

	t.Run("JobPosting block with an empty description", func(t *testing.T) {
		f := &fakeHTML{body: pageWithEmptyDescription}
		_, err := FetchDescription(context.Background(), f, "https://www.adzuna.co.uk/jobs/details/1")
		if err == nil {
			t.Fatal("expected an error, got none")
		}
	})

	t.Run("transport error surfaces unwrapped", func(t *testing.T) {
		wantErr := errors.New("boom")
		f := &fakeHTML{err: wantErr}
		_, err := FetchDescription(context.Background(), f, "https://www.adzuna.co.uk/jobs/details/1")
		if !errors.Is(err, wantErr) {
			t.Errorf("got %v, want wrapping %v", err, wantErr)
		}
	})
}
