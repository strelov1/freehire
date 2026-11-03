package ga4mp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFromEnvIsInertWithoutSecret(t *testing.T) {
	env := map[string]string{"GA4_MEASUREMENT_ID": "G-X"}
	if c := FromEnv(func(k string) string { return env[k] }); c != nil {
		t.Fatalf("want nil client without a secret, got %+v", c)
	}
	var nilClient *Client
	if err := nilClient.SignUp(context.Background(), "1.2", "3", "oauth"); err != nil {
		t.Fatalf("nil client must be a no-op, got %v", err)
	}
}

func TestSignUpPostsEventWithClientAndSession(t *testing.T) {
	var got map[string]any
	var query string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		query = r.URL.RawQuery
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &got)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	c := &Client{measurementID: "G-X", apiSecret: "s", baseURL: srv.URL, http: srv.Client()}
	if err := c.SignUp(context.Background(), "123.456", "1700000000", "oauth"); err != nil {
		t.Fatal(err)
	}
	if got["client_id"] != "123.456" {
		t.Fatalf("client_id = %v", got["client_id"])
	}
	ev := got["events"].([]any)[0].(map[string]any)
	p := ev["params"].(map[string]any)
	if ev["name"] != "sign_up" || p["method"] != "oauth" || p["session_id"] != "1700000000" {
		t.Fatalf("event = %v", ev)
	}
	if query != "api_secret=s&measurement_id=G-X" {
		t.Fatalf("query = %q", query)
	}
}

func TestSignUpRejectsForgedClientID(t *testing.T) {
	c := &Client{measurementID: "G-X", apiSecret: "s", baseURL: "http://127.0.0.1:0", http: http.DefaultClient}
	for _, bad := range []string{"", "abc", "1.2.3", "1.2;drop", "123456789012345.1"} {
		if err := c.SignUp(context.Background(), bad, "", "oauth"); err == nil {
			t.Errorf("client id %q must be refused", bad)
		}
	}
}

func TestSignUpReportsNon2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	c := &Client{measurementID: "G-X", apiSecret: "s", baseURL: srv.URL, http: srv.Client()}
	if err := c.SignUp(context.Background(), "1.2", "", "oauth"); err == nil {
		t.Fatal("a 400 must surface as an error")
	}
}

func TestSignUpFailureNeverLeaksTheSecretAndIsUncertain(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing listens any more: the connection is refused

	c := &Client{measurementID: "G-X", apiSecret: "TOP-SECRET-VALUE", baseURL: url, http: http.DefaultClient}
	err := c.SignUp(context.Background(), "1.2", "", "oauth")
	if err == nil {
		t.Fatal("want an error from an unreachable collector")
	}
	if strings.Contains(err.Error(), "TOP-SECRET-VALUE") {
		t.Fatalf("error leaks the API secret: %v", err)
	}
	if !errors.Is(err, ErrUncertain) {
		t.Fatalf("a transport failure must be uncertain, got %v", err)
	}
}

func TestSignUpHTTPRefusalIsNotUncertain(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer srv.Close()
	c := &Client{measurementID: "G-X", apiSecret: "s", baseURL: srv.URL, http: srv.Client()}
	err := c.SignUp(context.Background(), "1.2", "", "oauth")
	if err == nil || errors.Is(err, ErrUncertain) {
		t.Fatalf("a 400 is a definite failure, got %v", err)
	}
}
