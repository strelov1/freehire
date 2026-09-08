package handler

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// The tally's two sources are deliberate: rounds and tool outcomes are visible in the
// stream a client sees, while token counts are not — the runner emits one usage event per
// TURN, on its terminal path only, so a thirty-round autopilot reports the last call's
// tokens and nothing else. The bake-off substitutes the turn model anyway, so it counts
// tokens at that seam instead of asking the runner to change.
func TestBakeoffTallyCountsRoundsAndTokensFromTheirOwnSources(t *testing.T) {
	var tally bakeoffTally

	tally.observeCall(&llm.Usage{Input: 1000, Output: 50, CachedInput: 800}, 300*time.Millisecond)
	tally.observeEvent(assistant.Event{Kind: assistant.EventToolUse, Name: "cv_context"})
	tally.observeEvent(assistant.Event{Kind: assistant.EventToolResult, Result: `{"ok":true}`})
	tally.observeCall(&llm.Usage{Input: 1400, Output: 20, CachedInput: 1200}, 120*time.Millisecond)
	tally.observeEvent(assistant.Event{Kind: assistant.EventResult, StopReason: assistant.StopEndTurn})

	if tally.Rounds != 2 {
		t.Errorf("Rounds = %d, want 2", tally.Rounds)
	}
	if tally.Input != 2400 || tally.Output != 70 || tally.CachedInput != 2000 {
		t.Errorf("tokens = %d/%d cached %d, want 2400/70/2000", tally.Input, tally.Output, tally.CachedInput)
	}
	if tally.ToolCalls != 1 {
		t.Errorf("ToolCalls = %d, want 1", tally.ToolCalls)
	}
	if tally.Stop != assistant.StopEndTurn {
		t.Errorf("Stop = %q, want %q", tally.Stop, assistant.StopEndTurn)
	}
}

// The first round's latency is the one that matters: it is what a reader waits through
// before anything appears, and the reason a slow prefill reads as a hung turn.
func TestBakeoffTallyKeepsTheFirstRoundsLatency(t *testing.T) {
	var tally bakeoffTally
	tally.observeCall(&llm.Usage{Input: 1}, 900*time.Millisecond)
	tally.observeCall(&llm.Usage{Input: 1}, 10*time.Millisecond)

	if tally.FirstRound != 900*time.Millisecond {
		t.Errorf("FirstRound = %v, want 900ms", tally.FirstRound)
	}
}

// A provider that reports no counts at all is a round that happened and cost something
// unknown — counted as a round, and recorded as unmeasured rather than as free.
func TestBakeoffTallyCountsARoundWhoseProviderReportedNothing(t *testing.T) {
	var tally bakeoffTally
	tally.observeCall(nil, 50*time.Millisecond)

	if tally.Rounds != 1 {
		t.Errorf("Rounds = %d, want 1", tally.Rounds)
	}
	if tally.RoundsWithoutUsage != 1 {
		t.Errorf("RoundsWithoutUsage = %d, want 1", tally.RoundsWithoutUsage)
	}
	if tally.Input != 0 {
		t.Errorf("Input = %d, want 0 — nothing was reported to add", tally.Input)
	}
}

// The three ways a tool call fails are not one number. Arguments that did not decode and
// a tool name the model invented are the MODEL's failures and belong in its score; a
// service that fell over is ours, and counting it against the model would rank whichever
// candidate happened to run while the database was busy.
func TestBakeoffTallySeparatesTheModelsToolFailuresFromOurs(t *testing.T) {
	var tally bakeoffTally
	tally.observeEvent(assistant.Event{Kind: assistant.EventToolResult, IsError: true,
		Result: `{"error":"invalid arguments: json: unknown field \"titel\""}`})
	tally.observeEvent(assistant.Event{Kind: assistant.EventToolResult, IsError: true,
		Result: `{"error":"unknown tool \"search_vacancies\" — call one of the tools you were given"}`})
	tally.observeEvent(assistant.Event{Kind: assistant.EventToolResult, IsError: true,
		Result: `{"error":"reading the experience bank: connection refused"}`})

	if tally.BadArguments != 1 {
		t.Errorf("BadArguments = %d, want 1", tally.BadArguments)
	}
	if tally.UnknownTools != 1 {
		t.Errorf("UnknownTools = %d, want 1", tally.UnknownTools)
	}
	if tally.ServiceErrors != 1 {
		t.Errorf("ServiceErrors = %d, want 1", tally.ServiceErrors)
	}
}

