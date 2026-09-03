package billing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// The v2 customer object, trimmed to what we read.
const customerBody = `{
  "object": "customer",
  "id": "601",
  "project_id": "proj_test",
  "active_entitlements": {
    "object": "list",
    "items": [
      { "object": "customer.active_entitlement", "entitlement_id": "freehire Pro", "expires_at": 1790812800000 }
    ],
    "next_page": null
  }
}`

func testClient(t *testing.T, h http.HandlerFunc) *client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	// The server's own client, not safehttp's: safehttp refuses private addresses, which
	// is exactly right in production and exactly wrong for a loopback test server.
	return newClient("sk_test", testProjectID, srv.URL, srv.Client())
}

func TestClientSubscriberState(t *testing.T) {
	var gotPath, gotAuth string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.EscapedPath(), r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(customerBody))
	})

	sub, err := c.subscriberState(context.Background(), "601")
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	// v2 addresses a customer inside a project; v1 addressed a subscriber globally.
	if want := "/projects/proj_test/customers/601"; gotPath != want {
		t.Errorf("path: want %q, got %q", want, gotPath)
	}
	// A secret key, as a bearer token. A public key is refused by the provider on this
	// endpoint by design, so getting this wrong looks like an outage rather than a typo.
	if want := "Bearer sk_test"; gotAuth != want {
		t.Errorf("authorization: want %q, got %q", want, gotAuth)
	}

	items := sub.ActiveEntitlements.Items
	if len(items) != 1 {
		t.Fatalf("entitlements not read: %+v", items)
	}
	if items[0].EntitlementID != "freehire Pro" {
		t.Errorf("entitlement_id not read: %q", items[0].EntitlementID)
	}
	if items[0].ExpiresAt == nil {
		t.Fatal("expires_at not read")
	}
	if got := time.UnixMilli(*items[0].ExpiresAt).UTC().Format(time.RFC3339); got != "2026-10-01T00:00:00Z" {
		t.Errorf("expires_at decoded to %s", got)
	}
}

// TestClientTreatsAnUnknownCustomerAsFree is the case every account that never bought
// anything falls into. A 404 is an answer — "no purchases" — not a failure; treating it as
// one would leave events unprocessed forever for every identifier that was never ours.
func TestClientTreatsAnUnknownCustomerAsFree(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"Customer not found"}`))
	})

	sub, err := c.subscriberState(context.Background(), "601")
	if err != nil {
		t.Fatalf("a 404 must not be an error, got %v", err)
	}
	if len(sub.ActiveEntitlements.Items) != 0 {
		t.Fatalf("want no entitlements, got %+v", sub.ActiveEntitlements.Items)
	}
	if until := proUntilFrom(sub, []string{"freehire Pro"}); !until.IsZero() {
		t.Fatalf("an unknown customer must derive to the free plan, got %s", until)
	}
}

// TestClientEscapesTheIdentifier guards the path segment. The identifiers we send are
// integers, but the provider's namespace is arbitrary strings and a transfer can hand us
// one — an unescaped slash would silently address a different endpoint.
func TestClientEscapesTheIdentifier(t *testing.T) {
	var gotPath string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		_, _ = w.Write([]byte(`{"active_entitlements":{"items":[]}}`))
	})

	if _, err := c.subscriberState(context.Background(), "a/b"); err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if want := "/projects/proj_test/customers/a%2Fb"; gotPath != want {
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
		// The failure this package shipped with: a v2 secret key against a v1 endpoint.
		// It must surface as an error rather than as an empty customer, or every
		// subscriber would quietly read as free.
		{name: "wrong API version", status: http.StatusForbidden, body: `{"code":7723,"message":"incompatible with RevenueCat API V1"}`},
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
