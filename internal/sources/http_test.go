package sources

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestClientGetJSONDecodesAndSendsUserAgent(t *testing.T) {
	var gotUA string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"acme"}`))
	}))
	defer srv.Close()

	c := &Client{httpClient: srv.Client(), userAgent: "freehire-test"}

	var out struct {
		Name string `json:"name"`
	}
	if err := c.GetJSON(context.Background(), srv.URL, &out); err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	if out.Name != "acme" {
		t.Errorf("decoded name = %q, want %q", out.Name, "acme")
	}
	if gotUA != "freehire-test" {
		t.Errorf("User-Agent = %q, want %q", gotUA, "freehire-test")
	}
}

func TestClientGetJSONWithHeadersSendsCustomHeaderAlongsideStandard(t *testing.T) {
	var gotKey, gotUA, gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotKey = r.Header.Get("X-Api-Key")
		gotUA = r.Header.Get("User-Agent")
		gotAccept = r.Header.Get("Accept")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := &Client{httpClient: srv.Client(), userAgent: "freehire-test"}

	var out struct {
		OK bool `json:"ok"`
	}
	err := c.GetJSONWithHeaders(context.Background(), srv.URL, map[string]string{"X-Api-Key": "secret"}, &out)
	if err != nil {
		t.Fatalf("GetJSONWithHeaders: %v", err)
	}
	if gotKey != "secret" {
		t.Errorf("X-Api-Key = %q, want secret", gotKey)
	}
	if gotUA != "freehire-test" {
		t.Errorf("User-Agent = %q, want it preserved", gotUA)
	}
	if !strings.Contains(gotAccept, "json") {
		t.Errorf("Accept = %q, want it to request json", gotAccept)
	}
}

func TestClientPostJSONWithHeadersSendsCustomHeader(t *testing.T) {
	var gotRSC string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotRSC = r.Header.Get("RSC")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := &Client{httpClient: srv.Client()}

	var out struct {
		OK bool `json:"ok"`
	}
	err := c.PostJSONWithHeaders(context.Background(), srv.URL, map[string]string{"RSC": "1"}, map[string]string{"q": "x"}, &out)
	if err != nil {
		t.Fatalf("PostJSONWithHeaders: %v", err)
	}
	if gotRSC != "1" {
		t.Errorf("RSC = %q, want 1", gotRSC)
	}
}

func TestClientPostFormWithHeadersSendsFormEncodedBodyAndParsesHTML(t *testing.T) {
	var gotContentType, gotCSRF string
	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotContentType = r.Header.Get("Content-Type")
		gotCSRF = r.Header.Get("X-Csrftoken")
		_ = r.ParseForm()
		gotForm = r.PostForm
		_, _ = w.Write([]byte(`<html><body><p>hi</p></body></html>`))
	}))
	defer srv.Close()

	c := &Client{httpClient: srv.Client()}

	values := url.Values{"page": {"2"}, "csrfmiddlewaretoken": {"tok123"}}
	node, err := c.PostFormWithHeaders(context.Background(), srv.URL, map[string]string{"X-Csrftoken": "tok123"}, values)
	if err != nil {
		t.Fatalf("PostFormWithHeaders: %v", err)
	}
	if !strings.Contains(gotContentType, "application/x-www-form-urlencoded") {
		t.Errorf("Content-Type = %q, want form-urlencoded", gotContentType)
	}
	if gotCSRF != "tok123" {
		t.Errorf("X-Csrftoken = %q, want tok123", gotCSRF)
	}
	if gotForm.Get("page") != "2" || gotForm.Get("csrfmiddlewaretoken") != "tok123" {
		t.Errorf("form body = %v, want page=2 and csrfmiddlewaretoken=tok123", gotForm)
	}
	if got := textContent(node); got != "hi" {
		t.Errorf("parsed HTML text = %q, want %q", got, "hi")
	}
}

func TestClientCookieValueReadsCookieSetByAPriorRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "csrftoken", Value: "tok-abc", Path: "/"})
		_, _ = w.Write([]byte(`<html></html>`))
	}))
	defer srv.Close()

	jar, _ := cookiejar.New(nil)
	httpClient := srv.Client()
	httpClient.Jar = jar
	c := &Client{httpClient: httpClient}

	if _, err := c.GetHTML(context.Background(), srv.URL); err != nil {
		t.Fatalf("GetHTML: %v", err)
	}
	if got := c.CookieValue(srv.URL, "csrftoken"); got != "tok-abc" {
		t.Errorf("CookieValue = %q, want %q", got, "tok-abc")
	}
	if got := c.CookieValue(srv.URL, "nope"); got != "" {
		t.Errorf("CookieValue for unset cookie = %q, want empty", got)
	}
}

func TestClientCookieValueReturnsEmptyWithoutAJar(t *testing.T) {
	c := &Client{httpClient: &http.Client{}}
	if got := c.CookieValue("https://example.com", "csrftoken"); got != "" {
		t.Errorf("CookieValue without a jar = %q, want empty", got)
	}
}

func TestClientGetXMLDecodesAndSendsXMLAccept(t *testing.T) {
	var gotAccept string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAccept = r.Header.Get("Accept")
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<root><name>acme</name></root>`))
	}))
	defer srv.Close()

	c := &Client{httpClient: srv.Client()}

	var out struct {
		Name string `xml:"name"`
	}
	if err := c.GetXML(context.Background(), srv.URL, &out); err != nil {
		t.Fatalf("GetXML: %v", err)
	}
	if out.Name != "acme" {
		t.Errorf("decoded name = %q, want %q", out.Name, "acme")
	}
	if !strings.Contains(gotAccept, "xml") {
		t.Errorf("Accept = %q, want it to request xml", gotAccept)
	}
}

