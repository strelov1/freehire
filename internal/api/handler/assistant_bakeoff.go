package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
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

// bakeoffPriceTable is a dated capture of the gateway's own model catalogue.
//
// The date is not decoration. A price is only true on a day, and a report that cannot say
// when its prices were read cannot be told from one read this morning. Prices are read
// from the catalogue and never written by hand for the reason this table's first capture
// demonstrated: the catalogue quoted deepseek-v4-flash-0731 at $0.14/$0.28 per million
// while every secondhand summary of it said $0.05/$0.16. A hard-coded figure would have
// been wrong on the day it was typed and silent about it ever after.
type bakeoffPriceTable struct {
	Captured string
	Source   string
	Rates    bakeoffPrices
}

// bakeoffCaseFixture is the committed case set: real postings the profile-match sort put in
// front of the profile the cases are run against, captured with
//
//	freehire auth login                                     # once
//	curl -H "Authorization: Bearer <key>" \
//	  'https://freehire.me/api/v1/jobs/search?sort=match&limit=8'   # pick slugs
//	curl -sS 'https://freehire.me/api/v1/jobs/<slug>'               # the FULL posting
//
// Two details are load-bearing. They are real rather than written, because a synthetic
// vacancy states requirements somebody invented to be answerable, and a run over those
// measures how well a model answers a question built to be answered. And they are the ones
// this profile actually matches, because tailoring a backend CV against a posting it has no
// business answering measures refusal, not skill.
//
// The full posting comes from the DETAIL endpoint, never from the search hit: the list
// truncates a description at about a thousand characters, and every posting arriving at
// suspiciously the same length is the signature of that — a shorter measurement wearing the
// same case id.
const bakeoffCaseFixture = "testdata/bakeoff-cases.json"

// bakeoffVacancy is the posting a case tailors against, in the columns the seed needs.
type bakeoffVacancy struct {
	Source      string `json:"source"`
	ExternalID  string `json:"external_id"`
	URL         string `json:"url"`
	Slug        string `json:"slug"`
	Title       string `json:"title"`
	Company     string `json:"company"`
	Description string `json:"description"`
}

// bakeoffCase is one posting every candidate model is run against.
type bakeoffCase struct {
	ID      string         `json:"id"`
	Vacancy bakeoffVacancy `json:"vacancy"`
}

// bakeoffCaseSet is the committed cases and when they were captured.
type bakeoffCaseSet struct {
	Captured string        `json:"captured"`
	Cases    []bakeoffCase `json:"cases"`
}

// loadBakeoffCases reads the case set from disk.
//
// Failing to read the cases is the ONE failure that ends a bake-off — every other failure
// is a row in the report — so it is loud and names the file. Silence here would run every
// model over nothing and report a clean sweep of zero rows, which reads exactly like a
// bake-off that found no difference between them.
func loadBakeoffCases(path string) (bakeoffCaseSet, error) {
	raw, err := os.ReadFile(filepath.FromSlash(path))
	if err != nil {
		return bakeoffCaseSet{}, fmt.Errorf("bake-off cases: reading %s: %w", path, err)
	}
	return loadBakeoffCasesFrom(raw, path)
}

// loadBakeoffCasesFrom validates a case set that has already been read.
func loadBakeoffCasesFrom(raw []byte, origin string) (bakeoffCaseSet, error) {
	var set bakeoffCaseSet
	if err := json.Unmarshal(raw, &set); err != nil {
		return bakeoffCaseSet{}, fmt.Errorf("bake-off cases: parsing %s: %w", origin, err)
	}
	if len(set.Cases) == 0 {
		return bakeoffCaseSet{}, fmt.Errorf("bake-off cases: %s names no case", origin)
	}

	seen := make(map[string]struct{}, len(set.Cases))
	for _, c := range set.Cases {
		if strings.TrimSpace(c.ID) == "" {
			return bakeoffCaseSet{}, fmt.Errorf("bake-off cases: %s carries a case with no id, and rows are keyed by it", origin)
		}
		if _, dup := seen[c.ID]; dup {
			// Two rows keyed alike are indistinguishable in the report, and a reader
			// comparing them would be comparing two different postings.
			return bakeoffCaseSet{}, fmt.Errorf("bake-off cases: %s uses the id %q twice", origin, c.ID)
		}
		seen[c.ID] = struct{}{}

		// A posting with no description gives the run nothing to walk. The autopilot
		// would finish in a round or two and score well against a vacancy that asked for
		// nothing — the cheapest, fastest and most meaningless row in the table.
		if strings.TrimSpace(c.Vacancy.Description) == "" || strings.TrimSpace(c.Vacancy.Title) == "" {
			return bakeoffCaseSet{}, fmt.Errorf("bake-off cases: %s case %q has no title or no description to tailor against", origin, c.ID)
		}
	}

	return set, nil
}

