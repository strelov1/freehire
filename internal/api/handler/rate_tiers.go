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
// A key is never worse than no key: the keyed tiers' minute sits at or above the anonymous
// budget a route had before PUBLIC_ANON_LIMITS=strict (600), so holding one pays the day it is
// minted, and the same figures stay right once strict lowers the anonymous tier. The daily caps
// are what stop a mirror; the API tiers carry none because a mirror is what they are sold to.
// The anonymous figures apply only under strict (see publicGate.enforce).
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
func RateTiersFromEnv(getenv func(string) string) (RateTiers, error) {
	t := DefaultRateTiers()
	for _, s := range rateTierSpecs {
		l := t[s.tier]
		minuteVar, dayVar := "API_RATE_"+s.env+"_PER_MINUTE", "API_RATE_"+s.env+"_PER_DAY"
		if raw := strings.TrimSpace(getenv(minuteVar)); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n <= 0 {
				return nil, fmt.Errorf("%s must be a positive integer, got %q", minuteVar, raw)
			}
			l.PerMinute = n
		}
		if raw := strings.TrimSpace(getenv(dayVar)); raw != "" {
			n, err := strconv.Atoi(raw)
			if err != nil || n < 0 {
				return nil, fmt.Errorf("%s must be a non-negative integer (0 for no daily cap), got %q", dayVar, raw)
			}
			l.PerDay = n
		}
		t[s.tier] = l
	}
	return t, nil
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
