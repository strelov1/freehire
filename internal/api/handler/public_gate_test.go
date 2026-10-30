package handler

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/api/ratelimit"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/auth/oauth2server"
	"github.com/strelov1/freehire/internal/identity/billing"
	"github.com/strelov1/freehire/internal/platform/db"
)

// check is one Allow call a gate made.
type check struct {
	key    string
	limit  int
	window time.Duration
}

// checkRecorder admits or refuses everything and records each check it was asked.
type checkRecorder struct {
	mu     sync.Mutex
	refuse bool
	calls  []check
}

func (r *checkRecorder) Allow(_ context.Context, key string, limit int, window time.Duration) (ratelimit.Decision, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, check{key, limit, window})
	if r.refuse && !strings.HasPrefix(key, "keylookup:") {
		return ratelimit.Decision{Limit: limit, ResetAfter: window, RetryAfter: window}, nil
	}
	return ratelimit.Decision{Allowed: true, Limit: limit, Remaining: limit - 1, ResetAfter: window}, nil
}

// got is the budget checks made, without the gate's internal key-lookup budget.
func (r *checkRecorder) got() []check {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []check
	for _, c := range r.calls {
		if !strings.HasPrefix(c.key, "keylookup:") {
			out = append(out, c)
		}
	}
	return out
}

// fakeKeyTiers resolves token hashes to rows and counts lookups.
type fakeKeyTiers struct {
	mu      sync.Mutex
	rows    map[string]db.ResolveAPIKeyTierRow
	grants  map[string]db.ResolveOAuthGrantTierRow
	err     error
	lookups int
}

func (f *fakeKeyTiers) ResolveOAuthGrantTier(_ context.Context, hash string) (db.ResolveOAuthGrantTierRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lookups++
	if f.err != nil {
		return db.ResolveOAuthGrantTierRow{}, f.err
	}
	row, ok := f.grants[hash]
	if !ok {
		return db.ResolveOAuthGrantTierRow{}, pgx.ErrNoRows
	}
	return row, nil
}

func (f *fakeKeyTiers) ResolveAPIKeyTier(_ context.Context, hash string) (db.ResolveAPIKeyTierRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lookups++
	if f.err != nil {
		return db.ResolveAPIKeyTierRow{}, f.err
	}
	row, ok := f.rows[hash]
	if !ok {
		return db.ResolveAPIKeyTierRow{}, pgx.ErrNoRows
	}
	return row, nil
}

var gateNow = time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)

// wellFormedKey mints a token with the shape of a real key, distinct per seed.
func wellFormedKey(seed byte) string {
	return "fhk_" + base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{seed}, 32))
}

// wellFormedAccessToken mints a token with the shape of a real MCP or CLI access token.
func wellFormedAccessToken(seed byte) string {
	return "fhm_" + base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{seed}, 32))
}

var (
	apiKeyToken  = wellFormedKey('a')
	dataKeyToken = wellFormedKey('d')
	embedToken   = wellFormedKey('e')
	unpaidToken  = wellFormedKey('u')
	unknownToken = wellFormedKey('x')
	lapsedToken  = wellFormedKey('l')
	proToken     = wellFormedKey('p')
	ultraToken   = wellFormedKey('t')
	mcpProToken  = wellFormedAccessToken('m')
	mcpFreeToken = wellFormedAccessToken('f')
)

const gateIP = "203.0.113.9"

func paidKeys() *fakeKeyTiers {
	future := gateNow.Add(24 * time.Hour)
	return &fakeKeyTiers{rows: map[string]db.ResolveAPIKeyTierRow{
		auth.HashAPIKey(apiKeyToken):  {UserID: 7, Scope: auth.ScopeFull, ApiUntilStripe: ts(future)},
		auth.HashAPIKey(dataKeyToken): {UserID: 8, Scope: auth.ScopeFull, ApiDataUntilStripe: ts(future)},
		auth.HashAPIKey(embedToken):   {UserID: 7, Scope: auth.ScopeEmbed, ApiUntilStripe: ts(future)},
		auth.HashAPIKey(unpaidToken):  {UserID: 9, Scope: auth.ScopeFull},
		auth.HashAPIKey(lapsedToken):  {UserID: 10, Scope: auth.ScopeFull, ApiUntilStripe: ts(gateNow.Add(-time.Hour))},
		auth.HashAPIKey(proToken):     {UserID: 11, Scope: auth.ScopeFull, ProUntil: ts(future)},
		// Ultra and Pro at once: the better plan wins.
		auth.HashAPIKey(ultraToken): {UserID: 12, Scope: auth.ScopeFull, ProUntil: ts(future), UltraUntil: ts(future)},
	}, grants: map[string]db.ResolveOAuthGrantTierRow{
		oauth2server.HashToken(mcpProToken):  {UserID: 13, ProUntil: ts(future)},
		oauth2server.HashToken(mcpFreeToken): {UserID: 14},
	}}
}

