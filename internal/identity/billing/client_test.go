package billing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// The provider's subscription listing, trimmed to what we read.
const subscriptionsBody = `{
  "object": "list",
  "data": [
    {
      "id": "sub_1",
      "status": "active",
      "cancel_at": null,
      "items": { "data": [ { "current_period_end": 1790812800, "price": { "id": "price_pro_monthly" } } ] }
    }
  ]
}`

func testClient(t *testing.T, h http.HandlerFunc) *client {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	// The server's own client, not safehttp's: safehttp refuses private addresses, which is
	// exactly right in production and exactly wrong for a loopback test server.
	return newClient("sk_test", srv.URL, srv.Client())
}

func TestClientSubscriberState(t *testing.T) {
	var gotPath, gotQuery, gotAuth string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery, gotAuth = r.URL.Path, r.URL.RawQuery, r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(subscriptionsBody))
	})

	sub, err := c.subscriberState(context.Background(), "cus_9")
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}

	if gotPath != "/subscriptions" {
		t.Errorf("path: want /subscriptions, got %q", gotPath)
	}
	if want := "Bearer sk_test"; gotAuth != want {
		t.Errorf("authorization: want %q, got %q", want, gotAuth)
	}
	// status=all on purpose: filtering server-side would split the entitling rule between a
	// query parameter and entitlingStatuses.
	for _, want := range []string{"customer=cus_9", "status=all", "expand"} {
		if !strings.Contains(gotQuery, want) {
			t.Errorf("query %q is missing %q", gotQuery, want)
		}
	}

	if len(sub.Subscriptions) != 1 {
		t.Fatalf("subscriptions not read: %+v", sub.Subscriptions)
	}
	s := sub.Subscriptions[0]
	if s.Status != "active" {
		t.Errorf("status: got %q", s.Status)
	}
	// Read from the ITEM. The provider moved it there; a client reading only the top level
	// gets zero, and a zero period end is what an earlier fallback turned into "forever".
	if got := s.CurrentPeriodEnd.Format(time.RFC3339); got != "2026-10-01T00:00:00Z" {
		t.Errorf("current_period_end decoded to %s", got)
	}
	// Without expanding the price the items carry a bare id string and there is nothing to
	// match a configured price against.
	if len(s.PriceIDs) != 1 || s.PriceIDs[0] != "price_pro_monthly" {
		t.Errorf("prices not read: %v", s.PriceIDs)
	}
}

