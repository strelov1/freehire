package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
)

// jsonServer answers every request with body and records what it was asked.
func jsonServer(t *testing.T, body string, hit func(*http.Request)) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hit != nil {
			hit(r)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

type decodeOut struct {
	Name string `json:"name"`
}

// The three getters whose decode closure had no test: a 200 body must reach the caller
// decoded, through the shared decodeJSON.
func TestClientGetJSONOrNoContentDecodesBody(t *testing.T) {
	srv := jsonServer(t, `{"name":"acme","ignored":true}`, nil)
	c := &Client{httpClient: srv.Client(), userAgent: "freehire-test"}

	var out decodeOut
	found, err := c.GetJSONOrNoContent(context.Background(), srv.URL, nil, &out)
	if err != nil {
		t.Fatalf("GetJSONOrNoContent: %v", err)
	}
	if !found {
		t.Error("found = false, want true for a 200 with a body")
	}
	if out.Name != "acme" {
		t.Errorf("decoded name = %q, want %q", out.Name, "acme")
	}
}

func TestClientGetJSONResolvedDecodesBody(t *testing.T) {
	srv := jsonServer(t, `{"name":"acme"}`, nil)
	c := &Client{httpClient: srv.Client(), userAgent: "freehire-test"}

	var out decodeOut
	final, err := c.GetJSONResolved(context.Background(), srv.URL, &out)
	if err != nil {
		t.Fatalf("GetJSONResolved: %v", err)
	}
	if final != srv.URL {
		t.Errorf("final URL = %q, want %q", final, srv.URL)
	}
	if out.Name != "acme" {
		t.Errorf("decoded name = %q, want %q", out.Name, "acme")
	}
}

func TestClientPostFormJSONPostsFormAndDecodesBody(t *testing.T) {
	var gotForm url.Values
	srv := jsonServer(t, `{"name":"acme"}`, func(r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.PostForm
	})
	c := &Client{httpClient: srv.Client(), userAgent: "freehire-test"}

	var out decodeOut
	err := c.PostFormJSON(context.Background(), srv.URL, url.Values{"q": {"go"}}, &out)
	if err != nil {
		t.Fatalf("PostFormJSON: %v", err)
	}
	if gotForm.Get("q") != "go" {
		t.Errorf("posted form q = %q, want %q", gotForm.Get("q"), "go")
	}
	if out.Name != "acme" {
		t.Errorf("decoded name = %q, want %q", out.Name, "acme")
	}
}
