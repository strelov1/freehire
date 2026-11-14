package gmailsync

import (
	"context"
	"io"
	"net/http"
	"slices"
	"strings"
	"testing"
)

// pagedGmail answers the message search with two pages, keyed by the page token.
type pagedGmail struct{ tokens []string }

func (p *pagedGmail) RoundTrip(req *http.Request) (*http.Response, error) {
	token := req.URL.Query().Get("pageToken")
	p.tokens = append(p.tokens, token)
	body := `{"messages":[{"id":"a"},{"id":"b"}],"nextPageToken":"p2"}`
	if token == "p2" {
		body = `{"messages":[{"id":"c"}]}`
	}
	return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
}

func TestListATSMessageIDsFollowsEveryPage(t *testing.T) {
	gmail := &pagedGmail{}
	r := &apiReader{client: &http.Client{Transport: gmail}}
	ids, err := r.ListATSMessageIDs(context.Background(), "me@example.test", 0)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ids, []string{"a", "b", "c"}) {
		t.Fatalf("ids = %v, want [a b c]", ids)
	}
	if !slices.Equal(gmail.tokens, []string{"", "p2"}) {
		t.Fatalf("page tokens = %v, want first page then p2", gmail.tokens)
	}
}
