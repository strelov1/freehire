// Package plan answers one question for the metered AI features: may this user do this now.
// A caller names a feature and a reference and gets yes or no back.
//
// The unit is a calendar day in UTC. The reset is lazy: a new day is a different key, so nothing
// expires and no scheduled job is needed.
package plan

import "time"

// Tier is the plan a user is on. There is no staff tier: an exemption that exists only in code
// is invisible when the numbers look wrong.
type Tier string

const (
	TierFree Tier = "free"
	TierPro  Tier = "pro"
	// TierUltra is priced on auto-apply, the only action that drives a browser and submits to a
	// third party on the user's behalf.
	TierUltra Tier = "ultra"
)

// Paid reports whether this tier was bought. Use it instead of == TierPro so new paid tiers
// are included.
func (t Tier) Paid() bool { return t == TierPro || t == TierUltra }

// Feature identifies a metered action. The value is persisted in the ledger and matched by
// the idempotency index, so it must stay stable.
//
// FeatureAssistant covers the chat and profile presets together. Tailoring is not one of them:
// it has its own two bounds, and charging it here too would let the daily assistant allowance
// limit how deep one CV may be edited.
type Feature string

const (
	FeatureTailor    Feature = "tailor"
	FeatureFit       Feature = "match"
	FeatureAssistant Feature = "assistant"
	FeatureDictation Feature = "dictation"
	// FeatureCoverLetter is one drafted cover letter: three model calls over an already cached
	// fit analysis and tailoring context.
	FeatureCoverLetter Feature = "cover-letter"
	// FeatureAutoApply is one unattended application: a headless browser fills and submits the
	// employer's form. Its pro ceiling is a real daily count rather than a fair-use guard.
	FeatureAutoApply Feature = "auto-apply"
)

// AllFeatures lists every metered feature. Tests walk it and the usage surface reports each
// one, so a feature missing here is invisible to the user.
func AllFeatures() []Feature {
	return []Feature{
		FeatureTailor, FeatureFit, FeatureAssistant, FeatureDictation, FeatureCoverLetter,
		FeatureAutoApply,
	}
}

// TierOf resolves a plan from the columns that decide it: the better tier still live wins.
// Ultra wins even when the pro entitlement lasts longer (both can be live during an upgrade),
// so spending more never lowers the tier. An entitlement ends at the instant it expires; any
// grace is a product decision that belongs elsewhere.
func TierOf(proUntil, ultraUntil, now time.Time) Tier {
	if ultraUntil.After(now) {
		return TierUltra
	}
	if proUntil.After(now) {
		return TierPro
	}
	return TierFree
}

// Day is the UTC calendar day an allowance is keyed by, whatever the caller's clock says.
func Day(t time.Time) time.Time {
	u := t.UTC()
	return time.Date(u.Year(), u.Month(), u.Day(), 0, 0, 0, 0, time.UTC)
}

// ResetsAt is when the current day's allowances lapse: midnight UTC. Reported so a refusal
// says when to come back.
func ResetsAt(t time.Time) time.Time {
	return Day(t).AddDate(0, 0, 1)
}

// Allowance is what one plan permits for one feature in a day. When Unlimited, Limit carries
// the fair-use guard: it refuses, but is never shown as a plan limit.
type Allowance struct {
	Limit     int
	Unlimited bool
}

// tierAllowance is what one tier may do with one feature in a day. `daily` is a real ceiling,
// or the fair-use guard when `unlimited`. The zero value allows nothing.
type tierAllowance struct {
	daily     int
	unlimited bool
}

// featureConfig is one feature's economics on every plan.
//
// There is no free allowance: free gets no metered feature (see decide), so a field for it
// could only be pulled the wrong way.
type featureConfig struct {
	pro     tierAllowance
	ultra   tierAllowance
	enforce bool
}

// of returns this feature's allowance for one tier. Free and unknown tiers allow nothing.
func (fc featureConfig) of(tier Tier) tierAllowance {
	switch tier {
	case TierPro:
		return fc.pro
	case TierUltra:
		return fc.ultra
	}
	return tierAllowance{}
}