// A successful tool result is not a failure however much its payload looks like one. The
// stream flags an error explicitly, and reading the text instead would count a vacancy
// whose description contains the word "error" against the model.
func TestBakeoffTallyReadsTheErrorFlagAndNotTheText(t *testing.T) {
	var tally bakeoffTally
	tally.observeEvent(assistant.Event{Kind: assistant.EventToolResult,
		Result: `{"jobs":[{"title":"Error Budget Engineer"}]}`})

	if tally.BadArguments+tally.UnknownTools+tally.ServiceErrors != 0 {
		t.Errorf("a successful result was counted as a failure: %+v", tally)
	}
}

// A run that served part of its replayed prefix from cache says so, with the share, which
// is the number that decides what the model actually costs here.
func TestCacheVerdictReportsTheShareWhenTheCacheWasUsed(t *testing.T) {
	v := (&bakeoffTally{Rounds: 4, Input: 10_000, CachedInput: 8_000}).cacheVerdict()

	if v.State != cacheObserved {
		t.Errorf("State = %q, want %q", v.State, cacheObserved)
	}
	if v.Share != 0.8 {
		t.Errorf("Share = %v, want 0.8", v.Share)
	}
}

// Several rounds against a prefix that should have been warm, and not one cached token, is
// evidence about the provider — stated as the inference it is, because langchaingo cannot
// tell "reported nothing" from "hit nothing" on any single call.
func TestCacheVerdictCallsItUnobservedAcrossSeveralRounds(t *testing.T) {
	v := (&bakeoffTally{Rounds: 6, Input: 60_000}).cacheVerdict()

	if v.State != cacheNotObserved {
		t.Errorf("State = %q, want %q", v.State, cacheNotObserved)
	}
	if v.Share != 0 {
		t.Errorf("Share = %v, want 0", v.Share)
	}
}

// One round has no prefix behind it to have been cached, so a zero there is not evidence
// of anything. Reporting it as "no cache observed" would convict a provider on a run that
// never gave it the chance.
func TestCacheVerdictIsInconclusiveOnASingleRound(t *testing.T) {
	if v := (&bakeoffTally{Rounds: 1, Input: 3_000}).cacheVerdict(); v.State != cacheInconclusive {
		t.Errorf("State = %q, want %q on a one-round run", v.State, cacheInconclusive)
	}
}

// If no round reported token counts at all, a zero cached total is the absence of a
// measurement rather than a measurement of absence.
func TestCacheVerdictIsInconclusiveWhenNoRoundReportedTokens(t *testing.T) {
	tally := &bakeoffTally{Rounds: 5, RoundsWithoutUsage: 5}
	if v := tally.cacheVerdict(); v.State != cacheInconclusive {
		t.Errorf("State = %q, want %q when nothing was measured", v.State, cacheInconclusive)
	}
}

// The prefixes the classifier keys on belong to internal/ai/assistant, not here. Asserting
// them against the real producer means a rename there breaks this test rather than
// silently re-labelling every malformed call as somebody else's fault.
func TestTheBadArgumentPrefixStillMatchesWhatDecodeArgsProduces(t *testing.T) {
	var dst struct {
		Title string `json:"title"`
	}
	err := assistant.DecodeArgs(json.RawMessage(`{"titel":"x"}`), &dst)
	if err == nil {
		t.Fatal("DecodeArgs accepted an unknown field")
	}
	if !strings.HasPrefix(err.Error(), badArgumentsPrefix) {
		t.Errorf("DecodeArgs says %q, which no longer starts with %q", err.Error(), badArgumentsPrefix)
	}
}
