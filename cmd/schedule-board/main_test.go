package main

import (
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ingest/ingestsched"
)

// A nil field means "leave this alone". A curator raising the shard count must not
// silently reset a cadence somebody measured, which is the whole reason the edit is
// partial rather than a full row.
func TestEditTouchesOnlyTheFlagsThatWereGiven(t *testing.T) {
	in, err := edit("paylocity", editFlags{shards: 24})
	if err != nil {
		t.Fatalf("edit: %v", err)
	}

	if in.Shards == nil || *in.Shards != 24 {
		t.Errorf("Shards = %v, want 24", in.Shards)
	}
	if in.Cadence != nil {
		t.Errorf("Cadence = %v; an unspecified flag must leave the stored value alone", *in.Cadence)
	}
	if in.RunTimeout != nil {
		t.Errorf("RunTimeout = %v; want untouched", *in.RunTimeout)
	}
	if in.Enabled != nil {
		t.Errorf("Enabled = %v; want untouched", *in.Enabled)
	}
	if in.Managed != nil {
		t.Errorf("Managed = %v; want untouched", *in.Managed)
	}
}

// The reason is required at the CLI as well as in the schema. Both, because the schema is
// what stops a hand-written psql UPDATE and the CLI is what tells the person typing it
// why, before the round trip.
func TestEditRefusesADisableWithNoReason(t *testing.T) {
	for _, f := range []editFlags{
		{disable: true},
		{disable: true, reason: "   "},
	} {
		if _, err := edit("bayt", f); err == nil {
			t.Errorf("edit(%+v) = nil, want a refusal", f)
		}
	}

	in, err := edit("bayt", editFlags{disable: true, reason: "fingerprint client has no proxy"})
	if err != nil {
		t.Fatalf("disable with a reason: %v", err)
	}
	if in.Enabled == nil || *in.Enabled {
		t.Errorf("Enabled = %v, want false", in.Enabled)
	}
	if in.DisabledReason == nil || *in.DisabledReason == "" {
		t.Error("the reason did not reach the write")
	}
}

// The table bounds this too, but a curator deserves a sentence rather than a constraint
// name. An unbounded shard count is a typo away from a generate_series that outlives the
// scheduler's start timeout on every tick, which stops the whole fleet.
func TestEditRefusesAnAbsurdShardCount(t *testing.T) {
	if _, err := edit("paylocity", editFlags{shards: maxShards + 1}); err == nil {
		t.Errorf("--shards=%d was accepted", maxShards+1)
	}
	if _, err := edit("paylocity", editFlags{shards: 100000}); err == nil {
		t.Error("--shards=100000 was accepted")
	}
	// The largest real value must still go through.
	if _, err := edit("paylocity", editFlags{shards: 24}); err != nil {
		t.Errorf("--shards=24 (paylocity's real value) was refused: %v", err)
	}
}

func TestEditRefusesContradictoryFlags(t *testing.T) {
	if _, err := edit("greenhouse", editFlags{disable: true, enable: true, reason: "x"}); err == nil {
		t.Error("--disable --enable together were accepted")
	}
	if _, err := edit("greenhouse", editFlags{manage: true, unmanage: true}); err == nil {
		t.Error("--manage --unmanage together were accepted")
	}
}

// A typo written into the table would otherwise sit there, reported as refused by every
// scheduler tick, long after the person who typed it has moved on.
func TestEditRefusesAProviderKeyTheRegistryDoesNotKnow(t *testing.T) {
	_, err := edit("habrcareer", editFlags{shards: 2})
	if !errors.Is(err, ingestsched.ErrUnknownProvider) {
		t.Errorf("edit(habrcareer) = %v, want ErrUnknownProvider", err)
	}

	if _, err := edit("greenhouse;rm -rf /", editFlags{shards: 2}); !errors.Is(err, ingestsched.ErrUnsafeProviderKey) {
		t.Errorf("edit(unsafe key) = %v, want ErrUnsafeProviderKey", err)
	}
}

func TestEditCarriesCadenceTimeoutAndNotes(t *testing.T) {
	in, err := edit("reed", editFlags{
		cadence: 6 * time.Hour,
		timeout: 4500 * time.Second,
		notes:   "per-hour API request quota; a full crawl blows past it",
	})
	if err != nil {
		t.Fatalf("edit: %v", err)
	}

	if in.Cadence == nil || *in.Cadence != 6*time.Hour {
		t.Errorf("Cadence = %v, want 6h", in.Cadence)
	}
	if in.RunTimeout == nil || *in.RunTimeout != 4500*time.Second {
		t.Errorf("RunTimeout = %v, want 4500s", in.RunTimeout)
	}
	if in.Notes == nil || *in.Notes == "" {
		t.Error("Notes were dropped; the measurement is why the number is what it is")
	}
}

func TestEditFlipsTheRolloutGateBothWays(t *testing.T) {
	on, err := edit("greenhouse", editFlags{manage: true})
	if err != nil {
		t.Fatalf("edit --manage: %v", err)
	}
	if on.Managed == nil || !*on.Managed {
		t.Errorf("Managed = %v, want true", on.Managed)
	}

	off, err := edit("greenhouse", editFlags{unmanage: true})
	if err != nil {
		t.Fatalf("edit --unmanage: %v", err)
	}
	if off.Managed == nil || *off.Managed {
		t.Errorf("Managed = %v, want false — rollback is the reverse of the cutover step", off.Managed)
	}
}