func newTestGate(th ratelimit.Throttler, keys apiKeyTiers, strict bool, botSecret string) *publicGate {
	g := newPublicGate(th, keys, publicGateConfig{Strict: strict, Tiers: DefaultRateTiers().inForce(strict),
		BotSecret: botSecret, Origin: "https://freehire.me"})
	g.now = func() time.Time { return gateNow }
	return g
}

// gateApp mounts the gate's two budgets on two routes, the way the read and agent-search
// routes mount them.
func gateApp(g *publicGate) *fiber.App {
	app := fiber.New(fiber.Config{ProxyHeader: fiber.HeaderXForwardedFor})
	ok := func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) }
	app.Get("/read", g.limiter(publicReadBudget), ok)
	app.Get("/agent", g.limiter(agentSearchBudget), ok)
	return app
}

func gateGet(t *testing.T, app *fiber.App, path string, headers map[string]string) (int, string) {
	t.Helper()
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, path, nil)
	req.Header.Set(fiber.HeaderXForwardedFor, gateIP)
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("GET %s: %v", path, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func bearer(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

func TestPublicGate_LegacyAnonymousBudgetsUntilStrict(t *testing.T) {
	rec := &checkRecorder{}
	app := gateApp(newTestGate(rec, paidKeys(), false, ""))
	gateGet(t, app, "/read", nil)
	gateGet(t, app, "/agent", nil)
	want := []check{
		{"publicread:" + gateIP, publicReadsPerMinute, time.Minute},
		{"agentsearch:" + gateIP, agentSearchPerMinute, time.Minute},
	}
	if got := rec.got(); !equalChecks(got, want) {
		t.Fatalf("checks = %+v, want %+v", got, want)
	}
}

func TestPublicGate_StrictAnonymousIsSharedPerMinuteAndPerDay(t *testing.T) {
	rec := &checkRecorder{}
	app := gateApp(newTestGate(rec, paidKeys(), true, ""))
	gateGet(t, app, "/read", nil)
	gateGet(t, app, "/agent", nil)
	anon := DefaultRateTiers()[rateAnonymous]
	one := []check{
		{"anon:" + gateIP, anon.PerMinute, time.Minute},
		{"anonday:" + gateIP, anon.PerDay, 24 * time.Hour},
	}
	if got := rec.got(); !equalChecks(got, append(one, one...)) {
		t.Fatalf("checks = %+v, want the same two shared keys on both routes", got)
	}
	if anon.PerMinute != 120 || anon.PerDay != 2000 {
		t.Fatalf("anonymous budgets = %+v, the spec says 120 a minute and 2000 a day", anon)
	}
}

// Every credential of an account — any API key, any MCP or CLI access token — draws on one
// per-account budget at the account's tier, on both routes, before and after strict alike;
// only the daily caps below the API product wait for strict.
func TestPublicGate_KeysAreLimitedPerAccountAtTheirTier(t *testing.T) {
	cases := []struct {
		name  string
		token string
		user  string
		tier  rateTier
	}{
		{"api product", apiKeyToken, "7", rateAPI},
		{"data product", dataKeyToken, "8", rateData},
		{"key without a plan", unpaidToken, "9", rateFree},
		{"lapsed api product", lapsedToken, "10", rateFree},
		{"pro", proToken, "11", ratePro},
		{"ultra beats pro", ultraToken, "12", rateUltra},
		{"mcp token of a pro account", mcpProToken, "13", ratePro},
		{"mcp token without a plan", mcpFreeToken, "14", rateFree},
	}
	for _, strict := range []bool{false, true} {
		for _, tc := range cases {
			t.Run(tc.name+"/strict="+strconv.FormatBool(strict), func(t *testing.T) {
				rec := &checkRecorder{}
				app := gateApp(newTestGate(rec, paidKeys(), strict, ""))
				gateGet(t, app, "/read", bearer(tc.token))
				gateGet(t, app, "/agent", bearer(tc.token))
				l := DefaultRateTiers().inForce(strict)[tc.tier]
				one := []check{{"acct:" + tc.user, l.PerMinute, time.Minute}}
				if l.PerDay > 0 {
					one = append(one, check{"acctday:" + tc.user, l.PerDay, 24 * time.Hour})
				}
				if got := rec.got(); !equalChecks(got, append(one, one...)) {
					t.Fatalf("checks = %+v, want %+v on each route", got, one)
				}
			})
		}
	}
}

// A key is never worse than no key: every keyed tier's minute sits at or above the budget an
// anonymous caller has on any route before strict. And the ladder climbs.
func TestDefaultRateTiers_AKeyIsNeverWorseThanNone(t *testing.T) {
	tiers := DefaultRateTiers()
	for _, tier := range []rateTier{rateFree, ratePro, rateUltra, rateAPI, rateData} {
		if tiers[tier].PerMinute < publicReadsPerMinute || tiers[tier].PerMinute < agentSearchPerMinute {
			t.Errorf("%s = %+v, below the keyless budget", tier, tiers[tier])
		}
	}
	for i := 1; i < len(rateTierSpecs); i++ {
		if rateTierSpecs[i].def.PerMinute <= rateTierSpecs[i-1].def.PerMinute {
			t.Errorf("%s does not climb above %s", rateTierSpecs[i].tier, rateTierSpecs[i-1].tier)
		}
	}
	if tiers[rateAPI] != (rateLimit{PerMinute: 1200}) || tiers[rateData] != (rateLimit{PerMinute: 6000}) {
		t.Errorf("api tiers = %+v / %+v, the plans on sale say 1200 and 6000 a minute with no daily cap", tiers[rateAPI], tiers[rateData])
	}
}

// The daily caps below the API product wait for strict: without it a keyless caller has none,
// and a key that carried one would be worse than no key for exactly the callers it is meant for.
func TestRateTiers_DailyCapsArriveWithStrict(t *testing.T) {
	def := DefaultRateTiers()
	before, after := def.inForce(false), def.inForce(true)
	for _, tier := range []rateTier{rateFree, ratePro, rateUltra} {
		if before[tier].PerDay != 0 || before[tier].PerMinute != def[tier].PerMinute {
			t.Errorf("%s before strict = %+v, want its minute and no daily cap", tier, before[tier])
		}
		if after[tier] != def[tier] {
			t.Errorf("%s under strict = %+v, want %+v", tier, after[tier], def[tier])
		}
	}
	for _, tier := range []rateTier{rateAnonymous, rateAPI, rateData} {
		if before[tier] != def[tier] {
			t.Errorf("%s before strict = %+v, want it untouched", tier, before[tier])
		}
	}
}

func TestPublicGate_UnresolvedOrEmbedKeysAreAnonymous(t *testing.T) {
	for _, token := range []string{embedToken, unknownToken, "fhk_not-a-real-shape", wellFormedAccessToken('z'), "fhm_short"} {
		t.Run(token, func(t *testing.T) {
			rec := &checkRecorder{}
			app := gateApp(newTestGate(rec, paidKeys(), true, ""))
			gateGet(t, app, "/read", bearer(token))
			got := rec.got()
			if len(got) == 0 || got[0].key != "anon:"+gateIP {
				t.Fatalf("checks = %+v, want the anonymous budget", got)
			}
		})
	}
}

func TestPublicGate_ResolvedTiersAreCachedFailuresAreNot(t *testing.T) {
	keys := paidKeys()
	app := gateApp(newTestGate(&checkRecorder{}, keys, true, ""))
	gateGet(t, app, "/read", bearer(apiKeyToken))
	gateGet(t, app, "/read", bearer(apiKeyToken))
	gateGet(t, app, "/read", bearer(unknownToken))
	gateGet(t, app, "/read", bearer(unknownToken))
	if keys.lookups != 2 {
		t.Fatalf("lookups = %d, want 2 (one per distinct key, negatives cached too)", keys.lookups)
	}

	broken := &fakeKeyTiers{err: errors.New("database is down")}
	rec := &checkRecorder{}
	app = gateApp(newTestGate(rec, broken, true, ""))
	if code, _ := gateGet(t, app, "/read", bearer(apiKeyToken)); code != fiber.StatusOK {
		t.Fatalf("a failed lookup answered %d, want the request served as anonymous", code)
	}
	gateGet(t, app, "/read", bearer(apiKeyToken))
	if broken.lookups != 2 {
		t.Fatalf("lookups = %d, want 2 — a failure must not be cached", broken.lookups)
	}
	if got := rec.got(); got[0].key != "anon:"+gateIP {
		t.Fatalf("checks = %+v, want anonymous", got)
	}
}

func TestPublicGate_AnonymousRequestsCostNoLookup(t *testing.T) {
	keys := paidKeys()
	app := gateApp(newTestGate(&checkRecorder{}, keys, true, ""))
	gateGet(t, app, "/read", nil)
	gateGet(t, app, "/read", map[string]string{"Authorization": "Bearer a.session.jwt"})
	if keys.lookups != 0 {
		t.Fatalf("lookups = %d, want 0", keys.lookups)
	}
}

func TestPublicGate_VerifiedBots(t *testing.T) {
	const secret = "s3cret"
	cases := []struct {
		name      string
		secret    string
		header    string
		wantCheck bool
	}{
		{"matching secret is exempt", secret, secret, false},
		{"forged value is anonymous", secret, "guess", true},
		{"no secret configured is anonymous", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := &checkRecorder{}
			app := gateApp(newTestGate(rec, paidKeys(), true, tc.secret))
			gateGet(t, app, "/read", map[string]string{verifiedBotHeader: tc.header})
			if got := len(rec.got()) > 0; got != tc.wantCheck {
				t.Fatalf("budget checked = %v, want %v", got, tc.wantCheck)
			}
		})
	}
}

// A refused caller is pointed one rung up the ladder from where they stand.
func TestPublicGate_RefusalNamesTheNextRung(t *testing.T) {
	cases := []struct {
		name    string
		headers map[string]string
		say     string
		want    string
	}{
		{"anonymous", nil, "Create a free account at freehire.me for a higher limit", "https://freehire.me/my/integrations/api-keys"},
		{"free key", bearer(unpaidToken), "Upgrade your account at freehire.me for a higher limit", "https://freehire.me/pricing"},
		{"pro", bearer(proToken), "See the API plans at freehire.me for a higher limit", "https://freehire.me/jobs-api"},
		{"api product", bearer(apiKeyToken), "See the API plans at freehire.me for a higher limit", "https://freehire.me/jobs-api"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := gateApp(newTestGate(&checkRecorder{refuse: true}, paidKeys(), true, ""))
			code, body := gateGet(t, app, "/read", tc.headers)
			if code != fiber.StatusTooManyRequests || !strings.Contains(body, tc.want) || !strings.Contains(body, tc.say) {
				t.Fatalf("status %d body %q, want 429 saying %q and naming %s", code, body, tc.say, tc.want)
			}
		})
	}
}

