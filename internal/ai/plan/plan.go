// Package plan answers one question for the metered AI features: may this user do this
// now. Everything behind that question — which plan they are on, what it allows per day,
// how much they have already used, and whether a refusal is switched on yet — is this
// package's business and nobody else's. A caller names a feature and a reference; it gets
// yes or no back.
//
// The unit is a calendar day in UTC, and the reset is lazy: a new day is a different key,
// so nothing expires anything and no scheduled job can forget to run.
//
// It replaces internal/ai/credits, which counted a currency. The measurement that ended
// that design is in the add-plan-limits change: the assistant was 99.4% of model spend
// and debited nothing, while tailoring debited three points once per session and nothing
// for the 54 turns one account then ran inside it.
package plan

import "time"

// Tier is the plan a user is on. There are two, and there is no third for staff — an
// exemption that exists only in code is one nobody can see when the numbers look wrong.
type Tier string

const (
	TierFree Tier = "free"
	TierPro  Tier = "pro"
)

// Feature identifies a metered action. The value is persisted in the ledger and matched
// by the idempotency index, so it must stay stable.
//
// FeatureAssistant covers the chat and profile presets together, deliberately: they are
// the same conversation surface pointed at different things, and a per-preset allowance
// would only teach a user which name to type. Tailoring is NOT one of them — it is metered
// by its own two bounds (a daily session count and a per-session turn ceiling), and
// charging it here as well would let the daily assistant allowance decide how deep one CV
// may be edited.
type Feature string

const (
	FeatureTailor    Feature = "tailor"
	FeatureFit       Feature = "match"
	FeatureAssistant Feature = "assistant"
	FeatureDictation Feature = "dictation"
	// FeatureCoverLetter is one drafted cover letter: three model calls over a fit analysis
	// and a tailoring context that are already computed and cached by the time it runs. It is
	// the cheapest metered feature per action, which is why its daily figures sit above the
	// tailoring session's and below the assistant's.
	FeatureCoverLetter Feature = "cover-letter"
)

// AllFeatures lists every metered feature. Tests walk it, and the usage surface reports
// each one, so a feature added to the config and forgotten here is a feature the user
// cannot see.
func AllFeatures() []Feature {
	return []Feature{FeatureTailor, FeatureFit, FeatureAssistant, FeatureDictation, FeatureCoverLetter}
}

// TierOf resolves a plan from the one column that decides it. A zero or past pro_until is
// free; a future one is pro. The comparison is strict, so a subscription is over at the
// instant it expires — any grace beyond that is a product decision and belongs where
// somebody can see it, not in this comparison.
func TierOf(proUntil, now time.Time) Tier {
	if proUntil.After(now) {
		return TierPro
	}
	return TierFree
}

// Day is the UTC calendar day an allowance is keyed by. Always UTC, whatever the caller's
// clock says: the counter row is keyed this way, and reading it in local time would put a
// user's own sense of "today" out of step with the row that bounds them.
func Day(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// ResetsAt is the instant the current day's allowances lapse — midnight UTC on the next
// day. Reported to the user, so a refusal says when to come back rather than just no.
func ResetsAt(t time.Time) time.Time {
	return Day(t).AddDate(0, 0, 1)
}

// Allowance is what one plan permits for one feature in a day. A pro-plan allowance is
// Unlimited with Limit carrying the fair-use guard behind it: the guard is real and it
// refuses, but it is not a plan limit and is never shown as one.
type Allowance struct {
	Limit     int
	Unlimited bool
}

// featureConfig is one feature's economics on both plans.
type featureConfig struct {
	freeDaily  int
	proFairUse int
	enforce    bool
}

// Config is the whole product decision in one place: which features are metered, how much
// each plan allows, and whether a refusal is switched on. Call sites name a feature and
// nothing else, which is what keeps five handlers from drifting apart about what a limit is.
type Config struct {
	features map[Feature]featureConfig

	// TailorTurnsPerSession bounds how deep one tailoring session goes, separately from
	// how many sessions a day may be started. Both bounds are needed and they stop
	// different things: measured on production the median session ran 2.7 turns and one
	// ran 54, so a session count alone leaves that session unbounded.
	TailorTurnsPerSession int
}

// DefaultConfig is the configuration the change ships with.
//
// The free numbers sit a little above the 90th percentile of measured behaviour, so an
// ordinary day never meets a wall and a deliberate evening of editing does. The fair-use
// guards are set roughly twenty times higher: they exist to stop a script from draining
// the gateway on one subscription, not to bound a person.
//
// Enforcement is OFF for every feature. These numbers come from 101 users over 23 days
// under no paywall at all, and turning them on before the shadow run has been read means
// refusing real people on a number nobody checked.
func DefaultConfig() Config {
	return Config{
		features: map[Feature]featureConfig{
			FeatureTailor:    {freeDaily: 2, proFairUse: 40, enforce: false},
			FeatureFit:       {freeDaily: 3, proFairUse: 60, enforce: false},
			FeatureAssistant: {freeDaily: 10, proFairUse: 200, enforce: false},
			FeatureDictation: {freeDaily: 10, proFairUse: 200, enforce: false},
			// Ships with enforcement OFF like every other feature: the shadow run is read
			// first, and a ceiling set before there is any usage to read is a guess.
			FeatureCoverLetter: {freeDaily: 3, proFairUse: 60, enforce: false},
		},
		TailorTurnsPerSession: 15,
	}
}

// with returns a copy of this configuration, having applied edit to each feature. Every
// variation copies rather than mutating, because a Config holds a map and two callers
// sharing one would otherwise reconfigure each other's.
func (c Config) with(edit func(Feature, *featureConfig)) Config {
	out := Config{
		features:              make(map[Feature]featureConfig, len(c.features)),
		TailorTurnsPerSession: c.TailorTurnsPerSession,
	}
	for f, fc := range c.features {
		edit(f, &fc)
		out.features[f] = fc
	}
	return out
}

// Enforcing returns this configuration with every feature's refusal switched on — the
// state the shadow run ends in, and the one a test asserting a refusal has to be in.
func (c Config) Enforcing() Config {
	return c.with(func(_ Feature, fc *featureConfig) { fc.enforce = true })
}

// WithFreeDaily returns this configuration with one feature's free allowance set to n.
func (c Config) WithFreeDaily(f Feature, n int) Config {
	return c.with(func(k Feature, fc *featureConfig) {
		if k == f {
			fc.freeDaily = n
		}
	})
}

// FreeDaily is the free plan's daily allowance for a feature. An unconfigured feature
// allows nothing rather than everything: a surface somebody forgot to configure should
// show up as refused in a shadow run, not as free in a bill.
func (c Config) FreeDaily(f Feature) int { return c.features[f].freeDaily }

// ProFairUse is the guard above which even a pro account is refused for the rest of the day.
func (c Config) ProFairUse(f Feature) int { return c.features[f].proFairUse }

// Enforced reports whether a refusal for this feature actually refuses. False is shadow
// mode: the consumption is recorded and reported, and the caller is allowed through.
func (c Config) Enforced(f Feature) bool { return c.features[f].enforce }

// Allowance is what the given plan permits for the feature in a day.
func (c Config) Allowance(tier Tier, f Feature) Allowance {
	cfg, ok := c.features[f]
	if !ok {
		return Allowance{}
	}
	if tier == TierPro {
		return Allowance{Limit: cfg.proFairUse, Unlimited: true}
	}
	return Allowance{Limit: cfg.freeDaily}
}
