package linkedinauth

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// pinClock freezes the clock this package anchors expiry against, and restores it after. Every
// value Exchange and Refresh produce is a deadline computed from it.
func pinClock(t *testing.T, at time.Time) {
	t.Helper()
	previous := now
	now = func() time.Time { return at }
	t.Cleanup(func() { now = previous })
}

// tokenServerAt points the package's token endpoint at a stub. The endpoint is a constant in
// production, so this is the only way to assert what is actually sent — and what is sent is
// form-encoded, where a wrong grant_type looks identical to a right one until LinkedIn answers.
func tokenServerAt(t *testing.T, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(handler)
	previous := tokenEndpoint
	tokenEndpoint = srv.URL
	t.Cleanup(func() {
		tokenEndpoint = previous
		srv.Close()
	})
	return srv
}

func testCredentials() Credentials {
	return Credentials{
		ClientID:     "client",
		ClientSecret: "secret",
		RedirectURI:  "https://freehire.me/oauth/linkedin",
	}
}

func TestAuthorizeURLCarriesWhatLinkedInRequires(t *testing.T) {
	raw := testCredentials().AuthorizeURL("state-1")
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()

	for param, want := range map[string]string{
		"response_type": "code",
		"client_id":     "client",
		"redirect_uri":  "https://freehire.me/oauth/linkedin",
		"scope":         Scope,
		"state":         "state-1",
	} {
		if q.Get(param) != want {
			t.Errorf("%s = %q, want %q", param, q.Get(param), want)
		}
	}
}

// The one question this whole feature's design turns on: did LinkedIn issue a refresh token?
// Both answers must survive the parse, because both are ordinary.
func TestExchangeReadsBothGrantShapes(t *testing.T) {
	issued := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)

	t.Run("with a refresh token", func(t *testing.T) {
		pinClock(t, issued)
		tokenServerAt(t, func(w http.ResponseWriter, r *http.Request) {
			if err := r.ParseForm(); err != nil {
				t.Errorf("unreadable form: %v", err)
			}
			if got := r.Form.Get("grant_type"); got != "authorization_code" {
				t.Errorf("grant_type = %q", got)
			}
			if got := r.Form.Get("code"); got != "the-code" {
				t.Errorf("code = %q", got)
			}
			// LinkedIn compares the redirect_uri across both legs and refuses a mismatch.
			if got := r.Form.Get("redirect_uri"); got != "https://freehire.me/oauth/linkedin" {
				t.Errorf("redirect_uri = %q", got)
			}
			writeBody(t, w, `{"access_token":"AQV","expires_in":5184000,`+
				`"refresh_token":"AQW","refresh_token_expires_in":31536000,"scope":"w_organization_social"}`)
		})

		tok, err := testCredentials().Exchange(context.Background(), nil, "the-code")
		if err != nil {
			t.Fatal(err)
		}
		if tok.AccessToken != "AQV" || !tok.Renewable() {
			t.Fatalf("token = %+v", tok)
		}
		if want := issued.Add(5184000 * time.Second); !tok.ExpiresAt.Equal(want) {
			t.Errorf("ExpiresAt = %s, want %s", tok.ExpiresAt, want)
		}
		if want := issued.Add(31536000 * time.Second); !tok.RefreshExpiresAt.Equal(want) {
			t.Errorf("RefreshExpiresAt = %s, want %s", tok.RefreshExpiresAt, want)
		}
	})

	// The expected case outside the Marketing Developer Platform. An absent refresh token must
	// read as "cannot renew" rather than as a parse failure, or the sign-in that produced a
	// perfectly usable 60-day token would look broken.
	t.Run("without a refresh token", func(t *testing.T) {
		pinClock(t, issued)
		tokenServerAt(t, func(w http.ResponseWriter, _ *http.Request) {
			writeBody(t, w, `{"access_token":"AQV","expires_in":5184000,"scope":"w_organization_social"}`)
		})

		tok, err := testCredentials().Exchange(context.Background(), nil, "the-code")
		if err != nil {
			t.Fatal(err)
		}
		if tok.Renewable() {
			t.Error("a token with no refresh_token reported itself renewable")
		}
		// Zero, not year 1 plus a duration: the renewal worker reads a zero here as "there is
		// no grant expiry to warn about", and any other value would warn about a date in the past.
		if !tok.RefreshExpiresAt.IsZero() {
			t.Errorf("RefreshExpiresAt = %s, want the zero time", tok.RefreshExpiresAt)
		}
	})
}

