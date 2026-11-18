package plan

import (
	"log"
	"os"
	"strconv"
	"strings"
)

// ConfigFromEnv is DefaultConfig with the environment's overrides applied.
//
// PLAN_ENFORCE lists the features whose refusal is switched on ("match,tailor", or "all"), so
// they can be turned on one at a time.
//
// PLAN_PRO_DAILY_<FEATURE>, PLAN_ULTRA_DAILY_<FEATURE> and PLAN_TAILOR_TURNS_PER_SESSION move
// the numbers without a deploy. One variable per tier moves both the ceiling and the fair-use
// guard, which share a field.
//
// There is no PLAN_FREE_DAILY_<FEATURE>: free gets no AI, and a lever that can only weaken that
// is one to avoid. An override that cannot be read keeps the default and logs, because a typo
// resolving to zero would refuse every subscriber.
func ConfigFromEnv() Config {
	cfg := DefaultConfig()

	enforced := enforcedFeatures(os.Getenv("PLAN_ENFORCE"))
	for f, fc := range cfg.features {
		if enforced[f] {
			fc.enforce = true
		}
		suffix := envSuffix(f)
		if n, ok := envPositive("PLAN_PRO_DAILY_" + suffix); ok {
			fc.pro.daily = n
		}
		if n, ok := envPositive("PLAN_ULTRA_DAILY_" + suffix); ok {
			fc.ultra.daily = n
		}
		cfg.features[f] = fc
	}
	if n, ok := envPositive("PLAN_TAILOR_TURNS_PER_SESSION"); ok {
		cfg.TailorTurnsPerSession = n
	}
	if n, ok := envPositive("SUPPORT_DAILY_MESSAGES"); ok {
		cfg.SupportMessagesPerDay = n
	}
	if n, ok := envPositive("SUPPORT_TURNS_PER_SESSION"); ok {
		cfg.SupportTurnsPerSession = n
	}
	if n, ok := envPositive("SUPPORT_DAILY_MESSAGES_GLOBAL"); ok {
		cfg.SupportMessagesGlobalPerDay = n
	}
	return cfg
}

// envSuffix is the variable-name half of a feature: upper case, dash written as underscore.
// systemd's EnvironmentFile= rejects dashes in names, so "auto-apply" would otherwise be
// unsettable as PLAN_PRO_DAILY_AUTO-APPLY.
func envSuffix(f Feature) string {
	return strings.ToUpper(strings.ReplaceAll(string(f), "-", "_"))
}

// enforcedFeatures reads the enforcement list. An unknown name is logged and ignored, not
// guessed at: "mach" must not turn on "match".
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