func TestPublicGate_StrictDayCapRefusesAfterTheMinutePasses(t *testing.T) {
	rec := &dayRefuser{}
	app := gateApp(newTestGate(rec, paidKeys(), true, ""))
	if code, _ := gateGet(t, app, "/read", nil); code != fiber.StatusTooManyRequests {
		t.Fatalf("status = %d, want 429 from the daily budget", code)
	}
}

// dayRefuser admits every per-minute check and refuses every daily one.
type dayRefuser struct{}

func (dayRefuser) Allow(_ context.Context, key string, limit int, window time.Duration) (ratelimit.Decision, error) {
	if strings.HasPrefix(key, "anonday:") {
		return ratelimit.Decision{Limit: limit, ResetAfter: window, RetryAfter: window}, nil
	}
	return ratelimit.Decision{Allowed: true, Limit: limit, Remaining: limit, ResetAfter: window}, nil
}

func equalChecks(a, b []check) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestAPILimitsViewReadsTheEnforcedBudgets(t *testing.T) {
	tiers := DefaultRateTiers()
	legacy, strict := apiLimitsFor(false, tiers.inForce(false)), apiLimitsFor(true, tiers.inForce(true))
	if legacy.AnonymousPerMinute != publicReadsPerMinute || legacy.AnonymousPerDay != 0 {
		t.Errorf("legacy anonymous = %+v", legacy)
	}
	if legacy.Tiers[0] != (rateTierView{Tier: rateAnonymous, PerMinute: publicReadsPerMinute}) {
		t.Errorf("legacy ladder starts at %+v, want the anonymous budget in force", legacy.Tiers[0])
	}
	if legacy.Tiers[1] != (rateTierView{Tier: rateFree, PerMinute: tiers[rateFree].PerMinute}) {
		t.Errorf("legacy free rung = %+v, want no daily cap before strict", legacy.Tiers[1])
	}
	anon := tiers[rateAnonymous]
	if strict.AnonymousPerMinute != anon.PerMinute || strict.AnonymousPerDay != anon.PerDay {
		t.Errorf("strict anonymous = %+v", strict)
	}
	if strict.APIPerMinute != tiers[rateAPI].PerMinute || strict.DataPerMinute != tiers[rateData].PerMinute {
		t.Errorf("paid = %+v", strict)
	}
	if len(strict.Tiers) != len(rateTierSpecs) {
		t.Fatalf("ladder = %+v, want every tier", strict.Tiers)
	}
	for i, s := range rateTierSpecs {
		if want := (rateTierView{Tier: s.tier, PerMinute: tiers[s.tier].PerMinute, PerDay: tiers[s.tier].PerDay}); strict.Tiers[i] != want {
			t.Errorf("rung %d = %+v, want %+v", i, strict.Tiers[i], want)
		}
	}
}

