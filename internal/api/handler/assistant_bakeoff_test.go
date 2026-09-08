package handler

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/candidate/atscheck"
	"github.com/strelov1/freehire/internal/candidate/cvmatch"
	"github.com/strelov1/freehire/internal/platform/llm"
)

func matchOf(overall int) *cvmatch.Score { return &cvmatch.Score{Overall: overall} }
func deltaOf(change int) *atscheck.Delta { return &atscheck.Delta{Change: change} }

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

func rate(v float64) *float64 { return &v }

// nearly compares dollars without inviting a float-equality flake into a money assertion.
func nearly(t *testing.T, got, want float64, what string) {
	t.Helper()
	if diff := got - want; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("%s = %.9f, want %.9f", what, got, want)
	}
}

// The cache-read rate is the whole point of measuring cached tokens: it is where a model
// that caches the replayed prefix stops costing what its input rate says.
func TestBakeoffCostPricesCachedTokensAtTheCacheRate(t *testing.T) {
	prices := bakeoffPrices{"deepseek/deepseek-v4-flash-0731": {
		Input: 0.05, Output: 0.16, CacheRead: rate(0.013),
	}}
	tally := bakeoffTally{Rounds: 3, Input: 1_000_000, CachedInput: 800_000, Output: 100_000}

	cost := prices.cost("deepseek/deepseek-v4-flash-0731", tally)
	if !cost.Known {
		t.Fatal("cost unknown for a model the table names")
	}
	// 200k fresh input at 0.05, 800k cached at 0.013, 100k output at 0.16.
	nearly(t, cost.USD, 0.2*0.05+0.8*0.013+0.1*0.16, "USD")
}

// A table that names no cache rate is not a table naming zero. A provider without a
// prompt-cache discount charges its ordinary input rate for those tokens, and defaulting
// the unnamed rate to zero would hand it the cheapest possible run in the ranking.
func TestBakeoffCostChargesTheInputRateWhenNoCacheRateIsNamed(t *testing.T) {
	prices := bakeoffPrices{"m": {Input: 1, Output: 2}}
	tally := bakeoffTally{Rounds: 3, Input: 1_000_000, CachedInput: 900_000, Output: 0}

	nearly(t, prices.cost("m", tally).USD, 1.0, "USD")
}

// A model the table does not name is reported without a cost. Zero would rank it first.
func TestBakeoffCostIsUnknownForAModelTheTableDoesNotName(t *testing.T) {
	cost := bakeoffPrices{}.cost("who/knows", bakeoffTally{Rounds: 2, Input: 1_000_000})

	if cost.Known {
		t.Errorf("cost reported as known: %+v", cost)
	}
	if cost.USD != 0 {
		t.Errorf("USD = %v, want 0 alongside Known=false", cost.USD)
	}
}

// Rounds whose provider reported nothing are rounds we were not billed for in this sum but
// were billed for in reality. The figure is a floor, and the row says so rather than
// passing an undercount off as the price.
func TestBakeoffCostIsAFloorWhenSomeRoundsReportedNoTokens(t *testing.T) {
	prices := bakeoffPrices{"m": {Input: 1, Output: 1}}
	cost := prices.cost("m", bakeoffTally{Rounds: 4, RoundsWithoutUsage: 2, Input: 1_000_000})

	if !cost.Floor {
		t.Error("Floor = false on a run with unmeasured rounds")
	}
}

// The same impossible reading usageBuckets refuses to split is refused here: subtracting
// would price a negative number of fresh tokens and make the run look like a refund.
func TestBakeoffCostChargesTheWholeInputWhenCachedExceedsIt(t *testing.T) {
	prices := bakeoffPrices{"m": {Input: 1, Output: 0, CacheRead: rate(0)}}
	cost := prices.cost("m", bakeoffTally{Rounds: 2, Input: 100_000, CachedInput: 900_000})

	nearly(t, cost.USD, 0.1, "USD")
}

// The ranking is on the deterministic scores, ATS breaking a tie on match. Nothing here
// consults a model: what is good is read from the CVs the report carries.
func TestBakeoffRankingOrdersOnMatchThenAtsDelta(t *testing.T) {
	rows := []bakeoffRow{
		{Model: "c", Match: matchOf(70), ATS: deltaOf(9)},
		{Model: "a", Match: matchOf(88), ATS: deltaOf(1)},
		{Model: "b", Match: matchOf(88), ATS: deltaOf(4)},
	}
	rankBakeoffRows(rows)

	if got := []string{rows[0].Model, rows[1].Model, rows[2].Model}; got[0] != "b" || got[1] != "a" || got[2] != "c" {
		t.Errorf("order = %v, want [b a c]", got)
	}
}