// TestClientTreatsNoSubscriptionsAsFree is the case every account that cancelled falls into.
// An empty list is an answer, not a failure.
func TestClientTreatsNoSubscriptionsAsFree(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"object":"list","data":[]}`))
	})

	sub, err := c.subscriberState(context.Background(), "cus_9")
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if until := proUntilFrom(sub, []string{"price_pro_monthly"}); !until.IsZero() {
		t.Fatalf("no subscriptions must derive to the free plan, got %s", until)
	}
}

func TestClientErrors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
	}{
		{name: "unauthorised", status: http.StatusUnauthorized, body: `{"error":{"message":"invalid api key"}}`},
		{name: "provider error", status: http.StatusInternalServerError, body: `{"error":{"message":"boom"}}`},
		{name: "rate limited", status: http.StatusTooManyRequests, body: `{"error":{"message":"slow down"}}`},
		{name: "not JSON", status: http.StatusOK, body: `<html>gateway</html>`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			})
			if _, err := c.subscriberState(context.Background(), "cus_9"); err == nil {
				t.Fatal("want an error, got nil")
			}
		})
	}
}

// TestClientCheckoutSessionCarriesTheAccount guards the two places our account id travels.
// One covers the first purchase, before any customer binding exists; the other covers every
// renewal after it. Losing either loses the ability to say whose payment this was.
func TestClientCheckoutSession(t *testing.T) {
	var gotForm string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.Form.Encode()
		_, _ = w.Write([]byte(`{"url":"https://checkout.stripe.com/c/pay/cs_test_123"}`))
	})

	url, err := c.createCheckoutSession(context.Background(), 601, "buyer@example.com",
		"price_pro_monthly", "https://freehire.me/my/plan", "https://freehire.me/my/plan", "", "")
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if url != "https://checkout.stripe.com/c/pay/cs_test_123" {
		t.Fatalf("url: got %q", url)
	}
	for _, want := range []string{"client_reference_id=601", "freehire_user_id", "mode=subscription", "price_pro_monthly"} {
		if !strings.Contains(gotForm, want) {
			t.Errorf("form %q is missing %q", gotForm, want)
		}
	}
}

// TestClientCheckoutReusesAKnownCustomer: a second purchase must not create a second
// customer for one person, which would leave two subscriptions nobody sums.
func TestClientCheckoutReusesAKnownCustomer(t *testing.T) {
	var gotForm string
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		gotForm = r.Form.Encode()
		_, _ = w.Write([]byte(`{"url":"https://checkout.stripe.com/c/pay/cs_test_456"}`))
	})

	if _, err := c.createCheckoutSession(context.Background(), 601, "buyer@example.com",
		"price_pro_monthly", "https://freehire.me/my/plan", "https://freehire.me/my/plan", "cus_9", ""); err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if !strings.Contains(gotForm, "customer=cus_9") {
		t.Errorf("form %q does not reuse the known customer", gotForm)
	}
	if strings.Contains(gotForm, "customer_creation") {
		t.Errorf("form %q asks for a new customer despite holding one", gotForm)
	}
}

// TestClientCheckoutNeverSendsCustomerCreation guards a parameter the provider refuses in
// subscription mode. The refusal is invisible from a candidate's side: the session is never
// created, the handler answers 404, and the upgrade button hides itself as though billing
// were switched off — which is exactly how it reached production unnoticed.
func TestClientCheckoutNeverSendsCustomerCreation(t *testing.T) {
	for _, existing := range []string{"", "cus_9"} {
		var gotForm string
		c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
			_ = r.ParseForm()
			gotForm = r.Form.Encode()
			_, _ = w.Write([]byte(`{"url":"https://checkout.stripe.com/c/pay/cs_test"}`))
		})

		if _, err := c.createCheckoutSession(context.Background(), 601, "buyer@example.com",
			"price_pro_monthly", "https://freehire.me/my/plan", "https://freehire.me/my/plan", existing, ""); err != nil {
			t.Fatalf("want no error, got %v", err)
		}
		if strings.Contains(gotForm, "customer_creation") {
			t.Fatalf("form %q sends customer_creation, which subscription mode refuses", gotForm)
		}
	}
}

func TestClientPortalSession(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"url":"https://billing.stripe.com/p/session/test_123"}`))
	})

	url, err := c.createPortalSession(context.Background(), "cus_9", "https://freehire.me/my/plan")
	if err != nil {
		t.Fatalf("want no error, got %v", err)
	}
	if url != "https://billing.stripe.com/p/session/test_123" {
		t.Fatalf("url: got %q", url)
	}
}

// TestClientHonoursContext matters because these calls sit inside a webhook handler with a
// budget at the far end. A provider that hangs must fail the apply, not the delivery.
func TestClientHonoursContext(t *testing.T) {
	c := testClient(t, func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	if _, err := c.subscriberState(ctx, "cus_9"); err == nil {
		t.Fatal("want an error when the context expires, got nil")
	}
}

// TestClientReadsThePeriodEndFromEitherPlace guards the migration that caused the bug. An
// account pinned to an older API version still sends the field at the subscription level;
// a current one sends it on the item. Reading only one place produces a subscription with no
// end, which entitles nobody — a paying customer silently reading as free.
func TestClientReadsThePeriodEndFromEitherPlace(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{
			name: "current API: on the item",
			body: `{"data":[{"status":"active","items":{"data":[{"current_period_end":1790812800,"price":{"id":"price_pro_monthly"}}]}}]}`,
		},
		{
			name: "older API: on the subscription",
			body: `{"data":[{"status":"active","current_period_end":1790812800,"items":{"data":[{"price":{"id":"price_pro_monthly"}}]}}]}`,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := testClient(t, func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(tc.body))
			})
			sub, err := c.subscriberState(context.Background(), "cus_9")
			if err != nil {
				t.Fatalf("want no error, got %v", err)
			}
			until := proUntilFrom(sub, []string{"price_pro_monthly"})
			if got := until.Format(time.RFC3339); got != "2026-10-01T00:00:00Z" {
				t.Fatalf("want 2026-10-01T00:00:00Z, got %s", got)
			}
		})
	}
}
