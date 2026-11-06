package plan

import (
	"testing"
	"time"
)

func TestDecideSupportIsFlatForEveryTier(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	for _, tier := range []Tier{TierFree, TierPro, TierUltra} {
		d := decideSupport(2)(tier, FeatureSupport, 1, false, now)
		if !d.Allowed || d.Charge != 1 || d.Used != 2 {
			t.Errorf("%s below the limit: %+v, want allowed and charged one", tier, d)
		}
		d = decideSupport(2)(tier, FeatureSupport, 2, false, now)
		if d.Allowed || d.Charge != 0 {
			t.Errorf("%s at the limit: %+v, want refused", tier, d)
		}
	}
}

func TestDecideSupportLetsAChargedReferenceThroughFree(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	d := decideSupport(2)(TierFree, FeatureSupport, 2, true, now)
	if !d.Allowed || d.Charge != 0 {
		t.Errorf("a reference already paid for: %+v, want allowed at no charge", d)
	}
}

func TestSupportIsNotAPlanFeature(t *testing.T) {
	for _, f := range AllFeatures() {
		if f == FeatureSupport {
			t.Fatal("support is in AllFeatures, so every plan walk would treat it as a metered paid feature")
		}
	}
}

func TestSupportConfigDefaultsAndEnv(t *testing.T) {
	cfg := ConfigFromEnv()
	if cfg.SupportMessagesPerDay != 20 || cfg.SupportTurnsPerSession != 12 || cfg.SupportMessagesGlobalPerDay != 2000 {
		t.Errorf("defaults = %d/%d/%d, want 20/12/2000",
			cfg.SupportMessagesPerDay, cfg.SupportTurnsPerSession, cfg.SupportMessagesGlobalPerDay)
	}

	t.Setenv("SUPPORT_DAILY_MESSAGES", "5")
	t.Setenv("SUPPORT_TURNS_PER_SESSION", "4")
	t.Setenv("SUPPORT_DAILY_MESSAGES_GLOBAL", "100")
	cfg = ConfigFromEnv()
	if cfg.SupportMessagesPerDay != 5 || cfg.SupportTurnsPerSession != 4 || cfg.SupportMessagesGlobalPerDay != 100 {
		t.Errorf("overrides = %d/%d/%d, want 5/4/100",
			cfg.SupportMessagesPerDay, cfg.SupportTurnsPerSession, cfg.SupportMessagesGlobalPerDay)
	}

	t.Setenv("SUPPORT_DAILY_MESSAGES", "0")
	t.Setenv("SUPPORT_TURNS_PER_SESSION", "nope")
	cfg = ConfigFromEnv()
	if cfg.SupportMessagesPerDay != 20 || cfg.SupportTurnsPerSession != 12 {
		t.Errorf("a non-positive or unparseable value changed the default: %d/%d",
			cfg.SupportMessagesPerDay, cfg.SupportTurnsPerSession)
	}
}
