//go:build integration

// Integration tests for placing referral credit on an account, against a real Postgres and
// a stubbed provider. The property under test is the one that cannot be checked without the
// database: the customer binding is write-once, so crediting an account that has never
// bought creates a customer, and crediting one that already has must not create a second.
//
// Run with: go test -tags=integration ./internal/identity/billing/
package billing

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// creditStub answers the three calls CreditAccount makes, and counts the one that matters.
func creditStub(customers *int, credits *int, body *string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasPrefix(r.URL.Path, "/prices/"):
			_, _ = w.Write([]byte(`{"id":"` + proPrice + `","unit_amount":500,"currency":"usd",` +
				`"recurring":{"interval":"month"}}`))
		case r.URL.Path == "/customers":
			*customers++
			_, _ = w.Write([]byte(`{"id":"cus_created"}`))
		case strings.HasSuffix(r.URL.Path, "/balance_transactions"):
			*credits++
			buf := make([]byte, 1024)
			n, _ := r.Body.Read(buf)
			*body = string(buf[:n])
			_, _ = w.Write([]byte(`{"id":"cbtxn_1"}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}
}

func TestCreditCreatesACustomerForSomebodyWhoHasNeverBought(t *testing.T) {
	var customers, credits int
	var body string
	s, pool := newService(t, creditStub(&customers, &credits, &body))
	ctx := context.Background()
	userID := insertUser(t, pool, "earner@example.com")

	if err := s.CreditAccount(ctx, userID, 250, "invite_reward_1"); err != nil {
		t.Fatalf("CreditAccount: %v", err)
	}
	if customers != 1 {
		t.Fatalf("customers created = %d, want 1 — a balance credit needs a customer to sit "+
			"on, and holding the reward until their own checkout meant marking credit "+
			"consumed by a session that is abandoned more often than completed", customers)
	}
	if credits != 1 {
		t.Fatalf("credits placed = %d, want 1", credits)
	}
	if !strings.Contains(body, "amount=-250") {
		t.Fatalf("body = %q, want amount=-250 — a positive amount is a DEBT the customer "+
			"owes us", body)
	}

	var bound string
	if err := pool.QueryRow(ctx,
		`SELECT stripe_customer_id FROM users WHERE id = $1`, userID).Scan(&bound); err != nil {
		t.Fatalf("reading the binding: %v", err)
	}
	if bound != "cus_created" {
		t.Fatalf("binding = %q, want cus_created", bound)
	}
}

func TestCreditReusesAnExistingCustomer(t *testing.T) {
	var customers, credits int
	var body string
	s, pool := newService(t, creditStub(&customers, &credits, &body))
	ctx := context.Background()
	userID := insertUser(t, pool, "subscriber@example.com")

	if _, err := pool.Exec(ctx,
		`UPDATE users SET stripe_customer_id = 'cus_existing' WHERE id = $1`, userID); err != nil {
		t.Fatalf("binding the customer: %v", err)
	}

	if err := s.CreditAccount(ctx, userID, 250, "invite_reward_2"); err != nil {
		t.Fatalf("CreditAccount: %v", err)
	}
	if customers != 0 {
		t.Fatalf("customers created = %d, want 0 — the binding is write-once, so a second "+
			"customer would be one nothing ever reads again", customers)
	}

	var bound string
	if err := pool.QueryRow(ctx,
		`SELECT stripe_customer_id FROM users WHERE id = $1`, userID).Scan(&bound); err != nil {
		t.Fatalf("reading the binding: %v", err)
	}
	if bound != "cus_existing" {
		t.Fatalf("binding = %q, want it unchanged", bound)
	}
}

func TestCreditRefusesANonPositiveAmount(t *testing.T) {
	var customers, credits int
	var body string
	s, pool := newService(t, creditStub(&customers, &credits, &body))
	userID := insertUser(t, pool, "zero@example.com")

	if err := s.CreditAccount(context.Background(), userID, 0, "invite_reward_3"); err == nil {
		t.Fatal("a zero credit was accepted — the sign convention is applied inside the " +
			"client, so a negative reaching it would BILL somebody for a reward")
	}
	if credits != 0 {
		t.Fatalf("credits placed = %d, want 0", credits)
	}
}
