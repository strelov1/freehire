package billing

import (
	"context"
	"fmt"
	"net/http"
	"testing"
)

func TestCheckoutAbandoned(t *testing.T) {
	for _, tt := range []struct {
		name, status, payment, subscriptions, reference string
		providerError, want                             bool
	}{
		{name: "expired unpaid", status: "expired", payment: "unpaid", reference: "7", want: true},
		{name: "open", status: "open", payment: "unpaid", reference: "7"},
		{name: "checkout complete with delayed payment", status: "complete", payment: "unpaid", reference: "7"},
		{name: "paid", status: "expired", payment: "paid", reference: "7"},
		{name: "different account", status: "expired", payment: "unpaid", reference: "8"},
		{name: "another session converted", status: "expired", payment: "unpaid", reference: "7", subscriptions: subscriptionsBody},
		{name: "provider unavailable", providerError: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := testClient(t, func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/checkout/sessions/cs_123":
					if tt.providerError {
						http.Error(w, "unavailable", http.StatusServiceUnavailable)
						return
					}
					_, _ = fmt.Fprintf(w, `{"status":%q,"payment_status":%q,"client_reference_id":%q,"customer":"cus_1"}`, tt.status, tt.payment, tt.reference)
				case "/subscriptions":
					if r.URL.Query().Get("customer") != "cus_1" {
						t.Error("reading the wrong Stripe customer")
					}
					body := tt.subscriptions
					if body == "" {
						body = `{"data":[]}`
					}
					_, _ = fmt.Fprint(w, body)
				default:
					t.Errorf("unexpected request %s", r.URL.Path)
					http.NotFound(w, r)
				}
			})
			service := &Service{cfg: Config{Prices: []string{"price_pro_monthly"}}, client: client}
			got, err := service.CheckoutAbandoned(context.Background(), 7, "cs_123")
			if got != tt.want || (err != nil) != tt.providerError {
				t.Fatalf("CheckoutAbandoned = %t, %v", got, err)
			}
		})
	}
}
