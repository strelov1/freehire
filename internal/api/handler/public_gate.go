package handler

import (
	"context"
	"crypto/subtle"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"golang.org/x/sync/singleflight"

	"github.com/strelov1/freehire/internal/api/ratelimit"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/billing"
	"github.com/strelov1/freehire/internal/platform/db"
)

// The commercial API's budgets (openspec/changes/commercial-api-paid-keys).
//
// Anonymous: measured on prod for 2026-10-19, 34 of 24,859 API-calling IPs made more than
// 2,000 requests that day, and they were catalogue mirrors; the ai-job-search skill's IPs
// average 14 a day but burst to 200–300 a minute. So the DAY cap is what stops a mirror, and
// the minute cap is set where a skill burst degrades into Retry-After rather than breaking.
// Both are shared by the read and agent-search routes, so switching routes does not double a
// caller's allowance.
//
// Paid: per ACCOUNT, all of its keys drawing on one budget.
const (
	anonPerMinute     = 120
	anonPerDay        = 2000
	paidAPIPerMinute  = 1200
	paidDataPerMinute = 6000
)

// verifiedBotHeader is set by a Cloudflare Transform Rule when cf.client.bot is true, carrying
// a secret. The origin also listens publicly, so the header means nothing unless its value is
// the secret.
const verifiedBotHeader = "X-FH-Verified-Bot"

// keyTierTTL bounds how long a resolved key is trusted without asking again: a revoked key or
// a lapsed subscription keeps its rate for at most this long.
const keyTierTTL = time.Minute

// keyLookupTimeout bounds the one database read the gate may make, like ratelimit's own
// allowTimeout: a slow database must degrade a keyed request to anonymous, not stall it.
const keyLookupTimeout = 150 * time.Millisecond

// keyLookupsPerMinute bounds how many UNCACHED key lookups one IP may cause. A made-up key
// that passes the shape check costs a database read, so without this a flood of distinct
// fake keys would be a flood of queries on the hottest reads on the site. A real client
// presents the same key or two, which the cache answers after the first request.
const keyLookupsPerMinute = 30

// missCacheMax bounds the cache of keys that resolved to nothing. Its size is attacker-chosen,
// so past the bound it starts over — which costs only more lookups, already bounded above.
// Real keys live in a separate map whose size is bounded by the keys that exist, so a flood
// of fake ones can never evict them.
const missCacheMax = 10_000

// localsGateKeyUser carries the owner of a full-scope key the gate verified. It is NOT
// auth.LocalsUserID: a public route stays unauthenticated, and this only lets what counts the
// request (agentViews) tell one account from another behind a shared address.
const localsGateKeyUser = "public_gate_key_user"

// publicBudget is one public-read route class's anonymous budget before the strict switch.
type publicBudget struct {
	name      string
	perMinute int
	// legacyOnly keeps this budget for anonymous callers even under strict. The ChatGPT
	// app's MCP surfaces are reached from OpenAI's egress addresses, which every ChatGPT user
	// shares — a per-IP daily cap there would be a cap on the whole channel, our largest.
	legacyOnly bool
}

var (
	publicReadBudget  = publicBudget{name: "publicread", perMinute: publicReadsPerMinute}
	agentSearchBudget = publicBudget{name: "agentsearch", perMinute: agentSearchPerMinute}
	// The same NAME as agentSearchBudget on purpose: the MCP surfaces and /agent/jobs/search
	// run the same query, so they draw on one bucket, as they did before this gate existed.
	chatAgentBudget = publicBudget{name: agentSearchBudget.name, perMinute: agentSearchPerMinute, legacyOnly: true}
)

// apiKeyTiers resolves a presented key to its owner and the owner's API entitlement.
type apiKeyTiers interface {
	ResolveAPIKeyTier(ctx context.Context, tokenHash string) (db.ResolveAPIKeyTierRow, error)
}

