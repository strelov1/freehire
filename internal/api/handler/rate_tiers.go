package handler

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/identity/billing"
)

// rateTier is which public-read budget a caller draws on. It is not a plan: the anonymous tier
// is an address, not an account, and the two API tiers are a product sold apart from Pro and
// Ultra. It is the ladder a caller climbs to read more of the catalogue.
type rateTier string

const (
	rateAnonymous rateTier = "anonymous"
	rateFree      rateTier = "free"
	ratePro       rateTier = "pro"
	rateUltra     rateTier = "ultra"
	rateAPI       rateTier = "api"
	rateData      rateTier = "api_data"
)

// rateLimit is one tier's budget. PerDay 0 is no daily cap, which is what the API product sells.
type rateLimit struct {
	PerMinute int
	PerDay    int
}

// rateTierSpecs is the ladder, lowest first, with each tier's environment name and the figure
// it ships with.
//
// A key is never worse than no key on any route: the keyed tiers' minute sits at or above the
// largest per-route budget a keyless caller had before PUBLIC_ANON_LIMITS=strict (600), so
// holding one pays the day it is minted, and the same figures stay right once strict lowers the
// anonymous tier. The daily caps are what stop a mirror, and they wait for strict like the
// anonymous one does (inForce): a key with a cap beside an address without one is a key a
// mirror deletes. The API tiers carry none because a mirror is what they are sold to.
var rateTierSpecs = []struct {
	tier rateTier
	env  string
	def  rateLimit
}{
	{rateAnonymous, "ANONYMOUS", rateLimit{PerMinute: 120, PerDay: 2_000}},
	{rateFree, "FREE", rateLimit{PerMinute: 600, PerDay: 10_000}},
	{ratePro, "PRO", rateLimit{PerMinute: 800, PerDay: 30_000}},
	{rateUltra, "ULTRA", rateLimit{PerMinute: 1_000, PerDay: 100_000}},
	{rateAPI, "API", rateLimit{PerMinute: 1_200}},
	{rateData, "DATA", rateLimit{PerMinute: 6_000}},
}

// RateTiers is every tier's budget, as the server enforces it and /plans publishes it.
type RateTiers map[rateTier]rateLimit

// inForce is the ladder as it applies under the given anonymous mode. Without strict a keyless
// caller has no daily cap, so neither does a keyed tier below the API product: the caps arrive
// together, the day the anonymous one does. The anonymous rung itself is not consulted then —
// the gate keeps the legacy per-route budgets — so it is left as configured.
func (t RateTiers) inForce(strict bool) RateTiers {
	out := RateTiers{}
	for tier, l := range t {
		if !strict && (tier == rateFree || tier == ratePro || tier == rateUltra) {
			l.PerDay = 0
		}
		out[tier] = l
	}
	return out
}

// DefaultRateTiers is the ladder as it ships.
func DefaultRateTiers() RateTiers {
	t := RateTiers{}
	for _, s := range rateTierSpecs {
		t[s.tier] = s.def
	}
	return t
}

// RateTiersFromEnv is DefaultRateTiers with API_RATE_<TIER>_PER_MINUTE and
// API_RATE_<TIER>_PER_DAY applied, so a figure moves without a deploy.
//
// A value that cannot be read fails the boot rather than falling back. The default is not a
// safe answer to a typo here: an operator who set a figure would watch the server enforce a
// different one with nothing saying so. A minute must be positive; a day of 0 lifts the cap.
//
// So does a ladder that makes a key worse than no key under the anonymous mode it will run
// in (strict is PUBLIC_ANON_LIMITS=strict): `API_RATE_FREE_PER_MINUTE=60` is far likelier a
// dropped zero than a decision, and the server would otherwise enforce it in silence.
func RateTiersFromEnv(getenv func(string) string, strict bool) (RateTiers, error) {
	t := DefaultRateTiers()
	for _, s := range rateTierSpecs {
		l := t[s.tier]
		if err := envOverride(getenv, "API_RATE_"+s.env+"_PER_MINUTE", 1, &l.PerMinute); err != nil {
			return nil, err
		}
		if err := envOverride(getenv, "API_RATE_"+s.env+"_PER_DAY", 0, &l.PerDay); err != nil {
			return nil, err
		}
		t[s.tier] = l
	}
	if err := t.keyNeverWorse(strict); err != nil {
		return nil, err
	}
	return t, nil
}

// keyNeverWorse refuses a ladder that steps down anywhere, in force: no keyed tier may give
// less than a caller with no key gets (the larger per-route minute before strict, the
// anonymous tier under it), and no tier may give less than the one below it — an upgrade that
// shrinks an allowance is the same mistake one rung up. A day of 0 is no cap, so it outranks
// every number.
func (t RateTiers) keyNeverWorse(strict bool) error {
	prev, prevName := rateLimit{PerMinute: max(publicReadsPerMinute, agentSearchPerMinute)}, "no key"
	if strict {
		prev = t[rateAnonymous]
	}
	inForce := t.inForce(strict)
	for _, s := range rateTierSpecs {
		if s.tier == rateAnonymous {
			continue
		}
		l := inForce[s.tier]
		if l.PerMinute < prev.PerMinute {
			return fmt.Errorf("API_RATE_%s_PER_MINUTE is %d, below the %d a minute of %s: a key must not be worse than none, nor a tier worse than the one below it",
				s.env, l.PerMinute, prev.PerMinute, prevName)
		}
		if dayBelow(l.PerDay, prev.PerDay) {
			return fmt.Errorf("API_RATE_%s_PER_DAY is %d, below the daily allowance of %s (%s): a key must not be worse than none, nor a tier worse than the one below it",
				s.env, l.PerDay, prevName, dayString(prev.PerDay))
		}
		prev, prevName = l, string(s.tier)
	}
	return nil
}

// dayBelow reports whether a daily cap gives less than another, a cap of 0 being none.
func dayBelow(day, than int) bool {
	switch {
	case day == 0:
		return false
	case than == 0:
		return true
	default:
		return day < than
	}
}

func dayString(day int) string {
	if day == 0 {
		return "no cap"
	}
	return strconv.Itoa(day)
}

// envOverride sets *dst from the variable when it is set, refusing a value below floor. Not
// worker.EnvInt64: that reader takes positive integers only, and a day of 0 is a real setting
// here — no daily cap.
func envOverride(getenv func(string) string, name string, floor int, dst *int) error {
	raw := strings.TrimSpace(getenv(name))
	if raw == "" {
		return nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < floor {
		return fmt.Errorf("%s must be an integer of at least %d, got %q", name, floor, raw)
	}
	*dst = n
	return nil
}

// accountRateTier is the tier an account's credential earns: the API product first, since it
// is the one sold for reading, then the better of Pro and Ultra, then free.
func accountRateTier(apiUntil, dataUntil, proUntil, ultraUntil, now time.Time) rateTier {
	switch billing.APITierOf(apiUntil, dataUntil, now) {
	case billing.APITierData:
		return rateData
	case billing.APITierAPI:
		return rateAPI
	}
	switch plan.TierOf(proUntil, ultraUntil, now) {
	case plan.TierUltra:
		return rateUltra
	case plan.TierPro:
		return ratePro
	}
	return rateFree
}
