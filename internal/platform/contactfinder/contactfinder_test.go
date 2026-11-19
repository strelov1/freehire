package contactfinder

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func fake(t *testing.T, handle func(path string, body map[string]any) (int, string)) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		status, out := handle(r.URL.Path, body)
		w.WriteHeader(status)
		_, _ = w.Write([]byte(out))
	}))
	t.Cleanup(srv.Close)
	return New(srv.URL, "tok")
}

func TestFind(t *testing.T) {
	c := fake(t, func(path string, body map[string]any) (int, string) {
		if path != "/v1/find" || body["domain"] != "acme.com" || body["user_ref"] != "42" {
			return http.StatusBadRequest, ""
		}
		return http.StatusOK, `{"contacts":[{"name":"Anna","title":"Recruiter","email":"anna@acme.com","confidence":"high"}]}`
	})
	got, err := c.Find(context.Background(), "acme.com", "42")
	if err != nil || len(got) != 1 || got[0].Email != "anna@acme.com" || got[0].Confidence != "high" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestFindEmptyIsNotNil(t *testing.T) {
	c := fake(t, func(string, map[string]any) (int, string) { return http.StatusOK, `{"contacts":[]}` })
	got, err := c.Find(context.Background(), "acme.com", "1")
	if err != nil || got == nil || len(got) != 0 {
		t.Fatalf("got %#v, %v", got, err)
	}
}

func TestFailuresAreUnavailable(t *testing.T) {
	for name, c := range map[string]*Client{
		"503":       fake(t, func(string, map[string]any) (int, string) { return http.StatusServiceUnavailable, "" }),
		"garbage":   fake(t, func(string, map[string]any) (int, string) { return http.StatusOK, "not json" }),
		"bad token": New(fake(t, nil).url, "wrong"),
		"down":      New("http://127.0.0.1:1", "tok"),
	} {
		if _, err := c.Find(context.Background(), "acme.com", "1"); !errors.Is(err, ErrUnavailable) {
			t.Errorf("%s: err = %v, want ErrUnavailable", name, err)
		}
	}
}

func TestSuppressed(t *testing.T) {
	c := fake(t, func(path string, body map[string]any) (int, string) {
		if path != "/v1/check" || len(body["emails"].([]any)) != 2 {
			return http.StatusBadRequest, ""
		}
		return http.StatusOK, `{"suppressed":["b@acme.com"]}`
	})
	got, err := c.Suppressed(context.Background(), []string{"a@acme.com", "b@acme.com"})
	if err != nil || !got["b@acme.com"] || got["a@acme.com"] {
		t.Fatalf("got %v, %v", got, err)
	}
}
