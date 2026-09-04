package billing

import (
	"context"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// PublicPrice is one thing a visitor may buy, as a pricing page needs it.
//
// The money is read from the PROVIDER, never from our configuration. A price written into
// an environment variable beside its id is a second source of truth about what something
// costs, and the two disagree the first time one is changed — on the page a customer can
// hold us to.
type PublicPrice struct {
	ID string `json:"id"`
	// AmountCents is the smallest currency unit, as the provider stores it. The page
	// formats; nothing here rounds.
	AmountCents int64  `json:"amount_cents"`
	Currency    string `json:"currency"`
	// Interval is "month" or "year".
	Interval string `json:"interval"`
	// Default marks the price a new subscriber is sold unless they choose otherwise.
	Default bool `json:"default"`
}

// priceCacheTTL bounds how stale a displayed price may be.
//
// The endpoint is public and unauthenticated, so without a cache a crawler would turn every
// page view into a provider call. Five minutes is short enough that a deliberate price
// change appears within one coffee break, and long enough that the page costs nothing.
const priceCacheTTL = 5 * time.Minute

type priceCache struct {
	mu      sync.Mutex
	at      time.Time
	entries []PublicPrice
}

// PublicPrices lists what may be bought, cheapest interval first as configured.
//
// A provider that cannot be reached returns the last good answer if there is one, and an
// empty list otherwise. An empty list is the honest failure: the page then shows the plan
// comparison without a price, rather than a price that might be wrong.
func (s *Service) PublicPrices(ctx context.Context) []PublicPrice {
	if !s.Enabled() {
		return nil
	}

	s.prices.mu.Lock()
	defer s.prices.mu.Unlock()

	if time.Since(s.prices.at) < priceCacheTTL && s.prices.entries != nil {
		return s.prices.entries
	}

	out := make([]PublicPrice, 0, len(s.cfg.Prices))
	for i, id := range s.cfg.Prices {
		p, err := s.client.price(ctx, id)
		if err != nil {
			// Keep whatever we last had rather than publishing a partial list: a page
			// showing one of two plans is worse than a page showing neither, because it
			// looks complete.
			return s.prices.entries
		}
		p.Default = i == 0
		out = append(out, p)
	}

	s.prices.at, s.prices.entries = time.Now(), out
	return out
}

// price reads one price from the provider.
func (c *client) price(ctx context.Context, id string) (PublicPrice, error) {
	var raw struct {
		ID         string `json:"id"`
		UnitAmount int64  `json:"unit_amount"`
		Currency   string `json:"currency"`
		Recurring  struct {
			Interval string `json:"interval"`
		} `json:"recurring"`
	}
	if err := c.do(ctx, http.MethodGet, "/prices/"+url.PathEscape(id), nil, &raw); err != nil {
		return PublicPrice{}, err
	}
	return PublicPrice{
		ID:          raw.ID,
		AmountCents: raw.UnitAmount,
		Currency:    raw.Currency,
		Interval:    raw.Recurring.Interval,
	}, nil
}

// Sells reports whether this price is one we offer. The checkout route asks before creating
// a session, because the price arrives from the browser and a caller who could name any
// price could name one costing nothing.
func (c Config) Sells(priceID string) bool {
	for _, id := range c.Prices {
		if id == priceID {
			return true
		}
	}
	return false
}
