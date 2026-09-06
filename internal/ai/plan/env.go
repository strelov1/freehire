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
// PLAN_FREE_DAILY_<FEATURE>, PLAN_PRO_DAILY_<FEATURE>, PLAN_ULTRA_DAILY_<FEATURE> and
// PLAN_TAILOR_TURNS_PER_SESSION move the numbers themselves. They exist because the shipped
// values are a reading of three August weeks under no paywall, and the shadow run is
// expected to contradict them. PLAN_PRO_DAILY_AUTO_APPLY earns its keep twice over: that
// one number is a ceiling under a plan people had already bought, so it has to move without
// a deploy on the day somebody complains.
//
// A tier's figure is its ceiling where it has one and its fair-use guard where it does not,
// which is why one variable per tier moves both: they are the same number in the same field.
// PLAN_FAIR_USE_<FEATURE> was the older spelling of the pro one and is gone — two names for
// one field is a question every reader has to answer twice, and nothing had ever set it.
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
		suffix := envSuffix(f)
		if n, ok := envPositive("PLAN_FREE_DAILY_" + suffix); ok {
			fc.free.daily = n
		}
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
	return cfg
}

// envSuffix is the variable-name half of a feature: upper case, with the dash a name may
// carry written as an underscore.
//
// The dash is not cosmetic. "cover-letter" and "auto-apply" are persisted in the ledger and
// matched by the idempotency index, so those values cannot change — but systemd's
// EnvironmentFile= will not accept a variable whose name holds a dash, so a suffix taken
// verbatim asks for PLAN_PRO_DAILY_AUTO-APPLY, which is unsettable on the host it has to be
// set on. Reading the underscored spelling is what makes the documented lever exist for the
// two features that need it most: auto-apply is the one feature that ships enforcing, and
// pro's ceiling on it is what the tier above is sold on.
func envSuffix(f Feature) string {
	return strings.ToUpper(strings.ReplaceAll(string(f), "-", "_"))
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
