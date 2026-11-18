package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientGetTextResolvedReturnsTheFinalURLFromItsFetch(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			http.Redirect(w, r, "/team/", http.StatusFound)
			return
		}
		_, _ = w.Write([]byte(`<a href="careers">Careers</a>`))
	}))
	defer srv.Close()
	c := &Client{httpClient: srv.Client(), userAgent: "freehire-test"}
	body, final, err := c.GetTextResolved(context.Background(), srv.URL+"/start")
	if err != nil || final != srv.URL+"/team/" || body != `<a href="careers">Careers</a>` {
		t.Fatalf("resolved text: %s, %s, %v", body, final, err)
	}
}