// bakeoffPriceFixture is the committed capture. Refresh it — and only then re-read a
// report's figures — with:
//
//	curl -sS 'https://openrouter.ai/api/v1/models' | jq --arg d "$(date -u +%Y-%m-%d)" '{
//	  captured: $d,
//	  source: "https://openrouter.ai/api/v1/models",
//	  data: [ .data[] | select(.id | IN($ARGS.positional[])) | {id, name, context_length, pricing} ]
//	}' --args <model-id>... > internal/api/handler/testdata/openrouter-prices.json
//
// It is trimmed to the candidates on purpose: the whole catalogue is hundreds of models
// whose churn would land in every diff, and a model absent from the table is already
// reported with an unknown cost rather than a wrong one.
const bakeoffPriceFixture = "testdata/openrouter-prices.json"

// catalogueCapture is the wire shape of the committed fixture: the gateway's own model
// entries, unaltered, wrapped in the two facts the gateway does not supply — when they
// were read and from where.
type catalogueCapture struct {
	Captured string `json:"captured"`
	Source   string `json:"source"`
	Data     []struct {
		ID      string `json:"id"`
		Pricing struct {
			// The catalogue quotes dollars per TOKEN, as strings. A string is not
			// JSON-number sloppiness: these are values like 0.000000028, and reading
			// them as text is how the exact quote survives to be parsed once, here.
			Prompt     string `json:"prompt"`
			Completion string `json:"completion"`
			// InputCacheRead is a pointer because its ABSENCE is the fact that matters:
			// a provider quoting no cache rate and one quoting zero are different, and
			// only the second means a cached token is free.
			InputCacheRead *string `json:"input_cache_read"`
		} `json:"pricing"`
	} `json:"data"`
}

// parseBakeoffPrices reads a capture into the per-million rates everything downstream uses.
//
// It refuses rather than degrades on three shapes, all of which would otherwise reach a
// report looking like an answer: a capture naming no model (a failed capture, not an empty
// catalogue — the rule cmd/build-suggestions follows when it will not swap in an empty
// dictionary), a capture with no date, and a rate that will not parse. The last is named
// rather than skipped: a model dropped from the table is indistinguishable downstream from
// one the catalogue never listed, and would be reported with an unknown cost — which hides
// the typo instead of showing it.
func parseBakeoffPrices(raw []byte) (bakeoffPriceTable, error) {
	var capture catalogueCapture
	if err := json.Unmarshal(raw, &capture); err != nil {
		return bakeoffPriceTable{}, fmt.Errorf("price capture: %w", err)
	}
	if capture.Captured == "" {
		return bakeoffPriceTable{}, errors.New("price capture: no capture date, so its age cannot be reported")
	}
	if len(capture.Data) == 0 {
		return bakeoffPriceTable{}, errors.New("price capture: names no model, which is a failed capture rather than an empty catalogue")
	}

	rates := make(bakeoffPrices, len(capture.Data))
	for _, m := range capture.Data {
		in, err := perMillion(m.Pricing.Prompt)
		if err != nil {
			return bakeoffPriceTable{}, fmt.Errorf("price capture: %s prompt rate: %w", m.ID, err)
		}
		out, err := perMillion(m.Pricing.Completion)
		if err != nil {
			return bakeoffPriceTable{}, fmt.Errorf("price capture: %s completion rate: %w", m.ID, err)
		}
		r := bakeoffRates{Input: in, Output: out}
		if m.Pricing.InputCacheRead != nil {
			cache, err := perMillion(*m.Pricing.InputCacheRead)
			if err != nil {
				return bakeoffPriceTable{}, fmt.Errorf("price capture: %s cache-read rate: %w", m.ID, err)
			}
			r.CacheRead = &cache
		}
		rates[m.ID] = r
	}

	return bakeoffPriceTable{Captured: capture.Captured, Source: capture.Source, Rates: rates}, nil
}

// perMillion turns the catalogue's dollars-per-token string into dollars per million.
func perMillion(quoted string) (float64, error) {
	v, err := strconv.ParseFloat(strings.TrimSpace(quoted), 64)
	if err != nil {
		return 0, fmt.Errorf("%q is not a rate", quoted)
	}
	return v * 1_000_000, nil
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
