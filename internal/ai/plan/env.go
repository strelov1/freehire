package plan

import (
	"log"
	"os"
	"strconv"
	"strings"
)

// ConfigFromEnv is DefaultConfig with the environment's overrides applied.
//
// Two things are worth turning without a deploy, and they are not the same thing.
//
// PLAN_ENFORCE names the features whose refusal is switched ON — "match,tailor", or "all".
// It is the end of the shadow run, feature by feature, and it is spelled as a list rather
// than a boolean because the features are turned on one at a time, cheapest first.
//
// PLAN_FREE_DAILY_<FEATURE> and PLAN_TAILOR_TURNS_PER_SESSION move the numbers themselves.
// They exist because the shipped values are a reading of three August weeks under no
// paywall, and the shadow run is expected to contradict them.
//
// An override that cannot be read keeps the default and says so in the log. The alternative
// — a typo resolving to zero — would refuse the feature to every free account and look
// exactly like a deliberate decision.
func ConfigFromEnv() Config {
	cfg := DefaultConfig()

	enforced := enforcedFeatures(os.Getenv("PLAN_ENFORCE"))
	for f, fc := range cfg.features {
		if enforced[f] {
			fc.enforce = true
		}
		if n, ok := envPositive("PLAN_FREE_DAILY_" + strings.ToUpper(string(f))); ok {
			fc.freeDaily = n
		}
		if n, ok := envPositive("PLAN_FAIR_USE_" + strings.ToUpper(string(f))); ok {
			fc.proFairUse = n
		}
		cfg.features[f] = fc
	}
	if n, ok := envPositive("PLAN_TAILOR_TURNS_PER_SESSION"); ok {
		cfg.TailorTurnsPerSession = n
	}
	return cfg
}

// enforcedFeatures reads the enforcement list. A name that matches no feature is ignored
// with a log line rather than guessed at: resolving "mach" to "match" would be a switch
// nobody typed, and silently dropping it would leave an operator sure they had turned
// something on.
func enforcedFeatures(list string) map[Feature]bool {
	out := map[Feature]bool{}
	for _, raw := range strings.Split(list, ",") {
		name := strings.TrimSpace(raw)
		if name == "" {
			continue
		}
		if name == "all" {
			for _, f := range AllFeatures() {
				out[f] = true
			}
			continue
		}
		known := false
		for _, f := range AllFeatures() {
			if string(f) == name {
				out[f], known = true, true
			}
		}
		if !known {
			log.Printf("plan: PLAN_ENFORCE names %q, which is not a metered feature — ignored", name)
		}
	}
	return out
}

// envPositive reads a positive integer override, reporting whether one was usable.
func envPositive(key string) (int, bool) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return 0, false
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 {
		log.Printf("plan: %s=%q is not a positive number — keeping the default", key, raw)
		return 0, false
	}
	return n, true
}