func TestAPIPlanFrom(t *testing.T) {
	tiers := DefaultRateTiers()
	future := ts(gateNow.Add(time.Hour))
	cases := []struct {
		name    string
		row     db.GetUserAPIEntitlementRow
		product billing.APITier
		rate    rateTier
	}{
		{"nothing", db.GetUserAPIEntitlementRow{}, billing.APITierNone, rateFree},
		{"pro", db.GetUserAPIEntitlementRow{ProUntil: future}, billing.APITierNone, ratePro},
		{"api beats ultra", db.GetUserAPIEntitlementRow{ApiUntilStripe: future, UltraUntil: future}, billing.APITierAPI, rateAPI},
		{"data", db.GetUserAPIEntitlementRow{ApiUntilStripe: future, ApiDataUntilStripe: future}, billing.APITierData, rateData},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := apiPlanFrom(tc.row, tiers, gateNow)
			if got.Tier != tc.product || got.RateTier != tc.rate ||
				got.PerMinute != tiers[tc.rate].PerMinute || got.PerDay != tiers[tc.rate].PerDay {
				t.Fatalf("got %+v, want product %q at rate %q", got, tc.product, tc.rate)
			}
			if (got.Until != nil) != (tc.product != billing.APITierNone) {
				t.Fatalf("until = %v for product %q", got.Until, tc.product)
			}
		})
	}
}

