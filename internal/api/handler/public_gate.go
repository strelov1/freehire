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
	"github.com/jackc/pgx/v5/pgtype"
	"golang.org/x/sync/singleflight"

	"github.com/strelov1/freehire/internal/api/ratelimit"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/auth/oauth2server"
	"github.com/strelov1/freehire/internal/platform/db"
)

// The budgets themselves are the rate-tier ladder (rate_tiers.go).
//
// Anonymous, under strict: measured on prod for 2026-10-19, 34 of 24,859 API-calling IPs made
// more than 2,000 requests that day, and they were catalogue mirrors; the ai-job-search skill's
// IPs average 14 a day but burst to 200–300 a minute. So the DAY cap is what stops a mirror, and
// the minute cap is set where a skill burst degrades into Retry-After rather than breaking.
//
// Every tier's budget is shared by the read and agent-search routes, so switching routes does
// not double a caller's allowance. A keyed tier is per ACCOUNT: all of its keys and access
// tokens draw on one budget.

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

// apiKeyTiers resolves a presented credential — an API key, or an MCP or CLI access token — to
// its owner and how far the owner's paid plans reach.
type apiKeyTiers interface {
	ResolveAPIKeyTier(ctx context.Context, tokenHash string) (db.ResolveAPIKeyTierRow, error)
	ResolveOAuthGrantTier(ctx context.Context, tokenHash string) (db.ResolveOAuthGrantTierRow, error)
}

// resolvedCredential is what the gate keeps about a credential it has looked up: whose it is,
// whether it may carry a tier at all, and the plan horizons the tier is read from per request.
type resolvedCredential struct {
	userID                                    int64
	full                                      bool
	apiUntil, dataUntil, proUntil, ultraUntil time.Time
}

// publicGateConfig is what the environment decides about the gate.
type publicGateConfig struct {
	// Strict applies the anonymous tier's shared budget (PUBLIC_ANON_LIMITS=strict). Off keeps
	// each route's previous per-minute budget for a caller with no key; keyed tiers apply either
	// way, which is why their minute never sits below that previous budget.
	Strict bool
	// Tiers is every tier's budget, already in force for Strict (RateTiers.inForce).
	Tiers RateTiers
	// BotSecret is VERIFIED_BOT_SECRET; empty disables the verified-bot exemption.
	BotSecret string
	// Origin is the site's address. A 429 names the next rung up from it, so a refused caller
	// learns how to stop being refused.
	Origin string
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
	hits    map[string]cachedCredential
	misses  map[string]time.Time
}

type cachedCredential struct {
	cred    resolvedCredential
	expires time.Time
}

func newPublicGate(throttler ratelimit.Throttler, keys apiKeyTiers, cfg publicGateConfig) *publicGate {
	return &publicGate{throttler: throttler, keys: keys, cfg: cfg, now: time.Now,
		hits: map[string]cachedCredential{}, misses: map[string]time.Time{}}
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
		userID, tier := g.callerTier(c)
		if userID != 0 {
			c.Locals(localsGateKeyUser, userID)
		}
		if err := g.enforce(c, b, userID, tier); err != nil {
			if path, next := nextRung(tier); next != "" && isTooManyRequests(err) && g.cfg.Origin != "" {
				return fiber.NewError(fiber.StatusTooManyRequests,
					"Rate limit reached. "+next+" for a higher limit: "+g.cfg.Origin+path)
			}
			return err
		}
		return c.Next()
	}
}

// nextRung is where a refused caller of a tier can go for a bigger budget, and the sentence
// that sends them there, or "" at the top of the ladder. The refusal is read by whoever wrote
// the client — and, through an agent, by the person it works for — so it names freehire and
// what to do, not the mechanism; Retry-After already says when to try again.
func nextRung(tier rateTier) (path, next string) {
	switch tier {
	case rateAnonymous:
		return "/my/integrations/api-keys", "Create a free freehire account"
	case rateFree:
		return "/pricing", "Upgrade your freehire account"
	case ratePro, rateUltra, rateAPI:
		return "/jobs-api", "See the freehire API plans"
	}
	return "", ""
}

func (g *publicGate) enforce(c *fiber.Ctx, b publicBudget, userID int64, tier rateTier) error {
	if tier != rateAnonymous {
		id := strconv.FormatInt(userID, 10)
		return g.enforceMinuteAndDay(c, "acct:"+id, "acctday:"+id, g.cfg.Tiers[tier])
	}
	ip := c.IP()
	if !g.cfg.Strict || b.legacyOnly {
		return ratelimit.Enforce(c, g.throttler, b.name+":"+ip, b.perMinute, time.Minute)
	}
	return g.enforceMinuteAndDay(c, "anon:"+ip, "anonday:"+ip, g.cfg.Tiers[rateAnonymous])
}

