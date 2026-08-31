package plan

import "testing"

func TestConfigFromEnvDefaultsToTheShippedConfig(t *testing.T) {
	cfg := ConfigFromEnv()

	if cfg.FreeDaily(FeatureFit) != DefaultConfig().FreeDaily(FeatureFit) {
		t.Errorf("an unset environment changed the free allowance")
	}
	for _, f := range AllFeatures() {
		if cfg.Enforced(f) {
			t.Errorf("%q enforces with nothing set; enforcement is opt-in per feature", f)
		}
	}
}

func TestEnforcementIsNamedPerFeature(t *testing.T) {
	t.Setenv("PLAN_ENFORCE", "match, dictation")
	cfg := ConfigFromEnv()

	if !cfg.Enforced(FeatureFit) || !cfg.Enforced(FeatureDictation) {
		t.Error("a named feature does not enforce")
	}
	if cfg.Enforced(FeatureTailor) || cfg.Enforced(FeatureAssistant) {
		t.Error("an unnamed feature enforces; the switch must turn on exactly what it names")
	}
}

func TestEnforceAllIsSpelledOut(t *testing.T) {
	t.Setenv("PLAN_ENFORCE", "all")
	cfg := ConfigFromEnv()

	for _, f := range AllFeatures() {
		if !cfg.Enforced(f) {
			t.Errorf("%q does not enforce under 'all'", f)
		}
	}
}

func TestAnUnknownFeatureNameIsIgnoredNotGuessed(t *testing.T) {
	// A typo must not silently enforce something else, and must not stop the features
	// that were spelled correctly from taking effect.
	t.Setenv("PLAN_ENFORCE", "mach,tailor")
	cfg := ConfigFromEnv()

	if !cfg.Enforced(FeatureTailor) {
		t.Error("a correctly named feature was lost because another name was misspelled")
	}
	if cfg.Enforced(FeatureFit) {
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