// publicGateConfig is what the environment decides about the gate.
type publicGateConfig struct {
	// Strict applies the shared 120/min + 2,000/day anonymous budgets (PUBLIC_ANON_LIMITS=strict).
	// Off keeps each route's previous per-minute budget, so paid plans can go on sale before the
	// free tier tightens.
	Strict bool
	// BotSecret is VERIFIED_BOT_SECRET; empty disables the verified-bot exemption.
	BotSecret string
	// PricingURL is named in a 429, so a refused caller learns how to stop being refused.
	PricingURL string
}

// publicGate is the rate limiter of every public read route: one place that decides which
// class a caller is in — trusted peer, verified bot, paid key, anonymous — and which budget
// that class draws on.
type publicGate struct {
	throttler ratelimit.Throttler
	keys      apiKeyTiers
	cfg       publicGateConfig
	now       func() time.Time

	lookups singleflight.Group
	mu      sync.Mutex
	hits    map[string]cachedKeyTier
	misses  map[string]time.Time
}

type cachedKeyTier struct {
	row     db.ResolveAPIKeyTierRow
	expires time.Time
}

func newPublicGate(throttler ratelimit.Throttler, keys apiKeyTiers, cfg publicGateConfig) *publicGate {
	return &publicGate{throttler: throttler, keys: keys, cfg: cfg, now: time.Now,
		hits: map[string]cachedKeyTier{}, misses: map[string]time.Time{}}
}

// limiter is the middleware a public read route mounts for its budget class.
//
// A nil gate passes everything, the same degradation ratelimit.Middleware has for a nil
// Throttler: a test that builds a middleware set without one is not testing limits.
func (g *publicGate) limiter(b publicBudget) fiber.Handler {
	if g == nil {
		return func(c *fiber.Ctx) error { return c.Next() }
	}
	return func(c *fiber.Ctx) error {
		if ratelimit.TrustedPeer(c) || g.verifiedBot(c) {
			return c.Next()
		}
		userID, tier := g.paidTier(c)
		if userID != 0 {
			c.Locals(localsGateKeyUser, userID)
		}
		if err := g.enforce(c, b, userID, tier); err != nil {
			// The upsell names the paid plans to an ANONYMOUS caller only: a paying one who
			// ran out already holds a key, and telling them to use one reads as a bug.
			if tier == billing.APITierNone && isTooManyRequests(err) && g.cfg.PricingURL != "" {
				return fiber.NewError(fiber.StatusTooManyRequests,
					"too many requests, please try again later — for higher limits use an API key: "+g.cfg.PricingURL)
			}
			return err
		}
		return c.Next()
	}
}

func (g *publicGate) enforce(c *fiber.Ctx, b publicBudget, userID int64, tier billing.APITier) error {
	if tier != billing.APITierNone {
		return ratelimit.Enforce(c, g.throttler, "paid:"+strconv.FormatInt(userID, 10), paidPerMinute(tier), time.Minute)
	}
	ip := c.IP()
	if !g.cfg.Strict || b.legacyOnly {
		return ratelimit.Enforce(c, g.throttler, b.name+":"+ip, b.perMinute, time.Minute)
	}
	// The minute FIRST, so a request it refuses never reaches the day: GCRA charges only what
	// it admits, and a skill bursting past the minute cap would otherwise burn its daily
	// allowance on refusals within minutes. On an admitted request the response reports the
	// minute's budget — the one a client paces against and the one that refuses it first.
	minuteKey, dayKey := "anon:"+ip, "anonday:"+ip
	minute, minuteKnown := ratelimit.Check(c, g.throttler, minuteKey, anonPerMinute, time.Minute)
	if minuteKnown && !minute.Allowed {
		return ratelimit.Apply(c, minute, minuteKey, time.Minute)
	}
	if day, dayKnown := ratelimit.Check(c, g.throttler, dayKey, anonPerDay, 24*time.Hour); dayKnown && !day.Allowed {
		return ratelimit.Apply(c, day, dayKey, 24*time.Hour)
	}
	if minuteKnown {
		return ratelimit.Apply(c, minute, minuteKey, time.Minute)
	}
	return nil
}