// A run that never finished has no score, and an absent score is not a low one. Sorting it
// as zero would say the model tried and did badly, when it did not get to try.
func TestBakeoffRankingPutsFailedRunsLastWithoutScoringThemZero(t *testing.T) {
	rows := []bakeoffRow{
		{Model: "failed", Failure: "context deadline exceeded"},
		{Model: "poor", Match: matchOf(3)},
	}
	rankBakeoffRows(rows)

	if rows[0].Model != "poor" || rows[1].Model != "failed" {
		t.Errorf("order = [%s %s], want [poor failed]", rows[0].Model, rows[1].Model)
	}
}

// The catalogue quotes dollars per TOKEN in strings; everything downstream reasons in
// dollars per million, because that is the unit a person compares models in.
func TestParseBakeoffPricesConvertsPerTokenStringsToPerMillion(t *testing.T) {
	table := mustParsePrices(t)

	got, ok := table.Rates["deepseek/deepseek-v4-flash-0731"]
	if !ok {
		t.Fatalf("model missing from the parsed table: %v", table.Rates)
	}
	nearly(t, got.Input, 0.14, "input per million")
	nearly(t, got.Output, 0.28, "output per million")
	if got.CacheRead == nil {
		t.Fatal("cache-read rate absent for a model the catalogue prices")
	}
	nearly(t, *got.CacheRead, 0.028, "cache read per million")
}

// The distinction the whole price table turns on, and the fixture holds both halves of it:
// a free model's input really is zero, and its cache-read rate is genuinely unquoted. A
// parser that defaulted the missing one to zero could not tell them apart, and would price
// a cached token at nothing for every provider that offers no discount.
func TestParseBakeoffPricesKeepsAnUnquotedCacheRateAbsentNotZero(t *testing.T) {
	got := mustParsePrices(t).Rates["google/gemma-4-31b-it:free"]

	if got.Input != 0 || got.Output != 0 {
		t.Errorf("a free model's quoted zeros were lost: %+v", got)
	}
	if got.CacheRead != nil {
		t.Errorf("CacheRead = %v, want nil — the catalogue quotes none", *got.CacheRead)
	}
}

// The capture date travels with the table because a price is only true on a day. A report
// that cannot say when its prices were read cannot be told from one read this morning.
func TestParseBakeoffPricesCarriesTheCaptureDate(t *testing.T) {
	if d := mustParsePrices(t).Captured; d == "" {
		t.Error("the parsed table carries no capture date")
	}
}

// A capture naming no model is a failed capture, not an empty catalogue — the same rule
// cmd/build-suggestions follows when it refuses to swap in an empty dictionary.
func TestParseBakeoffPricesRefusesACaptureWithNoModels(t *testing.T) {
	if _, err := parseBakeoffPrices([]byte(`{"captured":"2026-09-08","data":[]}`)); err == nil {
		t.Error("an empty capture parsed cleanly")
	}
}

// A table whose age is unknown would be reported as though it were current.
func TestParseBakeoffPricesRefusesACaptureWithNoDate(t *testing.T) {
	raw := []byte(`{"data":[{"id":"m","pricing":{"prompt":"0.000001","completion":"0.000002"}}]}`)
	if _, err := parseBakeoffPrices(raw); err == nil {
		t.Error("a capture with no date parsed cleanly")
	}
}

// A price that does not parse is named rather than skipped: a model silently dropped from
// the table reads downstream as one the catalogue never listed, and is reported with an
// unknown cost instead of the wrong one — which hides the typo rather than showing it.
func TestParseBakeoffPricesNamesTheModelWhoseRateWillNotParse(t *testing.T) {
	raw := []byte(`{"captured":"2026-09-08","data":[{"id":"broken/model","pricing":{"prompt":"tuppence","completion":"0.000002"}}]}`)
	_, err := parseBakeoffPrices(raw)
	if err == nil {
		t.Fatal("an unparseable rate was accepted")
	}
	if !strings.Contains(err.Error(), "broken/model") {
		t.Errorf("error %q does not name the offending model", err)
	}
}

func mustParsePrices(t *testing.T) bakeoffPriceTable {
	t.Helper()
	raw, err := os.ReadFile(filepath.FromSlash(bakeoffPriceFixture))
	if err != nil {
		t.Fatalf("reading the price fixture: %v", err)
	}
	table, err := parseBakeoffPrices(raw)
	if err != nil {
		t.Fatalf("parseBakeoffPrices: %v", err)
	}
	return table
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
