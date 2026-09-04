package billing

import (
	"context"
	"net/http"
	"net/url"
	"slices"
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

// priceRetryAfter is how long a FAILED read is honoured for.
//
// A failure has to expire the cache too, or an unreachable provider turns every request
// into another attempt at it — which is the load pattern this cache exists to prevent,
// arriving exactly when the provider can least take it, and spending the rate limit that
// the webhook's own reads need. Shorter than the TTL because a page with no price recovers
// on its own rather than waiting out a full cycle.
const priceRetryAfter = 30 * time.Second

// priceCache holds the last answer and decides who goes and gets the next one.
//
// The lock covers the FIELDS and never a provider call. Holding it across the network would
// serialise every visitor behind one request to a third party — on a public page, in front
// of crawler traffic — which is a self-inflicted outage rather than a cache.
type priceCache struct {
	mu sync.Mutex
	// until is when the held answer stops being served. Set on success and on failure
	// alike, so a provider outage backs off instead of retrying per request.
	until    time.Time
	entries  []PublicPrice
	fetching bool
}

// claim reports whether this caller is the one that should refresh, and hands back what is
// currently held either way.
//
// At most one refresh is in flight. The others are not queued behind it — they are served
// the previous answer immediately, because a pricing page that renders a five-minute-old
// price now is better than one that renders the current price after a provider timeout.
func (p *priceCache) claim() (entries []PublicPrice, mine bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.fetching || time.Now().Before(p.until) {
		return p.entries, false
	}
	p.fetching = true
	return p.entries, true
}

// publish stores a refresh's outcome and returns what callers should now be given.
func (p *priceCache) publish(entries []PublicPrice, err error) []PublicPrice {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.fetching = false
	if err != nil {
		p.until = time.Now().Add(priceRetryAfter)
		return p.entries
	}
	p.entries, p.until = entries, time.Now().Add(priceCacheTTL)
	return entries
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

	held, mine := s.prices.claim()
	if !mine {
		return held
	}
	// The claim is released by publish on every path, including the error one — a refresh
	// that returned without clearing the flag would leave the cache frozen on its last
	// answer for the life of the process.
	return s.prices.publish(s.fetchPrices(ctx))
}

// fetchPrices reads every configured price from the provider.
//
// All or nothing. A partial list is worse than none, because a page showing one of two
// plans looks complete — so a single failure discards what was already read.
func (s *Service) fetchPrices(ctx context.Context) ([]PublicPrice, error) {
	out := make([]PublicPrice, 0, len(s.cfg.Prices))
	for i, id := range s.cfg.Prices {
		p, err := s.client.price(ctx, id)
		if err != nil {
			return nil, err
		}
		p.Default = i == 0
		out = append(out, p)
	}
	return out, nil
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
	return slices.Contains(c.Prices, priceID)
}