// Config is the whole product decision in one place: which features are metered, what each
// plan allows, and whether a refusal is switched on.
type Config struct {
	features map[Feature]featureConfig

	// TailorTurnsPerSession bounds how deep one tailoring session goes, separately from how many
	// sessions a day may start; a session count alone leaves one session unbounded.
	TailorTurnsPerSession int

	// SupportMessagesPerDay, SupportTurnsPerSession and SupportMessagesGlobalPerDay bound the
	// support agent, which every account may talk to. See support.go.
	SupportMessagesPerDay       int
	SupportTurnsPerSession      int
	SupportMessagesGlobalPerDay int
}

// DefaultConfig is the shipped configuration.
//
// Fair-use guards sit far above human behaviour: they stop a script draining the gateway on one
// subscription, not a person. Every metered feature is subscription-only and enforcing, and
// `decide` refuses an unpaid tier before it reads any allowance, so there is no free number.
func DefaultConfig() Config {
	return Config{
		features: map[Feature]featureConfig{
			FeatureTailor: {
				pro:     tierAllowance{daily: 40, unlimited: true},
				ultra:   tierAllowance{daily: 120, unlimited: true},
				enforce: true,
			},
			FeatureFit: {
				pro:     tierAllowance{daily: 60, unlimited: true},
				ultra:   tierAllowance{daily: 180, unlimited: true},
				enforce: true,
			},
			FeatureAssistant: {
				pro:     tierAllowance{daily: 200, unlimited: true},
				ultra:   tierAllowance{daily: 600, unlimited: true},
				enforce: true,
			},
			FeatureDictation: {
				pro:     tierAllowance{daily: 200, unlimited: true},
				ultra:   tierAllowance{daily: 600, unlimited: true},
				enforce: true,
			},
			FeatureCoverLetter: {
				pro:     tierAllowance{daily: 60, unlimited: true},
				ultra:   tierAllowance{daily: 180, unlimited: true},
				enforce: true,
			},
			// Pro carries a real three a day rather than a fair-use guard: that ceiling is what Ultra is
			// sold on. Ultra's figure guards the host (each attempt is a browser) and is unmeasured.
			FeatureAutoApply: {
				pro:     tierAllowance{daily: 3},
				ultra:   tierAllowance{daily: 100, unlimited: true},
				enforce: true,
			},
		},
		TailorTurnsPerSession: 15,

		SupportMessagesPerDay:       20,
		SupportTurnsPerSession:      12,
		SupportMessagesGlobalPerDay: 2000,
	}
}

// with returns a copy of this configuration with edit applied to each feature. It copies
// because Config holds a map that callers must not share.
func (c Config) with(edit func(Feature, *featureConfig)) Config {
	out := Config{
		features:              make(map[Feature]featureConfig, len(c.features)),
		TailorTurnsPerSession: c.TailorTurnsPerSession,

		SupportMessagesPerDay:       c.SupportMessagesPerDay,
		SupportTurnsPerSession:      c.SupportTurnsPerSession,
		SupportMessagesGlobalPerDay: c.SupportMessagesGlobalPerDay,
	}
	for f, fc := range c.features {
		edit(f, &fc)
		out.features[f] = fc
	}
	return out
}

// Enforcing returns this configuration with every feature's refusal switched on, for tests
// asserting a refusal.
func (c Config) Enforcing() Config {
	return c.with(func(_ Feature, fc *featureConfig) { fc.enforce = true })
}

// WithProCeiling returns this configuration with one feature's pro allowance turned into a
// real daily ceiling of n. For tests only: free gets nothing, so a paid tier is the only way to
// get an allowance that can be consumed partway.
func (c Config) WithProCeiling(f Feature, n int) Config {
	return c.with(func(k Feature, fc *featureConfig) {
		if k == f {
			fc.pro = tierAllowance{daily: n}
		}
	})
}

// Enforced reports whether a refusal for this feature actually refuses. False is shadow mode:
// consumption is recorded and the caller goes through.
func (c Config) Enforced(f Feature) bool { return c.features[f].enforce }

// Allowance is what the given plan permits for the feature in a day. When unlimited, Limit
// carries the fair-use guard, which lets `decide` apply it to every unlimited tier.
func (c Config) Allowance(tier Tier, f Feature) Allowance {
	cfg, ok := c.features[f]
	if !ok {
		return Allowance{}
	}
	a := cfg.of(tier)
	return Allowance{Limit: a.daily, Unlimited: a.unlimited}
}
