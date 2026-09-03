package billing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// The provider's own example shape, trimmed to what we read.
const subscriberBody = `{
  "request_date": "2026-09-03T12:00:00Z",
  "subscriber": {
    "management_url": "https://pay.rev.cat/manage/601",
    "entitlements": {
      "pro": {
        "expires_date": "2026-10-01T00:00:00Z",
        "grace_period_expires_date": null,
        "product_identifier": "pro_monthly",
        "purchase_date": "2026-09-01T00:00:00Z"
      }
    },
    "subscriptions": {}
  }
}`

func testClient(t *testing.T, h http.HandlerFunc) *client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	// The server's own client, not safehttp's: safehttp refuses private addresses, which
	// is exactly right in production and exactly wrong for a loopback test server.
	return newClient("sk_test", srv.URL, srv.Client())
}

func TestClientSubscriberState(t *testing.T) {
	var gotPath, gotAuth string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.EscapedPath(), r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(subscriberBody))
	})

	sub, err := c.subscriberState(context.Background(), "601")
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	if want := "/subscribers/601"; gotPath != want {
		t.Errorf("path: want %q, got %q", want, gotPath)
	}
	// A secret key, as a bearer token. A public key is refused by the provider on this
	// endpoint by design, so getting this wrong looks like an outage rather than a typo.
	if want := "Bearer sk_test"; gotAuth != want {
		t.Errorf("authorization: want %q, got %q", want, gotAuth)
	}
	if sub.ManagementURL != "https://pay.rev.cat/manage/601" {
		t.Errorf("management_url not read: %q", sub.ManagementURL)
	}
	ent, ok := sub.Entitlements["pro"]
	if !ok {
		t.Fatalf("entitlements not read: %+v", sub.Entitlements)
	}
	if ent.ExpiresDate == nil || ent.ExpiresDate.Format(time.RFC3339) != "2026-10-01T00:00:00Z" {
		t.Errorf("expires_date not read: %+v", ent.ExpiresDate)
	}
	if ent.GracePeriodExpiresDate != nil {
		t.Errorf("a null grace period must stay nil, got %v", ent.GracePeriodExpiresDate)
	}
}

// TestClientEscapesTheIdentifier guards the path segment. The identifiers we send are
// integers, but the provider's namespace is arbitrary strings and a transfer can hand us
// one — an unescaped slash would silently address a different endpoint.
func TestClientEscapesTheIdentifier(t *testing.T) {
	var gotPath string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		_, _ = w.Write([]byte(`{"subscriber":{"entitlements":{}}}`))
	})

	if _, err := c.subscriberState(context.Background(), "a/b"); err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if want := "/subscribers/a%2Fb"; gotPath != want {
		t.Fatalf("path: want %q, got %q", want, gotPath)
	}
}

func TestClientErrors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{name: "unauthorised", status: http.StatusUnauthorized, body: `{"message":"invalid api key"}`},
		{name: "provider error", status: http.StatusInternalServerError, body: `{"message":"boom"}`},
		{name: "rate limited", status: http.StatusTooManyRequests, body: `{"message":"slow down"}`},
		{name: "not JSON", status: http.StatusOK, body: `<html>gateway</html>`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			if _, err := c.subscriberState(context.Background(), "601"); err == nil {
				t.Fatal("want an error, got nil")
			}
		})
	}
}

// TestClientHonoursContext matters because this call sits inside a webhook handler with a
// 60-second budget at the far end. A provider that hangs must fail the apply, not the
// delivery.
func TestClientHonoursContext(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if _, err := c.subscriberState(ctx, "601"); err == nil {
		t.Fatal("want an error when the context expires, got nil")
	}
}
