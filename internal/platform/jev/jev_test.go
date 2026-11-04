package jev

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAskSendsRequestAndDecodesAnswers(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/systemone" {
			t.Errorf("request %s %s", r.Method, r.URL.Path)
		}
		if a := r.Header.Get("Authorization"); a != "Bearer k" {
			t.Errorf("authorization = %q", a)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"answers":{"q":{"choice":"a","probabilities":{"a":0.9},"noul":0.4}}}`))
	}))
	t.Cleanup(srv.Close)

	ans, err := New("k", WithBaseURL(srv.URL)).Ask(context.Background(), map[string]string{"x": "y"},
		map[string]Question{"q": {Type: "choice", Instructions: "i", Criteria: map[string]string{"a": "A"}}})
	if err != nil {
		t.Fatalf("Ask: %v", err)
	}
	a := ans["q"]
	if a.Choice != "a" || a.Probabilities["a"] != 0.9 || a.Noul == nil || *a.Noul != 0.4 {
		t.Errorf("answer = %+v", a)
	}
	if got["model"] != Model {
		t.Errorf("model = %v", got["model"])
	}
}

func TestAskOmitsEmptyCriteria(t *testing.T) {
	var raw string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		raw = string(b)
		_, _ = w.Write([]byte(`{"answers":{}}`))
	}))
	t.Cleanup(srv.Close)
	if _, err := New("k", WithBaseURL(srv.URL)).Ask(context.Background(), nil,
		map[string]Question{"q": {Type: "noul", Instructions: "i"}}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(raw, "criteria") {
		t.Errorf("empty criteria sent: %s", raw)
	}
}

func TestAskErrors(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
		want   string
	}{
		"non-200":  {http.StatusUnauthorized, "bad key", "unexpected status 401"},
		"bad json": {http.StatusOK, "{", "decode response"},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(srv.Close)
			_, err := New("k", WithBaseURL(srv.URL)).Ask(context.Background(), nil, nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want %q", err, tc.want)
			}
		})
	}
}
