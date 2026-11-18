package billing

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"
)

// PaymentAttempt describes one charge, including retries of the same PaymentIntent.
// Only safe display fields cross this boundary; provider diagnostics stay private.
type PaymentAttempt struct {
	ID                  string    `json:"id"`
	Date                time.Time `json:"date"`
	AmountCents         int64     `json:"amount_cents"`
	AmountRefundedCents int64     `json:"amount_refunded_cents"`
	Currency            string    `json:"currency"`
	Status              string    `json:"status"`
	Reason              string    `json:"reason,omitempty"`
	CardBrand           string    `json:"card_brand,omitempty"`
	CardLast4           string    `json:"card_last4,omitempty"`
	CardFunding         string    `json:"card_funding,omitempty"`
	Wallet              string    `json:"wallet,omitempty"`
}

// PaymentAttempts is available before a first successful subscription, too.
func (s *Service) PaymentAttempts(ctx context.Context, userID int64) ([]PaymentAttempt, error) {
	if !s.cfg.Enabled() {
		return nil, ErrDisabled
	}
	customer, err := s.stripe().customerOf(ctx, userID)
	if errors.Is(err, ErrNoSubscription) {
		return []PaymentAttempt{}, nil
	}
	if err != nil {
		return nil, err
	}
	return s.client.paymentAttempts(ctx, customer)
}

func (c *client) paymentAttempts(ctx context.Context, customer string) ([]PaymentAttempt, error) {
	query := url.Values{"customer": {customer}, "limit": {"20"}}
	var raw struct {
		Data []struct {
			ID             string `json:"id"`
			Created        int64  `json:"created"`
			Amount         int64  `json:"amount"`
			AmountRefunded int64  `json:"amount_refunded"`
			Currency       string `json:"currency"`
			Status         string `json:"status"`
			Refunded       bool   `json:"refunded"`
			FailureCode    string `json:"failure_code"`
			Outcome        struct {
				Type   string `json:"type"`
				Reason string `json:"reason"`
			} `json:"outcome"`
			PaymentMethodDetails struct {
				Card struct {
					Brand   string `json:"brand"`
					Last4   string `json:"last4"`
					Funding string `json:"funding"`
					Wallet  struct {
						Type string `json:"type"`
					} `json:"wallet"`
				} `json:"card"`
			} `json:"payment_method_details"`
		} `json:"data"`
	}
	if err := c.do(ctx, http.MethodGet, "/charges?"+query.Encode(), nil, "", &raw); err != nil {
		return nil, err
	}
	out := make([]PaymentAttempt, 0, len(raw.Data))
	for _, charge := range raw.Data {
		card := charge.PaymentMethodDetails.Card
		attempt := PaymentAttempt{
			ID: charge.ID, Date: time.Unix(charge.Created, 0).UTC(),
			AmountCents: charge.Amount, AmountRefundedCents: charge.AmountRefunded,
			Currency: charge.Currency, Status: charge.Status,
			CardBrand: card.Brand, CardLast4: card.Last4, CardFunding: card.Funding,
			Wallet: card.Wallet.Type,
		}
		switch {
		case charge.Refunded:
			attempt.Status = "refunded"
		case charge.AmountRefunded > 0:
			attempt.Status = "partially_refunded"
		case charge.Status == "failed":
			attempt.Reason = paymentFailureReason(charge.Outcome.Type, charge.Outcome.Reason, charge.FailureCode)
		}
		out = append(out, attempt)
	}
	return out, nil
}

func paymentFailureReason(outcomeType, reason, failureCode string) string {
	// Security diagnostics (including lost/stolen card reports) must never be displayed.
	if outcomeType == "blocked" {
		return "security_check"
	}
	if reason == "" {
		reason = failureCode
	}
	switch reason {
	case "insufficient_funds", "expired_card", "incorrect_cvc", "incorrect_number",
		"processing_error", "authentication_required", "card_not_supported", "currency_not_supported":
		return reason
	default:
		return "declined"
	}
}
