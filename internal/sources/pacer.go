package sources

import (
	"context"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// waiter gates a request until the rate limiter admits it. *rate.Limiter satisfies it;
// tests inject a fake to assert the gate fires without timing flake.
type waiter interface {
	Wait(ctx context.Context) error
}

// rateLimitedHTMLGetter wraps an HTMLGetter with a shared limiter so its aggregate GetHTML
// request rate stays under the limit, independent of the caller's worker concurrency. One
// instance carries one limiter, so every request routed through it — across boards and both
// the listing and detail paths — shares the same token bucket.
type rateLimitedHTMLGetter struct {
	inner   HTMLGetter
	limiter waiter
}

// GetHTML blocks on the limiter before delegating, so a cancelled context surfaces as the
// Wait error and the inner fetch is skipped.
func (g rateLimitedHTMLGetter) GetHTML(ctx context.Context, url string) (*html.Node, error) {
	if err := g.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	return g.inner.GetHTML(ctx, url)
}

// careers-page.com rate-limits by a per-IP request budget per time window, so a full run must
// hold its aggregate request rate under it (proxy egress and a narrow worker pool cap the
// burst, not the total-per-window — see the careerspage-request-pacer change). The interval is
// conservative because the true budget is unknown: under-shooting only lengthens a run, while
// over-shooting re-introduces the 429 starvation. Tune from observed convergence.
const (
	careerspageRequestInterval = 800 * time.Millisecond // ~1.25 req/s
	careerspageRequestBurst    = 2
)

// pacedCareerPageGetter wraps a getter with a fresh limiter shared across one registry build,
// so all of careerspage's requests in a run are paced under careers-page.com's window budget.
func pacedCareerPageGetter(c HTMLGetter) HTMLGetter {
	return rateLimitedHTMLGetter{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(careerspageRequestInterval), careerspageRequestBurst),
	}
}

// vagas.com.br rate-limits by a per-IP request budget: a full national-board crawl (three area
// listings paginated + a detail fan-out over hundreds of postings) fired unpaced through the
// single egress proxy IP 429s that IP and then 429s even spaced requests during the penalty
// window. Its detail pool bursts to defaultDetailWorkers, so the pacer — not the pool — must
// hold the aggregate rate under the window. The interval is more conservative than careerspage's
// because vagas 429'd hard and its true budget is unknown; tune from observed convergence.
const (
	vagasRequestInterval = time.Second // ~1 req/s
	vagasRequestBurst    = 1
)

// pacedVagasGetter wraps a getter with a fresh limiter shared across one registry build, so all
// of vagas's requests in a run stay under vagas.com.br's per-IP window on the single proxy IP.
func pacedVagasGetter(c HTMLGetter) HTMLGetter {
	return rateLimitedHTMLGetter{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(vagasRequestInterval), vagasRequestBurst),
	}
}