// enforceMinuteAndDay applies a tier's two budgets; a PerDay of 0 is no daily cap.
//
// The minute FIRST, so a request it refuses never reaches the day: GCRA charges only what it
// admits, and a skill bursting past the minute cap would otherwise burn its daily allowance on
// refusals within minutes. On an admitted request the response reports the minute's budget —
// the one a client paces against and the one that refuses it first.
func (g *publicGate) enforceMinuteAndDay(c *fiber.Ctx, minuteKey, dayKey string, l rateLimit) error {
	minute, minuteKnown := ratelimit.Check(c, g.throttler, minuteKey, l.PerMinute, time.Minute)
	if minuteKnown && !minute.Allowed {
		return ratelimit.Apply(c, minute, minuteKey, time.Minute)
	}
	if l.PerDay > 0 {
		if day, dayKnown := ratelimit.Check(c, g.throttler, dayKey, l.PerDay, 24*time.Hour); dayKnown && !day.Allowed {
			return ratelimit.Apply(c, day, dayKey, 24*time.Hour)
		}
	}
	if minuteKnown {
		return ratelimit.Apply(c, minute, minuteKey, time.Minute)
	}
	return nil
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

// callerTier is the account and rate tier behind the request's credential, or (0,
// rateAnonymous) when it carries none, one of another scope, or one that could not be resolved.
// Two credentials count: an API key and an MCP or CLI access token, which act as the same
// account on every REST route (apiKeys.AuthenticateAPIKey). Only a full-scope key counts: an
// embed key is published inside third-party pages by design, and a keyed rate on it would be
// anyone's for the copying.
func (g *publicGate) callerTier(c *fiber.Ctx) (int64, rateTier) {
	token := auth.PresentedBearer(c)
	if g.keys == nil {
		return 0, rateAnonymous
	}
	var cred resolvedCredential
	var known bool
	switch {
	case auth.WellFormedAPIKey(token):
		cred, known = g.resolve(c, auth.HashAPIKey(token), g.lookupAPIKey)
	case oauth2server.WellFormedAccessToken(token):
		cred, known = g.resolve(c, oauth2server.HashToken(token), g.lookupAccessToken)
	}
	if !known || !cred.full {
		return 0, rateAnonymous
	}
	return cred.userID, accountRateTier(cred.apiUntil, cred.dataUntil, cred.proUntil, cred.ultraUntil, g.now())
}

func (g *publicGate) lookupAPIKey(ctx context.Context, hash string) (resolvedCredential, error) {
	row, err := g.keys.ResolveAPIKeyTier(ctx, hash)
	return credentialOf(row.UserID, row.Scope == auth.ScopeFull,
		row.ApiUntilStripe, row.ApiDataUntilStripe, row.ProUntil, row.UltraUntil), err
}

// lookupAccessToken resolves an MCP or CLI token, which is always full scope: an OAuth grant has
// no narrower variant to request.
func (g *publicGate) lookupAccessToken(ctx context.Context, hash string) (resolvedCredential, error) {
	row, err := g.keys.ResolveOAuthGrantTier(ctx, hash)
	return credentialOf(row.UserID, true,
		row.ApiUntilStripe, row.ApiDataUntilStripe, row.ProUntil, row.UltraUntil), err
}

func credentialOf(userID int64, full bool, api, data, pro, ultra pgtype.Timestamptz) resolvedCredential {
	return resolvedCredential{userID: userID, full: full,
		apiUntil: api.Time, dataUntil: data.Time, proUntil: pro.Time, ultraUntil: ultra.Time}
}

// resolve looks a credential up through the caches. The tier itself is resolved per request
// from the cached timestamps, so a subscription lapsing mid-entry is seen at once; what the
// cache delays is only a CHANGE to the account. A lookup that errored is not cached — the
// database being down must not pin a paying key to the anonymous rate for a minute after it
// recovers. The two credential kinds share the caches: their hashes cannot collide, since the
// tokens carry different prefixes.
func (g *publicGate) resolve(c *fiber.Ctx, hash string, lookup func(context.Context, string) (resolvedCredential, error)) (resolvedCredential, bool) {
	now := g.now()
	g.mu.Lock()
	hit, isHit := g.hits[hash]
	missAt, isMiss := g.misses[hash]
	g.mu.Unlock()
	switch {
	case isHit && now.Before(hit.expires):
		return hit.cred, true
	case isMiss && now.Before(missAt.Add(keyTierTTL)):
		return resolvedCredential{}, false
	}

	// An uncached lookup spends the caller's lookup budget. Out of it, the key is simply not
	// resolved this time: the request is served as anonymous, which is what it would be if
	// the key were fake, and a real key gets resolved on a later request.
	if !g.lookupAllowed(c) {
		return resolvedCredential{}, false
	}

	// One lookup per key at a time: when a busy paid key's entry expires, its concurrent
	// requests share one read rather than each issuing their own.
	v, err, _ := g.lookups.Do(hash, func() (any, error) {
		ctx, cancel := context.WithTimeout(c.Context(), keyLookupTimeout)
		defer cancel()
		return lookup(ctx, hash)
	})
	switch {
	case err == nil:
		cred := v.(resolvedCredential)
		g.mu.Lock()
		g.hits[hash] = cachedCredential{cred: cred, expires: now.Add(keyTierTTL)}
		delete(g.misses, hash)
		g.mu.Unlock()
		return cred, true
	case errors.Is(err, pgx.ErrNoRows):
		g.mu.Lock()
		if len(g.misses) >= missCacheMax {
			g.misses = map[string]time.Time{}
		}
		g.misses[hash] = now
		delete(g.hits, hash)
		g.mu.Unlock()
	}
	return resolvedCredential{}, false
}

// lookupAllowed spends one of the caller's uncached-lookup budget. It is a ratelimit.Check and
// never an Apply: this budget is internal, so it must not write rate-limit headers or a
// Retry-After onto a request that is then served. Fail-open like every limiter here.
func (g *publicGate) lookupAllowed(c *fiber.Ctx) bool {
	d, known := ratelimit.Check(c, g.throttler, "keylookup:"+c.IP(), keyLookupsPerMinute, time.Minute)
	return !known || d.Allowed
}
