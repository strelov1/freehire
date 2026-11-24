package billing

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestDisabledServiceRefusesEverything is the guarantee that lets this package ship in a
// public repository. A deployment that never sets the variables must find every entry point
// closed — and closed with ErrDisabled, which the HTTP surface renders as 404, not as a
// server error.
//
// The Service is constructed with a nil *db.Queries on purpose: if any of these paths
// reached the database, this test would panic rather than pass.
func TestDisabledServiceRefusesEverything(t *testing.T) {
	setEnv(t, "", "", "", "")
	s := New(ConfigFromEnv(), nil)

	if s.Enabled() {
		t.Fatal("want disabled")
	}

	if _, err := s.Accept([]byte(`{}`), "", time.Now()); !errors.Is(err, ErrDisabled) {
		t.Errorf("Accept: want ErrDisabled, got %v", err)
	}
	if _, _, err := s.Record(context.Background(), Event{ID: "evt_1"}); !errors.Is(err, ErrDisabled) {
		t.Errorf("Record: want ErrDisabled, got %v", err)
	}
	if err := s.SyncUser(context.Background(), 1); !errors.Is(err, ErrDisabled) {
		t.Errorf("SyncUser: want ErrDisabled, got %v", err)
	}
	if _, _, err := s.CheckoutURL(context.Background(), 1, "", Discount{}, false); !errors.Is(err, ErrNoCheckout) {
		t.Errorf("CheckoutURL: want ErrNoCheckout, got %v", err)
	}
	if _, err := s.ManagementURL(context.Background(), 1); !errors.Is(err, ErrNoCheckout) {
		t.Errorf("ManagementURL: want ErrNoCheckout, got %v", err)
	}
}

func TestAcceptVerifiesBeforeParsing(t *testing.T) {
	setEnv(t, "sk_test", testSecret, "price_pro_monthly", "https://freehire.me")
	s := New(ConfigFromEnv(), nil)

	now := time.Date(2026, time.September, 3, 12, 0, 0, 0, time.UTC)
	body := []byte(`{"id":"evt_1","type":"invoice.paid","data":{"object":{"customer":"cus_9"}}}`)

	t.Run("a signed delivery is parsed", func(t *testing.T) {
		ev, err := s.Accept(body, signHeader(body, testSecret, now), now)
		if err != nil {
			t.Fatalf("want no error, got %v", err)
		}
		if ev.ID != "evt_1" || ev.CustomerID != "cus_9" {
			t.Fatalf("parsed wrong: %+v", ev)
		}
	})

	t.Run("an unsigned delivery never reaches the parser", func(t *testing.T) {
		if _, err := s.Accept(body, "", now); err == nil {
			t.Fatal("want an error, got nil")
		}
	})

	t.Run("a wrongly signed delivery is refused even though it parses", func(t *testing.T) {
		if _, err := s.Accept(body, signHeader(body, "whsec_other", now), now); err == nil {
			t.Fatal("want an error, got nil")
		}
	})
}

func TestSponsoredTopupURLRefusals(t *testing.T) {
	setEnv(t, "", "", "", "")
	if _, err := New(ConfigFromEnv(), nil).SponsoredTopupURL(context.Background(), 1, 2, 5000); !errors.Is(err, ErrDisabled) {
		t.Errorf("disabled: want ErrDisabled, got %v", err)
	}

	setEnv(t, "sk_test", testSecret, proPrice, "https://freehire.me")
	s := New(ConfigFromEnv(), nil) // nil queries: the amount is refused before any read
	var below *BelowMinimumError
	if _, err := s.SponsoredTopupURL(context.Background(), 1, 2, 2899); !errors.As(err, &below) || below.MinCents != 2900 {
		t.Errorf("below the default minimum: got %v", err)
	}

	t.Setenv("SPONSORED_MIN_TOPUP_CENTS", "5000")
	s = New(ConfigFromEnv(), nil)
	if _, err := s.SponsoredTopupURL(context.Background(), 1, 2, 4999); !errors.As(err, &below) || below.MinCents != 5000 {
		t.Errorf("below the configured minimum: got %v", err)
	}
}