func TestClientGetJSONRetriesOnServerError(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := &Client{httpClient: srv.Client(), maxRetries: 2}

	var out struct {
		OK bool `json:"ok"`
	}
	if err := c.GetJSON(context.Background(), srv.URL, &out); err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2", attempts)
	}
	if !out.OK {
		t.Error("expected ok=true after retry")
	}
}

func TestClientGetJSONErrorsOnClientError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	c := &Client{httpClient: srv.Client()}

	var out map[string]any
	if err := c.GetJSON(context.Background(), srv.URL, &out); err == nil {
		t.Error("expected error on 404, got nil")
	}
}

func TestClientGetHTMLResolvedFollowsRedirectAndReturnsFinalURL(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/short", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/vacancies/1000166712", http.StatusMovedPermanently)
	})
	mux.HandleFunc("/vacancies/1000166712", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte(`<html><body><h1>Product manager</h1></body></html>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := &Client{httpClient: srv.Client()}

	node, final, err := c.GetHTMLResolved(context.Background(), srv.URL+"/short")
	if err != nil {
		t.Fatalf("GetHTMLResolved: %v", err)
	}
	if node == nil {
		t.Fatal("node is nil, want a parsed tree")
	}
	if !strings.HasSuffix(final, "/vacancies/1000166712") {
		t.Errorf("final URL = %q, want it to end at the redirect target", final)
	}
	if got := textContent(node); !strings.Contains(got, "Product manager") {
		t.Errorf("parsed text = %q, want the destination page body", got)
	}
}

func TestClientRetriesOn429ThenSucceeds(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 2 {
			w.Header().Set("Retry-After", "0") // ask for an immediate retry
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := &Client{httpClient: srv.Client(), maxRetries: 2}

	var out struct {
		OK bool `json:"ok"`
	}
	if err := c.GetJSON(context.Background(), srv.URL, &out); err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	if !out.OK {
		t.Error("expected ok=true after a 429 retry")
	}
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2 (429 then 200)", attempts)
	}
}

func TestClientGetHTMLSurfacesWAFChallengeAsTypedError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// AWS-WAF Challenge action: a 202 carrying the challenge marker header and a
		// tiny challenge shell (never the real posting).
		w.Header().Set("x-amzn-waf-action", "challenge")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`<html><head><script>window.gokuProps={}</script></head><body></body></html>`))
	}))
	defer srv.Close()

	c := &Client{httpClient: srv.Client(), maxRetries: 2}

	node, err := c.GetHTML(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected a challenge error, got nil")
	}
	var chErr *ChallengeError
	if !errors.As(err, &chErr) {
		t.Fatalf("error = %v, want a *ChallengeError", err)
	}
	if node != nil {
		t.Error("expected nil node: the challenge shell must not be decoded as content")
	}
}

func TestClientDoesNotRetryWAFChallenge(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("x-amzn-waf-action", "challenge")
		w.WriteHeader(http.StatusAccepted)
	}))
	defer srv.Close()

	c := &Client{httpClient: srv.Client(), maxRetries: 2}

	if _, err := c.GetHTML(context.Background(), srv.URL); err == nil {
		t.Fatal("expected a challenge error, got nil")
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 (a challenge is not a transient failure)", attempts)
	}
}

// A body past the size cap must name itself. Capping with a bare io.LimitReader hands the
// decoder a truncated stream, which is indistinguishable from a dropped connection — that
// is how a permanently oversized Greenhouse board (Anduril, 34.6 MiB against the old
// 32 MiB cap) spent weeks reporting "unexpected EOF" and cooling down as if transient.
func TestClientGetJSONSurfacesOversizedBodyAsTypedError(t *testing.T) {
	var attempts int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"content":%q}`, strings.Repeat("x", 4096))
	}))
	defer srv.Close()

	c := &Client{httpClient: srv.Client(), maxRetries: 2, maxBody: 512}

	var out struct {
		Content string `json:"content"`
	}
	err := c.GetJSON(context.Background(), srv.URL, &out)
	var tooLarge *BodyTooLargeError
	if !errors.As(err, &tooLarge) {
		t.Fatalf("GetJSON error = %v, want a *BodyTooLargeError", err)
	}
	if tooLarge.Limit != 512 {
		t.Errorf("reported limit = %d, want 512", tooLarge.Limit)
	}
	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 (an oversized body will not shrink on a retry)", attempts)
	}
}

// The cap is inclusive: a body of exactly maxBody bytes is complete, not truncated.
func TestClientGetJSONAcceptsBodyExactlyAtCap(t *testing.T) {
	body := `{"content":"acme"}`
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := &Client{httpClient: srv.Client(), maxBody: int64(len(body))}

	var out struct {
		Content string `json:"content"`
	}
	if err := c.GetJSON(context.Background(), srv.URL, &out); err != nil {
		t.Fatalf("GetJSON: %v", err)
	}
	if out.Content != "acme" {
		t.Errorf("decoded content = %q, want %q", out.Content, "acme")
	}
}