func TestPublicGate_AdmittedStrictRequestReportsTheMinuteBudget(t *testing.T) {
	app := gateApp(newTestGate(&checkRecorder{}, paidKeys(), true, ""))
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/read", nil)
	req.Header.Set(fiber.HeaderXForwardedFor, gateIP)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if want := DefaultRateTiers()[rateAnonymous].PerMinute; resp.Header.Get("X-RateLimit-Limit") != strconv.Itoa(want) {
		t.Fatalf("X-RateLimit-Limit = %q, want the minute budget %d", resp.Header.Get("X-RateLimit-Limit"), want)
	}
}

func TestPublicGate_MalformedKeysAndBotsCostNoLookup(t *testing.T) {
	keys := paidKeys()
	app := gateApp(newTestGate(&checkRecorder{}, keys, true, "s3cret"))
	gateGet(t, app, "/read", bearer("fhk_short"))
	gateGet(t, app, "/read", bearer(wellFormedKey('a')[:46]+"!"))
	h := bearer(apiKeyToken)
	h[verifiedBotHeader] = "s3cret"
	gateGet(t, app, "/read", h)
	if keys.lookups != 0 {
		t.Fatalf("lookups = %d, want 0", keys.lookups)
	}
}

// lookupRefuser refuses only the internal key-lookup budget.
type lookupRefuser struct{ checkRecorder }

