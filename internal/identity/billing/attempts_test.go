package billing

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

func TestPaymentAttemptsKeepEveryChargeAndHideProviderDetails(t *testing.T) {
	s := serviceWithProvider(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/charges" || r.URL.Query().Get("customer") != "cus_owned" || r.URL.Query().Get("limit") != "20" {
			t.Errorf("unexpected request: %s", r.URL)
		}
		_, _ = w.Write([]byte(`{"data":[
		{"id":"ch_paid","created":200,"status":"succeeded","amount":290,"currency":"brl","payment_intent":"pi_same","payment_method_details":{"card":{"brand":"mastercard","last4":"4242","funding":"credit","wallet":{"type":"apple_pay"}}}},
		{"id":"ch_retry","created":100,"status":"failed","amount":290,"currency":"brl","payment_intent":"pi_same","failure_code":"card_declined","outcome":{"type":"issuer_declined","reason":"insufficient_funds","risk_level":"normal","seller_message":"private diagnostic"}},
		{"id":"ch_blocked","status":"failed","outcome":{"type":"blocked","reason":"highest_risk_level","risk_level":"highest"}},
		{"id":"ch_unknown","status":"failed","failure_message":"private diagnostic","outcome":{"reason":"new_provider_reason"}},
		{"id":"ch_pending","status":"pending"},
		{"id":"ch_refunded","status":"succeeded","amount":290,"amount_refunded":290,"refunded":true},
		{"id":"ch_partial","status":"succeeded","amount":290,"amount_refunded":100}
	]}`))
	})
	out, err := s.client.paymentAttempts(context.Background(), "cus_owned")
	if err != nil {
		t.Fatal(err)
	}
	if len(out) != 7 || out[0].ID != "ch_paid" || out[1].ID != "ch_retry" {
		t.Fatalf("attempts lost or reordered: %+v", out)
	}
	if out[0].CardLast4 != "4242" || out[0].CardFunding != "credit" || out[0].Wallet != "apple_pay" {
		t.Fatalf("missing masked card details: %+v", out[0])
	}
	wantStatuses := []string{"succeeded", "failed", "failed", "failed", "pending", "refunded", "partially_refunded"}
	wantReasons := []string{"", "insufficient_funds", "security_check", "declined", "", "", ""}
	for i := range out {
		if out[i].Status != wantStatuses[i] || out[i].Reason != wantReasons[i] {
			t.Errorf("attempt %d: %+v", i, out[i])
		}
	}
	wire, err := json.Marshal(out)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"highest", "private diagnostic", "new_provider_reason", "pi_same", "risk_level"} {
		if strings.Contains(string(wire), secret) {
			t.Errorf("exposed %q: %s", secret, wire)
		}
	}
}

func TestPaymentAttemptsFailExplicitlyWhenProviderUnavailable(t *testing.T) {
	s := serviceWithProvider(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	if _, err := s.client.paymentAttempts(context.Background(), "cus_owned"); err == nil {
		t.Fatal("provider failure must not look like an empty history")
	}
}
