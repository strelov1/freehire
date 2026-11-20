package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientGetTextWithHeadersLetsTheCallerChooseAccept(t *testing.T) {
	var gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAccept = r.Header.Get("Accept")
	}))
	defer srv.Close()

	c := &Client{httpClient: srv.Client(), userAgent: "freehire-test"}
	if _, err := c.GetTextWithHeaders(context.Background(), srv.URL, map[string]string{"Accept": "text/javascript"}); err != nil {
		t.Fatalf("GetTextWithHeaders: %v", err)
	}
	if gotAccept != "text/javascript" {
		t.Errorf("Accept = %q, want the caller's text/javascript", gotAccept)
	}
}
