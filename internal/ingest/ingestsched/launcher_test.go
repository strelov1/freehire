package ingestsched

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"
)

func newTestLauncher(rec *recordedExec) SystemdLauncher {
	return SystemdLauncher{
		IngestBinary: "/opt/freehire/src/hire-current/ingest",
		WorkingDir:   "/opt/freehire/src/hire-current",
		EnvFile:      "/opt/freehire/.env",
		RunAs:        "freehire",
		exec:         rec.run,
	}
}

type recordedExec struct {
	name string
	args []string
	err  error
}

func (r *recordedExec) run(_ context.Context, name string, args ...string) error {
	r.name = name
	r.args = args
	return r.err
}

// argValue reads the value of a --flag=value argument, so a test asserts the VALUE rather
// than the exact position of a flag in a list that will grow.
func argValue(args []string, flag string) (string, bool) {
	for _, a := range args {
		if after, ok := strings.CutPrefix(a, flag+"="); ok {
			return after, true
		}
	}
	return "", false
}

// The per-provider timeout is the whole reason runs are transient units rather than one
// static template: a template can carry only one value, and this fleet needs 3000s for most
// providers and 4500s for the per-posting-detail ones.
func TestLaunchCarriesTheRunsOwnTimeout(t *testing.T) {
	rec := &recordedExec{}
	l := newTestLauncher(rec)

	err := l.Launch(context.Background(), Run{
		Provider: "paylocity", Shard: 3, Shards: 24, RunTimeout: 4500 * time.Second,
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}

	if rec.name != "systemd-run" {
		t.Errorf("exec %q, want systemd-run", rec.name)
	}
	if got, ok := argValue(rec.args, "--property=TimeoutStartSec"); !ok || got != "4500" {
		t.Errorf("TimeoutStartSec = %q (found=%v), want 4500", got, ok)
	}
}

func TestLaunchNamesTheUnitAfterTheProviderAndShard(t *testing.T) {
	rec := &recordedExec{}
	l := newTestLauncher(rec)

	if err := l.Launch(context.Background(), Run{
		Provider: "habr_career", Shard: 1, Shards: 1, RunTimeout: DefaultRunTimeout,
	}); err != nil {
		t.Fatalf("Launch: %v", err)
	}

	got, ok := argValue(rec.args, "--unit")
	if !ok {
		t.Fatalf("no --unit in %v", rec.args)
	}
	// The unit name is derived from the same string that selects the boards. That is the
	// habr_career fix in structural form: there is no second spelling to drift.
	if !strings.Contains(got, "habr_career") {
		t.Errorf("--unit = %q, want it to carry the provider key verbatim", got)
	}

	// Deliberately NOT --collect. systemd already collects a SUCCESSFUL transient unit, and
	// that absence is how Finished reads success; collecting failures too would erase the
	// exit code before anyone could read it, making "succeeded" and "failed" one answer.
	if slices.Contains(rec.args, "--collect") {
		t.Error("--collect would erase a failed run's exit code before it could be read")
	}
}

// An unsharded provider must be launched with no shard selector at all, not with a
// hand-written 1/1: cmd/ingest treats a missing selector as "crawl everything", and the two
// spellings would be one more thing that can disagree.
func TestLaunchOmitsTheShardSelectorForAnUnshardedProvider(t *testing.T) {
	rec := &recordedExec{}
	l := newTestLauncher(rec)

	if err := l.Launch(context.Background(), Run{
		Provider: "greenhouse", Shard: 1, Shards: 1, RunTimeout: DefaultRunTimeout,
	}); err != nil {
		t.Fatalf("Launch: %v", err)
	}

	for _, a := range rec.args {
		if strings.HasPrefix(a, "--shard=") {
			t.Errorf("unsharded run carries %q; want no shard selector", a)
		}
	}
	if rec.args[len(rec.args)-1] != "greenhouse" {
		t.Errorf("last argument = %q, want the bare provider key", rec.args[len(rec.args)-1])
	}
}

func TestLaunchPassesTheShardSelectorForAShardedProvider(t *testing.T) {
	rec := &recordedExec{}
	l := newTestLauncher(rec)

	if err := l.Launch(context.Background(), Run{
		Provider: "workday", Shard: 4, Shards: 6, RunTimeout: DefaultRunTimeout,
	}); err != nil {
		t.Fatalf("Launch: %v", err)
	}

	if last := rec.args[len(rec.args)-1]; last != "--shard=4/6" {
		t.Errorf("last argument = %q, want --shard=4/6", last)
	}
}

// The scheduler is privileged so that it may create transient units; the crawl itself must
// not be. Dropping to the service account is a property of the launch, not of the host's
// goodwill.
func TestLaunchDropsToTheUnprivilegedServiceAccount(t *testing.T) {
	rec := &recordedExec{}
	l := newTestLauncher(rec)

	if err := l.Launch(context.Background(), Run{
		Provider: "greenhouse", Shard: 1, Shards: 1, RunTimeout: DefaultRunTimeout,
	}); err != nil {
		t.Fatalf("Launch: %v", err)
	}

	if got, ok := argValue(rec.args, "--uid"); !ok || got != "freehire" {
		t.Errorf("--uid = %q (found=%v), want freehire", got, ok)
	}
	if got, ok := argValue(rec.args, "--property=EnvironmentFile"); !ok || got != "/opt/freehire/.env" {
		t.Errorf("EnvironmentFile = %q (found=%v)", got, ok)
	}
}

// The exit code comes from a string systemd hands back, is parsed with strconv.Atoi, and
// ends up in an int32 column. Unbounded, a value past int32 wraps on the way to the
// database and records a status the run never had — CodeQL flags exactly this shape, and
// it is right to. A process status is 0-255; anything else is systemd telling us something
// other than an exit code, so it is clamped and the raw text kept.
func TestExitStatusIsBoundedToAProcessStatus(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want int
	}{
		{"0", 0},
		{"1", 1},
		{"255", 255},
		{"", 0},
		{"not-a-number", 0},
		{"-1", 0},
		{"256", 255},
		{"9223372036854775807", 255},
		{"99999999999999999999999", 255}, // past int64 as well, so Atoi itself errors
	} {
		if got := exitStatus(tc.raw); got != tc.want {
			t.Errorf("exitStatus(%q) = %d, want %d", tc.raw, got, tc.want)
		}
	}
}

// The gate is not advisory. A key that ValidateProviderKey refuses must never become an
// argv element or a unit name, and the launcher is the last place that can still be true.
func TestLaunchRefusesAnUnsafeOrUnknownProviderKey(t *testing.T) {
	for _, provider := range []string{
		"greenhouse;rm -rf /",
		"greenhouse lever",
		"habrcareer", // well-shaped, but no adapter answers to it
	} {
		rec := &recordedExec{}
		l := newTestLauncher(rec)

		err := l.Launch(context.Background(), Run{
			Provider: provider, Shard: 1, Shards: 1, RunTimeout: DefaultRunTimeout,
		})
		if err == nil {
			t.Errorf("Launch(%q) = nil, want a refusal", provider)
		}
		if rec.name != "" {
			t.Errorf("Launch(%q) executed %q %v; nothing must run", provider, rec.name, rec.args)
		}
	}
}
