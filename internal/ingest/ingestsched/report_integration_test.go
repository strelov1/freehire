//go:build integration

package ingestsched

import (
	"context"
	"testing"
	"time"
)

func ptrInt(v int) *int                     { return &v }
func ptrDur(v time.Duration) *time.Duration { return &v }
func ptrStr(v string) *string               { return &v }
func ptrBool(v bool) *bool                  { return &v }

func reportFor(t *testing.T, repo *QueriesRepository, provider string) ProviderReport {
	t.Helper()
	rows, err := repo.Report(context.Background())
	if err != nil {
		t.Fatalf("Report: %v", err)
	}
	for _, r := range rows {
		if r.Provider == provider {
			return r
		}
	}
	t.Fatalf("provider %q absent from the report", provider)
	return ProviderReport{}
}

// The report must draw the distinction the spec asks for: running on defaults versus
// configured. It is the difference between "nobody has looked at this" and "somebody
// decided this".
func TestReportMarksDefaultedAndOverriddenProviders(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	addBoard(t, pool, "greenhouse", "acme")
	addBoard(t, pool, "paylocity", "globex")

	if err := repo.SaveOverride(ctx, OverrideInput{
		Provider: "paylocity", Shards: ptrInt(24), RunTimeout: ptrDur(4500 * time.Second),
		Notes: ptrStr("~10.42s/board measured"),
	}); err != nil {
		t.Fatalf("SaveOverride: %v", err)
	}

	if got := reportFor(t, repo, "greenhouse"); got.Overridden {
		t.Error("greenhouse reads as overridden; it has no row")
	}
	got := reportFor(t, repo, "paylocity")
	if !got.Overridden {
		t.Error("paylocity reads as defaulted; it has a row")
	}
	if got.Shards != 24 || got.RunTimeout != 4500*time.Second {
		t.Errorf("paylocity = %d shards / %v, want 24 / 4500s", got.Shards, got.RunTimeout)
	}
	// Cadence was not given, so the column default applies rather than a reset to
	// something the caller never asked for.
	if got.Cadence != DefaultCadence {
		t.Errorf("Cadence = %v, want the column default", got.Cadence)
	}
}

// A partial edit must not disturb what it did not name. This is the failure mode of a
// naive UPSERT: raising the shard count would silently reset a cadence someone measured.
func TestSaveOverrideLeavesUnnamedFieldsAlone(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	addBoard(t, pool, "paylocity", "acme")

	if err := repo.SaveOverride(ctx, OverrideInput{
		Provider: "paylocity", Shards: ptrInt(24), Cadence: ptrDur(24 * time.Hour),
		RunTimeout: ptrDur(4500 * time.Second), Notes: ptrStr("measured"),
	}); err != nil {
		t.Fatalf("first SaveOverride: %v", err)
	}
	// The second edit names only the shard count.
	if err := repo.SaveOverride(ctx, OverrideInput{Provider: "paylocity", Shards: ptrInt(12)}); err != nil {
		t.Fatalf("second SaveOverride: %v", err)
	}

	got := reportFor(t, repo, "paylocity")
	if got.Shards != 12 {
		t.Errorf("Shards = %d, want 12", got.Shards)
	}
	if got.Cadence != 24*time.Hour {
		t.Errorf("Cadence = %v, want the untouched 24h", got.Cadence)
	}
	if got.RunTimeout != 4500*time.Second {
		t.Errorf("RunTimeout = %v, want the untouched 4500s", got.RunTimeout)
	}
	if got.Notes != "measured" {
		t.Errorf("Notes = %q, want the untouched measurement", got.Notes)
	}
}

// The schema refuses a disable with no reason, and the repository must surface that rather
// than swallow it. psql is a writer too, which is why the rule lives in the table.
func TestSaveOverrideCannotDisableWithoutAReason(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	addBoard(t, pool, "bayt", "acme")

	if err := repo.SaveOverride(ctx, OverrideInput{
		Provider: "bayt", Enabled: ptrBool(false),
	}); err == nil {
		t.Fatal("disabling with no reason was accepted")
	}

	if err := repo.SaveOverride(ctx, OverrideInput{
		Provider: "bayt", Enabled: ptrBool(false),
		DisabledReason: ptrStr("fingerprint client has no proxy support"),
	}); err != nil {
		t.Fatalf("disabling with a reason: %v", err)
	}
	got := reportFor(t, repo, "bayt")
	if got.Enabled {
		t.Error("bayt still reads as enabled")
	}
	if got.DisabledReason == "" {
		t.Error("the reason is missing from the report, which is where an operator reads it")
	}
}

// A shard-count change that has not been reconciled yet is exactly the drift this change
// exists to remove, so the report must show BOTH numbers rather than the intended one.
func TestReportShowsRunStateSeparatelyFromTheIntendedShardCount(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	addBoard(t, pool, "workday", "acme")

	if err := repo.SaveOverride(ctx, OverrideInput{Provider: "workday", Shards: ptrInt(6)}); err != nil {
		t.Fatalf("SaveOverride: %v", err)
	}
	if got := reportFor(t, repo, "workday"); got.ShardsInState != 0 {
		t.Errorf("ShardsInState = %d before any reconcile, want 0", got.ShardsInState)
	}

	settings := Effective("workday", &Override{
		Provider: "workday", Shards: 6, Cadence: time.Hour, RunTimeout: DefaultRunTimeout, Enabled: true,
	})
	if _, err := repo.Reconcile(ctx, []Settings{settings}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	got := reportFor(t, repo, "workday")
	if got.ShardsInState != 6 {
		t.Errorf("ShardsInState = %d after reconcile, want 6", got.ShardsInState)
	}
	if got.NextDueAt == nil {
		t.Error("NextDueAt is nil; the report cannot answer 'is anything overdue'")
	}
}

// Everything an operator needs to answer "did this provider actually run" must survive the
// round trip, or the report is a picture of intentions rather than of the fleet.
func TestReportCarriesTheLastRunOutcome(t *testing.T) {
	repo, pool := newRepo(t)
	ctx := context.Background()
	addBoard(t, pool, "greenhouse", "acme")
	manage(t, pool, "greenhouse")
	if _, err := repo.Reconcile(ctx, []Settings{Effective("greenhouse", nil)}); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}

	if _, err := repo.Claim(ctx, 1, time.Minute); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if got := reportFor(t, repo, "greenhouse"); got.InFlight != 1 {
		t.Errorf("InFlight = %d while claimed, want 1", got.InFlight)
	}

	if err := repo.RecordFinish(ctx, "greenhouse", 1, 0, ""); err != nil {
		t.Fatalf("RecordFinish: %v", err)
	}
	got := reportFor(t, repo, "greenhouse")
	if got.InFlight != 0 {
		t.Errorf("InFlight = %d after the run finished, want 0", got.InFlight)
	}
	if got.LastFinishedAt == nil {
		t.Error("LastFinishedAt is nil after a finished run")
	}
}