func TestRefreshSendsTheRefreshGrant(t *testing.T) {
	pinClock(t, time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC))
	tokenServerAt(t, func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Errorf("unreadable form: %v", err)
		}
		if got := r.Form.Get("grant_type"); got != "refresh_token" {
			t.Errorf("grant_type = %q", got)
		}
		if got := r.Form.Get("refresh_token"); got != "AQW" {
			t.Errorf("refresh_token = %q", got)
		}
		writeBody(t, w, `{"access_token":"AQV2","expires_in":5184000,"refresh_token":"AQW","refresh_token_expires_in":26000000}`)
	})

	tok, err := testCredentials().Refresh(context.Background(), nil, "AQW")
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "AQV2" {
		t.Errorf("access token = %q", tok.AccessToken)
	}
}

// LinkedIn describes what it disliked in the body, and the two failures this flow actually
// produces — an expired code and a mismatched redirect — are indistinguishable from the status.
func TestExchangeReportsTheOAuthErrorBody(t *testing.T) {
	tokenServerAt(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		writeBody(t, w, `{"error":"invalid_request","error_description":"authorization code expired"}`)
	})

	_, err := testCredentials().Exchange(context.Background(), nil, "stale")
	if err == nil {
		t.Fatal("a rejected exchange was reported as success")
	}
	if !strings.Contains(err.Error(), "authorization code expired") {
		t.Errorf("error does not carry the description: %v", err)
	}
}

// A 200 carrying no token is not a token. Storing it would produce a credential that fails much
// later, at publish time, as a 401 nobody can trace back to the exchange.
func TestExchangeRefusesAnEmptySuccess(t *testing.T) {
	tokenServerAt(t, func(w http.ResponseWriter, _ *http.Request) {
		writeBody(t, w, `{"expires_in":5184000}`)
	})

	if _, err := testCredentials().Exchange(context.Background(), nil, "code"); err == nil {
		t.Fatal("an empty 200 was accepted as a credential")
	}
}

// LinkedIn answers some failures with an HTML error page. The error must still say what came
// back, or an operator sees "unexpected end of JSON input" and learns nothing.
func TestExchangeReportsANonJSONBody(t *testing.T) {
	tokenServerAt(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		writeBody(t, w, "<html>upstream is having a moment</html>")
	})

	_, err := testCredentials().Exchange(context.Background(), nil, "code")
	if err == nil {
		t.Fatal("a 502 HTML page was accepted")
	}
	if !strings.Contains(err.Error(), "upstream is having a moment") {
		t.Errorf("error hides the body: %v", err)
	}
}

func TestConfiguredNeedsEveryValue(t *testing.T) {
	if !testCredentials().Configured() {
		t.Error("a complete set reported itself unconfigured")
	}
	for _, drop := range []func(*Credentials){
		func(c *Credentials) { c.ClientID = "" },
		func(c *Credentials) { c.ClientSecret = "" },
		func(c *Credentials) { c.RedirectURI = "" },
	} {
		c := testCredentials()
		drop(&c)
		if c.Configured() {
			t.Errorf("a partial set reported itself configured: %+v", c)
		}
	}
}

// writeBody writes a stub response. A helper rather than fmt.Fprint at each call site because
// the write's error has to go somewhere, and a handler that silently fails to answer would
// surface as a confusing client-side error rather than as this test failing.
func writeBody(t *testing.T, w http.ResponseWriter, body string) {
	t.Helper()
	if _, err := io.WriteString(w, body); err != nil {
		t.Errorf("write stub response: %v", err)
	}
}
