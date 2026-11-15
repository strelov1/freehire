package billing

import (
	"context"
	"log"
	"net/http"
	"net/url"
	"slices"
	"sync"
	"time"
)

// PublicPrice is one thing a visitor may buy, as a pricing page needs it. The money is read
// from the provider, never from our configuration, to keep one source of truth.
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
	// Tier is which plan this price buys: "pro", "ultra", or "api"/"api_data" (see APITier).
	// It comes from the configured list the id appears in, the same thing that decides what a
	// subscription confers. A string, not plan.Tier, because this package must not import
	// internal/ai/plan.
	Tier string `json:"tier"`
}

// Price tier names as they travel on the wire; they match internal/ai/plan's spellings.
const (
	tierPro   = "pro"
	tierUltra = "ultra"
)

// priceCacheTTL bounds how stale a displayed price may be. The endpoint is public, so
// without a cache a crawler would turn every page view into a provider call.
const priceCacheTTL = 5 * time.Minute

// priceRetryAfter is how long a failed read is honoured, so an unreachable provider is not
// retried per request (and the webhook's rate limit is not spent). Shorter than the TTL so a
// page with no price recovers sooner.
const priceRetryAfter = 30 * time.Second

// priceCache holds the last answer and decides who fetches the next one. The lock covers
// the fields, never a provider call, or every visitor would serialise behind it.
type priceCache struct {
	mu sync.Mutex
	// until is when the held answer stops being served. Set on failure too, so an outage
	// backs off.
	until    time.Time
	entries  []PublicPrice
	fetching bool
}

// claim reports whether this caller should refresh, and returns what is held either way.
// At most one refresh is in flight; the others get the previous answer immediately.
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

// PublicPrices lists what may be bought, in configured order. An unreachable provider yields
// the last good answer, else an empty list, so the page shows no price rather than a wrong one.
func (s *Service) PublicPrices(ctx context.Context) []PublicPrice {
	// Stripe only: there is no store price list, so this skips the provider seam.
	if !s.cfg.Enabled() {
		return nil
	}

	held, mine := s.prices.claim()
	if !mine {
		return held
	}
	// publish clears the claim on every path, or the cache would freeze on its last answer.
	return s.prices.publish(s.fetchPrices(ctx))
}

// fetchPrices reads every configured price from the provider.
//
// All or nothing per product: a partial list looks complete. The API product is read apart
// so a mistyped API price id cannot take the Pro and Ultra prices off the page.
func (s *Service) fetchPrices(ctx context.Context) ([]PublicPrice, error) {
	out, err := s.readTiers(ctx, []priceTier{{tierPro, s.cfg.Prices}, {tierUltra, s.cfg.UltraPrices}})
	if err != nil {
		return nil, err
	}
	api, err := s.readTiers(ctx, []priceTier{{string(APITierAPI), s.cfg.APIPrices}, {string(APITierData), s.cfg.APIDataPrices}})
	if err != nil {
		log.Printf("billing: reading the API product's prices: %v (omitting them)", err)
		return out, nil
	}
	return append(out, api...), nil
}

type priceTier struct {
	tier string
	ids  []string
}

// readTiers reads one product's prices, failing as a whole on the first failure.
func (s *Service) readTiers(ctx context.Context, tiers []priceTier) ([]PublicPrice, error) {
	var out []PublicPrice
	for _, list := range tiers {
		for i, id := range list.ids {
			p, err := s.client.price(ctx, id)
			if err != nil {
				return nil, err
			}
			// The first of each list is that tier's default.
			p.Default = i == 0
			p.Tier = list.tier
			out = append(out, p)
		}
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
	if err := c.do(ctx, http.MethodGet, "/prices/"+url.PathEscape(id), nil, "", &raw); err != nil {
		return PublicPrice{}, err
	}
	return PublicPrice{
		ID:          raw.ID,
		AmountCents: raw.UnitAmount,
		Currency:    raw.Currency,
		Interval:    raw.Recurring.Interval,
	}, nil
}

// Sells reports whether this price is one we offer. Checkout asks first because the price
// comes from the browser and could otherwise name one costing nothing.
func (c Config) Sells(priceID string) bool {
	return slices.Contains(c.Prices, priceID) || slices.Contains(c.UltraPrices, priceID) ||
		slices.Contains(c.APIPrices, priceID) || slices.Contains(c.APIDataPrices, priceID)
}
