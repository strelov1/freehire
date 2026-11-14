package gmailsync

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type sentPageTransport struct {
	t     *testing.T
	calls int
}

func (s *sentPageTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s.calls++
	if req.URL.Query().Get("q") != "in:sent after:940" {
		s.t.Errorf("query=%q", req.URL.Query().Get("q"))
	}
	body := `{"messages":[{"id":"one","threadId":"known"}],"nextPageToken":"next"}`
	if s.calls == 2 {
		if req.URL.Query().Get("pageToken") != "next" {
			s.t.Error("missing next-page token")
		}
		body = `{"messages":[{"id":"two","threadId":"private"}]}`
	}
	return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header)}, nil
}

func TestSentDiscoveryPagesMetadataWithOverlap(t *testing.T) {
	transport := &sentPageTransport{t: t}
	reader := NewAPIReader(&http.Client{Transport: transport}, nil)
	refs, err := reader.ListSentMessages(context.Background(), 1000)
	if err != nil {
		t.Fatal(err)
	}
	if transport.calls != 2 || len(refs) != 2 || refs[0].ThreadID != "known" || refs[1].ID != "two" {
		t.Fatalf("calls=%d refs=%+v", transport.calls, refs)
	}
}