func (l *lookupRefuser) Allow(ctx context.Context, key string, limit int, window time.Duration) (ratelimit.Decision, error) {
	if strings.HasPrefix(key, "keylookup:") {
		return ratelimit.Decision{Limit: limit, RetryAfter: window}, nil
	}
	return l.checkRecorder.Allow(ctx, key, limit, window)
}

func TestPublicGate_LookupBudgetBoundsDatabaseReads(t *testing.T) {
	keys := paidKeys()
	app := gateApp(newTestGate(&lookupRefuser{}, keys, true, ""))
	req := httptest.NewRequestWithContext(context.Background(), fiber.MethodGet, "/read", nil)
	req.Header.Set(fiber.HeaderXForwardedFor, gateIP)
	req.Header.Set("Authorization", "Bearer "+apiKeyToken)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if keys.lookups != 0 {
		t.Fatalf("lookups = %d, want 0 once the lookup budget is spent", keys.lookups)
	}
	if resp.StatusCode != fiber.StatusOK || resp.Header.Get("Retry-After") != "" {
		t.Fatalf("status %d Retry-After %q — an exhausted lookup budget must serve the request as anonymous, silently",
			resp.StatusCode, resp.Header.Get("Retry-After"))
	}
}

func TestPublicGate_TopTierRefusalCarriesNoUpsell(t *testing.T) {
	app := gateApp(newTestGate(&checkRecorder{refuse: true}, paidKeys(), true, ""))
	code, body := gateGet(t, app, "/read", bearer(dataKeyToken))
	if code != fiber.StatusTooManyRequests || strings.Contains(body, "freehire.me/") {
		t.Fatalf("status %d body %q — the top tier has nowhere to be sent", code, body)
	}
}

func TestPublicGate_ChatAgentBudgetStaysLegacyUnderStrict(t *testing.T) {
	rec := &checkRecorder{}
	g := newTestGate(rec, paidKeys(), true, "")
	app := fiber.New(fiber.Config{ProxyHeader: fiber.HeaderXForwardedFor})
	app.Get("/mcp", chatAgentLimiter(g), func(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusOK) })
	gateGet(t, app, "/mcp", nil)
	want := []check{{"agentsearch:" + gateIP, agentSearchPerMinute, time.Minute}}
	if got := rec.got(); !equalChecks(got, want) {
		t.Fatalf("checks = %+v, want %+v — OpenAI's shared egress must not meet a per-IP daily cap", got, want)
	}
}

// minuteRefuser refuses the anonymous per-minute budget and admits everything else, counting
// what the daily budget was charged.
type minuteRefuser struct {
	mu         sync.Mutex
	dayCharged int
}

func (m *minuteRefuser) Allow(_ context.Context, key string, limit int, window time.Duration) (ratelimit.Decision, error) {
	if strings.HasPrefix(key, "anon:") {
		return ratelimit.Decision{Limit: limit, ResetAfter: window, RetryAfter: time.Second}, nil
	}
	if strings.HasPrefix(key, "anonday:") {
		m.mu.Lock()
		m.dayCharged++
		m.mu.Unlock()
	}
	return ratelimit.Decision{Allowed: true, Limit: limit, Remaining: limit - 1, ResetAfter: window}, nil
}

// A request the minute refuses must not cost a day: a skill bursting past the minute cap
// would otherwise burn its whole daily allowance on refusals within minutes.
func TestPublicGate_AMinuteRefusalSpendsNoDay(t *testing.T) {
	m := &minuteRefuser{}
	app := gateApp(newTestGate(m, paidKeys(), true, ""))
	for range 5 {
		if code, _ := gateGet(t, app, "/read", nil); code != fiber.StatusTooManyRequests {
			t.Fatalf("status = %d, want 429 from the minute", code)
		}
	}
	if m.dayCharged != 0 {
		t.Fatalf("the daily budget was charged %d times for requests the minute refused", m.dayCharged)
	}
}
