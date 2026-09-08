//go:build integration

// Integration test for SettingsDrift against a real engine: that a freshly-ensured
// index reports no drift, and that a real gap (an index that has never seen
// EnsureIndex or a rebuild) is actually visible through the live GetSettings call
// rather than only through the pure diffSettings unit tests.
package search

import (
	"context"
	"testing"
)

func TestIntegration_SettingsDriftIsEmptyAfterEnsureIndex(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)

	if err := c.EnsureIndex(ctx); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}
	if err := c.ensure(ctx, c.manager.Index(companyIndexUID), companyIndexUID, companyPrimaryKey, companySettings()); err != nil {
		t.Fatalf("ensure companies: %v", err)
	}

	got, err := c.SettingsDrift(ctx)
	if err != nil {
		t.Fatalf("SettingsDrift: %v", err)
	}
	if got != nil {
		t.Errorf("SettingsDrift() = %v, want nil right after both indexes were ensured", got)
	}
}

func TestIntegration_SettingsDriftReportsAFreshIndexAsMissingEverything(t *testing.T) {
	ctx := context.Background()
	c := startMeili(t)

	// A bare index, created but never given this package's settings — the state a
	// brand-new deployment's index is in before its first EnsureIndex/rebuild.
	if err := c.createIndex(ctx, c.facet, facetIndexUID, primaryKey); err != nil {
		t.Fatalf("createIndex: %v", err)
	}
	if err := c.createIndex(ctx, c.manager.Index(companyIndexUID), companyIndexUID, companyPrimaryKey); err != nil {
		t.Fatalf("createIndex companies: %v", err)
	}

	got, err := c.SettingsDrift(ctx)
	if err != nil {
		t.Fatalf("SettingsDrift: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("SettingsDrift() reported no drift for an index that was never given this package's settings")
	}
}
