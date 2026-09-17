package userjob

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestGateReplyRateBenchmark_BothSidesClearTheGate(t *testing.T) {
	you := ReplyRateSide{Applications: 15, Answered: 6}
	global := ReplyRateSide{Applications: 287, Answered: 97}

	got := GateReplyRateBenchmark(you, global)

	if got == nil {
		t.Fatal("got nil, want a benchmark — both sides clear the ten-application gate")
	}
	if got.You != you || got.Global != global {
		t.Errorf("got %+v, want You=%+v Global=%+v", got, you, global)
	}
}

func TestGateReplyRateBenchmark_CallerBelowOwnGate(t *testing.T) {
	you := ReplyRateSide{Applications: 4, Answered: 1}
	global := ReplyRateSide{Applications: 287, Answered: 97}

	if got := GateReplyRateBenchmark(you, global); got != nil {
		t.Errorf("got %+v, want nil — the caller has fewer than ten observable applications", got)
	}
}

func TestGateReplyRateBenchmark_GlobalBelowGate(t *testing.T) {
	you := ReplyRateSide{Applications: 15, Answered: 6}
	global := ReplyRateSide{Applications: 3, Answered: 1}

	if got := GateReplyRateBenchmark(you, global); got != nil {
		t.Errorf("got %+v, want nil — a personal rate with nothing to compare against is not a benchmark", got)
	}
}

// A caller with no connected mailbox never reaches GetUserResponseRate's "observable"
// predicate, so You.Applications is zero — the same gate that rejects a small sample
// also rejects this, with no separate "has a mailbox" check anywhere.
func TestGateReplyRateBenchmark_NoConnectedMailboxIsZeroObservable(t *testing.T) {
	you := ReplyRateSide{Applications: 0, Answered: 0}
	global := ReplyRateSide{Applications: 287, Answered: 97}

	if got := GateReplyRateBenchmark(you, global); got != nil {
		t.Errorf("got %+v, want nil", got)
	}
}

// A nil ReplyRate must be genuinely absent on the wire, never a null or zeroed object —
// the spec requires absence over an estimate, and a serialized null still tells a
// careless frontend "here is a value".
func TestPipeline_NilReplyRateIsOmittedFromJSON(t *testing.T) {
	p := Pipeline{Applications: 3, Stages: map[string]int64{"applied": 3}}

	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if strings.Contains(string(b), "reply_rate") {
		t.Errorf("got %s, want no reply_rate key when ReplyRate is nil", b)
	}
}

func TestPipeline_SetReplyRateIsSerialized(t *testing.T) {
	p := Pipeline{
		Applications: 3,
		Stages:       map[string]int64{"applied": 3},
		ReplyRate: &ReplyRateBenchmark{
			You:    ReplyRateSide{Applications: 12, Answered: 4},
			Global: ReplyRateSide{Applications: 287, Answered: 97},
		},
	}

	b, err := json.Marshal(p)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	if !strings.Contains(string(b), `"reply_rate"`) {
		t.Errorf("got %s, want a reply_rate key when ReplyRate is set", b)
	}
}

func TestExcludeCallerFromGlobal_SubtractsTheCallersOwnContribution(t *testing.T) {
	you := ReplyRateSide{Applications: 12, Answered: 4}
	globalTotal := ReplyRateSide{Applications: 287, Answered: 97}

	got := ExcludeCallerFromGlobal(you, globalTotal)

	want := ReplyRateSide{Applications: 275, Answered: 93}
	if got != want {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

// The global figure is a periodic rollup (cmd/rollup-company) while `you` is read live,
// so a caller's very recent application may count on their own live side before the next
// rollup has folded it into the total it is subtracted from. A negative count is nonsense,
// not a signal, so each side clamps at zero independently rather than surfacing it.
func TestExcludeCallerFromGlobal_ClampsAtZeroWhenTheRollupIsStale(t *testing.T) {
	you := ReplyRateSide{Applications: 5, Answered: 3}
	globalTotal := ReplyRateSide{Applications: 4, Answered: 1}

	got := ExcludeCallerFromGlobal(you, globalTotal)

	want := ReplyRateSide{Applications: 0, Answered: 0}
	if got != want {
		t.Errorf("got %+v, want %+v — neither field may go negative", got, want)
	}
}

func TestGateReplyRateBenchmark_ExactlyAtTheGate(t *testing.T) {
	you := ReplyRateSide{Applications: ObservableSampleGate, Answered: 2}
	global := ReplyRateSide{Applications: ObservableSampleGate, Answered: 4}

	if got := GateReplyRateBenchmark(you, global); got == nil {
		t.Error("got nil, want a benchmark — the gate is inclusive (\"at least ten\")")
	}
}