// paidPerMinute is the public-read rate an API tier buys; zero for no tier.
func paidPerMinute(tier billing.APITier) int {
	switch tier {
	case billing.APITierData:
		return paidDataPerMinute
	case billing.APITierAPI:
		return paidAPIPerMinute
	default:
		return 0
	}
}

func isTooManyRequests(err error) bool {
	var fe *fiber.Error
	return errors.As(err, &fe) && fe.Code == fiber.StatusTooManyRequests
}

func (g *publicGate) verifiedBot(c *fiber.Ctx) bool {
	if g.cfg.BotSecret == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(c.Get(verifiedBotHeader)), []byte(g.cfg.BotSecret)) == 1
}

// paidTier is the account and API tier behind the request's key, or APITierNone when the
// request carries no key, a key of another scope, a key of an account without the product, or
// a key that could not be resolved. Only a full-scope key counts: an embed key is published
// inside third-party pages by design, and a paid rate on it would be anyone's for the copying.
func (g *publicGate) paidTier(c *fiber.Ctx) (int64, billing.APITier) {
	token, ok := auth.PresentedAPIKey(c)
	if !ok || g.keys == nil || !auth.WellFormedAPIKey(token) {
		return 0, billing.APITierNone
	}
	row, known := g.resolve(c, auth.HashAPIKey(token))
	if !known || row.Scope != auth.ScopeFull {
		return 0, billing.APITierNone
	}
	return row.UserID, billing.APITierOf(row.ApiUntilStripe.Time, row.ApiDataUntilStripe.Time, g.now())
}

// resolve looks a key up through the caches. The tier itself is resolved per request from the
// cached timestamps, so a subscription lapsing mid-entry is seen at once; what the cache
// delays is only a CHANGE to the account. A lookup that errored is not cached — the database
// being down must not pin a paying key to the anonymous rate for a minute after it recovers.
func (g *publicGate) resolve(c *fiber.Ctx, hash string) (db.ResolveAPIKeyTierRow, bool) {
	now := g.now()
	g.mu.Lock()
	hit, isHit := g.hits[hash]
	missAt, isMiss := g.misses[hash]
	g.mu.Unlock()
	switch {
	case isHit && now.Before(hit.expires):
		return hit.row, true
	case isMiss && now.Before(missAt.Add(keyTierTTL)):
		return db.ResolveAPIKeyTierRow{}, false
	}

	// An uncached lookup spends the caller's lookup budget. Out of it, the key is simply not
	// resolved this time: the request is served as anonymous, which is what it would be if
	// the key were fake, and a real key gets resolved on a later request.
	if !g.lookupAllowed(c) {
		return db.ResolveAPIKeyTierRow{}, false
	}

	// One lookup per key at a time: when a busy paid key's entry expires, its concurrent
	// requests share one read rather than each issuing their own.
	v, err, _ := g.lookups.Do(hash, func() (any, error) {
		ctx, cancel := context.WithTimeout(c.Context(), keyLookupTimeout)
		defer cancel()
		return g.keys.ResolveAPIKeyTier(ctx, hash)
	})
	switch {
	case err == nil:
		row := v.(db.ResolveAPIKeyTierRow)
		g.mu.Lock()
		g.hits[hash] = cachedKeyTier{row: row, expires: now.Add(keyTierTTL)}
		delete(g.misses, hash)
		g.mu.Unlock()
		return row, true
	case errors.Is(err, pgx.ErrNoRows):
		g.mu.Lock()
		if len(g.misses) >= missCacheMax {
			g.misses = map[string]time.Time{}
		}
		g.misses[hash] = now
		delete(g.hits, hash)
		g.mu.Unlock()
	}
	return db.ResolveAPIKeyTierRow{}, false
}

// lookupAllowed spends one of the caller's uncached-lookup budget. It is a ratelimit.Check and
// never an Apply: this budget is internal, so it must not write rate-limit headers or a
// Retry-After onto a request that is then served. Fail-open like every limiter here.
func (g *publicGate) lookupAllowed(c *fiber.Ctx) bool {
	d, known := ratelimit.Check(c, g.throttler, "keylookup:"+c.IP(), keyLookupsPerMinute, time.Minute)
	return !known || d.Allowed
}
