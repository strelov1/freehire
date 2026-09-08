package handler

import (
	"sort"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/candidate/atscheck"
	"github.com/strelov1/freehire/internal/candidate/cvmatch"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// The model bake-off's measuring core: what one autopilot run cost and how it behaved.
// The live runs themselves are a //go:build llmlive test; everything here is pure, so it
// is checked by the ordinary suite rather than only by a run that spends money.
//
// See openspec/changes/assistant-model-bakeoff for why any of this is measured. The short
// version: a per-million price does not predict a turn's cost here, because the transcript
// is replayed every round, so what a model costs is decided by how many rounds it takes
// and how much of the replayed prefix its provider serves from cache. Neither figure
// appears on a price page.

// These prefixes belong to internal/ai/assistant — Registry.Call builds the first from
// DecodeArgs and writes the second itself. They are repeated rather than exported because
// they are a diagnostic read of a message written for a MODEL to act on, not a contract;
// TestTheBadArgumentPrefixStillMatchesWhatDecodeArgsProduces pins the copy to the original
// so a rename there fails here instead of quietly re-labelling every malformed call.
const (
	badArgumentsPrefix = "invalid arguments:"
	unknownToolPrefix  = "unknown tool "
)

// bakeoffTally accumulates one autopilot run from two sources, because one is not enough.
//
// Rounds and tool outcomes are visible in the event stream a client sees. Token counts are
// not: the runner emits a single usage event per turn, on its terminal path, so a run of
// thirty rounds reports the last call's tokens and says nothing about the other
// twenty-nine. The bake-off already substitutes the turn model, so it counts tokens at
// that seam — a measurement problem solved where it costs nothing rather than by changing
// what every assistant turn in production streams.
type bakeoffTally struct {
	// Rounds is model calls, the multiplier on everything else: the transcript is
	// replayed into each one, so round N carries the cost of rounds 1..N-1 with it.
	Rounds int
	// RoundsWithoutUsage counts calls whose provider reported no tokens at all. Such a
	// round is unmeasured, never free — a total built over fewer rounds than actually
	// ran is an undercount, and saying so is the only way a reader can tell.
	RoundsWithoutUsage int

	Input       int
	Output      int
	CachedInput int

	// FirstRound is how long the first model call took. It is what a reader waits
	// through before anything appears on screen, and a slow prefill here is what reads
	// to them as a turn that hung.
	FirstRound time.Duration

	ToolCalls int
	// BadArguments and UnknownTools are the MODEL's failures — arguments that did not
	// decode, and a tool name it invented. ServiceErrors are ours. Folding the three
	// into one number would rank whichever candidate happened to run while a database
	// was busy.
	BadArguments  int
	UnknownTools  int
	ServiceErrors int

	// Stop is the run's terminal reason, one of assistant's Stop* constants.
	Stop string
}

// observeCall records one model call: its cost, and how long it took to come back.
func (t *bakeoffTally) observeCall(usage *llm.Usage, took time.Duration) {
	if t.Rounds == 0 {
		t.FirstRound = took
	}
	t.Rounds++
	if usage == nil {
		t.RoundsWithoutUsage++
		return
	}
	t.Input += usage.Input
	t.Output += usage.Output
	t.CachedInput += usage.CachedInput
}

// observeEvent records one streamed frame of the run.
func (t *bakeoffTally) observeEvent(e assistant.Event) {
	switch e.Kind {
	case assistant.EventToolUse:
		t.ToolCalls++
	case assistant.EventToolResult:
		t.classifyToolFailure(e)
	case assistant.EventResult:
		t.Stop = e.StopReason
	}
}

// What a run can be said to show about its provider's prompt cache.
//
// There are three answers and not two, because a zero is ambiguous at call scope:
// langchaingo writes the cached key unconditionally from a zero-valued struct, so a
// provider that reported nothing and one that served nothing both arrive as zero. What
// disambiguates them is the RUN — several rounds against a prefix that should have been
// warm, all reporting zero, is evidence; one round is not.
const (
	cacheObserved     = "observed"
	cacheNotObserved  = "no cache observed"
	cacheInconclusive = "inconclusive"
)

// cacheReading is what a run showed about its provider's prompt cache.
type cacheReading struct {
	State string
	// Share is the fraction of input tokens served from cache, set only when the state
	// is cacheObserved. It stays zero otherwise rather than reporting a rate that was
	// never measured.
	Share float64
}

// cacheVerdict reads the run as a whole.
//
// A single round is inconclusive by construction: there was no earlier round for a cache
// to have held, so a zero there convicts a provider on a run that never gave it the
// chance. A run whose provider reported no token counts at all is likewise inconclusive —
// that is the absence of a measurement, not a measurement of absence.
func (t *bakeoffTally) cacheVerdict() cacheReading {
	switch {
	case t.CachedInput > 0 && t.Input > 0:
		return cacheReading{State: cacheObserved, Share: float64(t.CachedInput) / float64(t.Input)}
	case t.Rounds < 2 || t.RoundsWithoutUsage >= t.Rounds:
		return cacheReading{State: cacheInconclusive}
	default:
		return cacheReading{State: cacheNotObserved}
	}
}

// bakeoffRates is one model's prices, in dollars per MILLION tokens, as the gateway's
// own catalogue states them.
type bakeoffRates struct {
	Input  float64
	Output float64
	// CacheRead is what a cached input token costs. Absent is not zero: a free model's
	// genuine zero and a catalogue that names no cache rate are different facts, and
	// treating the second as the first would hand a provider with no cache discount at
	// all the cheapest possible run in the ranking.
	CacheRead *float64
}

// bakeoffPrices is the checked-in price table, keyed by the gateway's model id.
type bakeoffPrices map[string]bakeoffRates

// bakeoffCost is what a run cost, or the honest absence of that figure.
type bakeoffCost struct {
	// Known is false when the table does not name the model. The cost is then reported
	// as absent — a zero would sort it first, which is the one place a missing price
	// must never land.
	Known bool
	USD   float64
	// Floor is set when some rounds reported no tokens: those calls were billed in
	// reality and are missing from this sum, so the true figure is higher.
	Floor bool
}

// cost prices a run.
//
// Cached tokens are charged at the cache-read rate when the catalogue names one and at the
// ordinary input rate when it does not — which is what a provider without a prompt-cache
// discount actually charges for them.
func (p bakeoffPrices) cost(model string, t bakeoffTally) bakeoffCost {
	rates, ok := p[model]
	if !ok {
		return bakeoffCost{}
	}

	// The same impossible reading usageBuckets refuses to split: subtracting would price a
	// negative number of fresh tokens and turn the run into a refund.
	cached := t.CachedInput
	if cached < 0 || cached > t.Input {
		cached = 0
	}
	cachedRate := rates.Input
	if rates.CacheRead != nil {
		cachedRate = *rates.CacheRead
	}

	const perMillion = 1_000_000.0
	usd := (float64(t.Input-cached)*rates.Input +
		float64(cached)*cachedRate +
		float64(t.Output)*rates.Output) / perMillion

	return bakeoffCost{Known: true, USD: usd, Floor: t.RoundsWithoutUsage > 0}
}

// bakeoffRow is one (model, case) result: what the run cost, and what it produced.
type bakeoffRow struct {
	Model   string
	Case    string
	Vacancy string
	Tally   bakeoffTally
	Cost    bakeoffCost

	// Match and ATS are the deterministic quality scores, nil on a run that did not
	// complete. TailoredCV is the document itself — the report carries it so the CVs
	// can be read and compared without re-running anything, because whether one is any
	// good is a judgement no number here makes.
	Match      *cvmatch.Score
	ATS        *atscheck.Delta
	TailoredCV string

	// Failure is the reason a run did not complete, empty when it did.
	Failure string
}

// rankBakeoffRows orders rows best-first on the deterministic scores: match overall, with
// the ATS delta breaking a tie.
//
// A run that did not complete sorts last and is NOT scored zero. An absent score says the
// model never got to try; a zero would say it tried and did badly, and the two would rank
// a crashed run below a bad one for the wrong reason.
func rankBakeoffRows(rows []bakeoffRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if (a.Match == nil) != (b.Match == nil) {
			return b.Match == nil
		}
		if a.Match == nil {
			return false
		}
		if a.Match.Overall != b.Match.Overall {
			return a.Match.Overall > b.Match.Overall
		}
		return atsChange(a.ATS) > atsChange(b.ATS)
	})
}

// atsChange reads a delta's move, treating an absent delta as no move rather than as a
// fall — the ATS report is a second opinion, and its absence is not evidence against a CV.
func atsChange(d *atscheck.Delta) int {
	if d == nil {
		return 0
	}
	return d.Change
}

// classifyToolFailure sorts a failed tool result into whose failure it was.
//
// It keys on the stream's explicit error flag and never on the payload's text: a vacancy
// titled "Error Budget Engineer" comes back in a perfectly successful result, and a
// classifier reading for the word would charge the model for finding it.
func (t *bakeoffTally) classifyToolFailure(e assistant.Event) {
	if !e.IsError {
		return
	}
	switch {
	case strings.Contains(e.Result, badArgumentsPrefix):
		t.BadArguments++
	case strings.Contains(e.Result, unknownToolPrefix):
		t.UnknownTools++
	default:
		t.ServiceErrors++
	}
}
