package plan

import (
	"regexp"
	"testing"
)

func TestConfigFromEnvDefaultsToTheShippedConfig(t *testing.T) {
	cfg := ConfigFromEnv()

	if cfg.FreeDaily(FeatureFit) != DefaultConfig().FreeDaily(FeatureFit) {
		t.Errorf("an unset environment changed the free allowance")
	}
	for _, f := range AllFeatures() {
		if !cfg.Enforced(f) {
			t.Errorf("%q does not enforce with nothing set; every metered AI feature ships enforcing", f)
		}
	}
}

// enforcedFeatures is the pure function PLAN_ENFORCE's per-name behaviour actually lives
// in. It is tested directly rather than through ConfigFromEnv().Enforced(), because every
// feature now enforces by default — a naming test built on that combination could no
// longer tell "named" from "already on regardless".
func TestEnforcementIsNamedPerFeature(t *testing.T) {
	got := enforcedFeatures("match, dictation")

	if !got[FeatureFit] || !got[FeatureDictation] {
		t.Error("a named feature does not enforce")
	}
	if got[FeatureTailor] || got[FeatureAssistant] {
		t.Error("an unnamed feature enforces; the switch must turn on exactly what it names")
	}
}

func TestEnforceAllIsSpelledOut(t *testing.T) {
	got := enforcedFeatures("all")

	for _, f := range AllFeatures() {
		if !got[f] {
			t.Errorf("%q does not enforce under 'all'", f)
		}
	}
}

func TestAnUnknownFeatureNameIsIgnoredNotGuessed(t *testing.T) {
	// A typo must not silently enforce something else, and must not stop the features
	// that were spelled correctly from taking effect.
	got := enforcedFeatures("mach,tailor")

	if !got[FeatureTailor] {
		t.Error("a correctly named feature was lost because another name was misspelled")
	}
	if got[FeatureFit] {
		t.Error("a misspelled name was resolved to a feature by guessing")
	}
}

func TestFreeAllowanceOverride(t *testing.T) {
	t.Setenv("PLAN_FREE_DAILY_MATCH", "7")
	cfg := ConfigFromEnv()

	if got := cfg.FreeDaily(FeatureFit); got != 7 {
		t.Errorf("free daily allowance for match = %d, want 7", got)
	}
	if got := cfg.FreeDaily(FeatureTailor); got != DefaultConfig().FreeDaily(FeatureTailor) {
		t.Errorf("overriding one feature changed another (tailor = %d)", got)
	}
}

func TestADashedFeatureIsOverriddenByItsUnderscoredName(t *testing.T) {
	// The two features whose ledger value carries a dash. A suffix taken verbatim from it
	// asks for PLAN_PRO_DAILY_AUTO-APPLY, a name systemd's EnvironmentFile= will not parse,
	// so the documented lever would be unsettable on the host it exists for — and this one
	// is a ceiling under a plan people have already bought.
	t.Setenv("PLAN_PRO_DAILY_AUTO_APPLY", "9")
	t.Setenv("PLAN_FREE_DAILY_COVER_LETTER", "5")
	t.Setenv("PLAN_ULTRA_DAILY_AUTO_APPLY", "250")
	cfg := ConfigFromEnv()

	if got := cfg.Allowance(TierPro, FeatureAutoApply).Limit; got != 9 {
		t.Errorf("pro daily allowance for auto-apply = %d, want 9", got)
	}
	if got := cfg.Allowance(TierUltra, FeatureAutoApply).Limit; got != 250 {
		t.Errorf("ultra daily allowance for auto-apply = %d, want 250", got)
	}
	if got := cfg.FreeDaily(FeatureCoverLetter); got != 5 {
		t.Errorf("free daily allowance for cover-letter = %d, want 5", got)
	}
}

func TestEveryFeatureHasASettableOverrideName(t *testing.T) {
	// The guard for the next feature named with a dash: an override an operator cannot set
	// is indistinguishable from one that was ignored.
	settable := regexp.MustCompile(`^[A-Z0-9_]+$`)
	for _, f := range AllFeatures() {
		if suffix := envSuffix(f); !settable.MatchString(suffix) {
			t.Errorf("%q overrides PLAN_*_DAILY_%s, which is not a legal environment "+
				"variable name", f, suffix)
		}
	}
}

func TestAnUnreadableOverrideKeepsTheDefault(t *testing.T) {
	// A typo in a number must not silently mean zero. Zero would refuse the feature for
	// everybody on the free plan, which looks exactly like a deliberate decision.
	t.Setenv("PLAN_FREE_DAILY_MATCH", "three")
	if got, want := ConfigFromEnv().FreeDaily(FeatureFit), DefaultConfig().FreeDaily(FeatureFit); got != want {
		t.Errorf("an unparseable override left %d, want the default %d", got, want)
	}

	t.Setenv("PLAN_FREE_DAILY_MATCH", "-2")
	if got, want := ConfigFromEnv().FreeDaily(FeatureFit), DefaultConfig().FreeDaily(FeatureFit); got != want {
		t.Errorf("a negative override left %d, want the default %d", got, want)
	}
}

func TestTurnCeilingOverride(t *testing.T) {
	t.Setenv("PLAN_TAILOR_TURNS_PER_SESSION", "25")
	if got := ConfigFromEnv().TailorTurnsPerSession; got != 25 {
		t.Errorf("turn ceiling = %d, want 25", got)
	}
}
