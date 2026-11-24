package billing

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
)

// CheckoutAbandoned confirms that this account's consumer checkout expired unpaid.
// A live Stripe subscription can come from another session before its webhook arrives.
func (s *Service) CheckoutAbandoned(ctx context.Context, userID int64, sessionID string) (bool, error) {
	if s.client == nil {
		return false, ErrDisabled
	}
	var session struct {
		Status          string `json:"status"`
		PaymentStatus   string `json:"payment_status"`
		Customer        string `json:"customer"`
		ClientReference string `json:"client_reference_id"`
	}
	if err := s.client.do(ctx, http.MethodGet, "/checkout/sessions/"+url.PathEscape(sessionID), nil, "", &session); err != nil {
		return false, err
	}
	if session.Status != "expired" || session.PaymentStatus != "unpaid" ||
		session.Customer == "" || session.ClientReference != strconv.FormatInt(userID, 10) {
		return false, nil
	}
	customer, err := s.client.subscriberState(ctx, session.Customer)
	if err != nil {
		return false, err
	}
	for _, subscription := range customer.Subscriptions {
		if subscription.entitles(s.cfg.Prices) || subscription.entitles(s.cfg.UltraPrices) {
			return false, nil
		}
	}
	return true, nil
}
