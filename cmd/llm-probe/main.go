// Command llm-probe asks EVERY model alias this deployment routes through a trivial question
// a few times and publishes what came back as Prometheus gauges through the node_exporter
// textfile collector, one series per alias. Schedule it every few minutes.
//
// It exists because nothing measured what a provider ANSWERS. Three times in the week to
// 2026-09-08 a provider behind the gateway stopped serving while the gateway went on
// reporting it active — Cerebras on an empty wallet (402), then two Z.ai accounts, one
// rate-limited under a Fair Usage Policy (429) and one with a revoked key (401). Every one
// of them was discovered by a person noticing a feature was broken, and the last cost
// roughly half a day: the fit analysis was failing 55% of real users' requests before
// anyone looked at a log.
//
// The alias is what it probes, not individual keys. Only the gateway host holds the raw
// keys — the admin API returns them masked — so a probe run anywhere else cannot say WHICH
// key is dead. It can say the thing that actually matters and that nothing else says: what
// share of requests through the aliases this deployment uses are being served at all. On
// 2026-09-08 that share was 23%.
//
// It watches every alias because watching one was not enough. Until 2026-09-09 it asked only
// LLM_MODEL. Measured that day, this deployment routes through two: `flagship` answers from
// z.ai and carries the assistant, the fit analysis and enrichment; `fast`
// (SEARCH_INTENT_MODEL) answers from Gemini and carries the AI search filter, whose handler
// turns a gateway refusal into a 500. A dead `fast` would have broken AI search for everyone
// while `flagship` went on answering and this worker went on reporting it healthy — the exact
// silence the worker was built to end.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/llm"
	"github.com/strelov1/freehire/internal/platform/worker"
)

// textfileName is the collector file this worker owns. Like queue-metrics it deliberately
// does NOT follow the name-after-the-binary convention: worker.Main writes the run outcome
// to <binary>.prom after run() returns, so publishing here under "llm-probe" would have
// every run emit these gauges and then immediately overwrite them.
const textfileName = "freehire-llm.prom"

// probes is how many questions one run asks. Three is enough to tell a dead alias from a
// flaky one while staying too small to matter: each is a handful of tokens, so a run every
// five minutes costs less in a day than one fit analysis.
const probes = 3

// probeTimeout bounds one question. It is deliberately far below the callers' own budgets
// (90s for a fit-analysis stage, 180s for an assistant turn): this asks for five tokens, so
// anything approaching a real caller's deadline is already a fault worth reporting rather
// than something to keep waiting for.
const probeTimeout = 30 * time.Second

func main() { worker.Main(run) }

func run() int {
	// Gate before touching anything, the same order queue-metrics follows: with nowhere to
	// publish there is nothing worth spending a token on.
	dir := os.Getenv(worker.PromTextfileDirEnv)
	if dir == "" {
		log.Printf("llm-probe: %s is unset, nothing to publish", worker.PromTextfileDirEnv)
		return 0
	}

	cfg := config.LoadLLM()
	if !cfg.Settings(cfg.Model).Enabled() {
		log.Printf("llm-probe: LLM is unconfigured, nothing to probe")
		return 0
	}

	// Every alias the deployment routes through, not just LLM_MODEL. cmd/server reads these
	// same two through cmp.Or, so this asks exactly what production asks.
	aliases := probedAliases(cfg.Model, os.Getenv("ASSISTANT_MODEL"), os.Getenv("SEARCH_INTENT_MODEL"))

	snaps := make([]snapshot, 0, len(aliases))
	for _, alias := range aliases {
		client, flush, err := llm.NewClient(cfg.Settings(alias), "llm-probe")
		if err != nil {
			// One unbuildable client is not a reason to publish nothing about the others:
			// the file is written whole, so an early return here would erase every alias's
			// numbers and leave the collector reading a stale file as if it were current.
			log.Printf("llm-probe: build client for %q: %v", alias, err)
			continue
		}
		snaps = append(snaps, collect(context.Background(), client.WithTimeout(probeTimeout), alias))
		flush()
	}
	if len(snaps) == 0 {
		log.Printf("llm-probe: no alias could be probed")
		return 1
	}

	if err := worker.WriteTextfile(dir, textfileName, render(snaps)); err != nil {
		log.Printf("llm-probe: %v", err)
		return 1
	}

	for _, snap := range snaps {
		log.Printf("llm-probe: model=%s ok=%d/%d slowest=%s", snap.model, snap.ok, snap.attempts, snap.slowest)
	}
	// A failing alias is not this worker's failure. Exiting non-zero would paint the unit
	// red for something it is only reporting, and a red unit that means "the thing I watch
	// is broken" is indistinguishable from one that means "I am broken".
	return 0
}

// snapshot is one run's measurement.
type snapshot struct {
	model    string
	attempts int
	ok       int
	// slowest is the longest single probe, successful or not. It separates a refusal from
	// a stall without parsing an error string: a gateway that says no answers in
	// milliseconds, one that is deliberating burns the whole budget.
	slowest time.Duration
}

// collect asks the alias the same trivial question `probes` times.
//
// The failures are counted, not classified. A class would have to come from matching an
// error string, and internal/platform/llm's own notes say that wording changes silently
// where the status code does not — so a classifier here would decay into reporting
// "other" for the next new failure, which is worse than a number that is simply true. The
// slowest-probe gauge carries the distinction that actually matters, and a human reading
// the log after an alert gets the exact text.
func collect(ctx context.Context, client *llm.Client, model string) snapshot {
	snap := snapshot{model: model, attempts: probes}
	for i := 0; i < probes; i++ {
		start := time.Now()
		_, err := client.GenerateJSONStream(ctx, `Reply with {"ok":true} and nothing else.`, "ping", nil)
		took := time.Since(start)
		if took > snap.slowest {
			snap.slowest = took
		}
		if err != nil {
			log.Printf("llm-probe: probe %d/%d failed after %s: %v", i+1, probes, took, err)
			continue
		}
		snap.ok++
	}
	return snap
}

// probedAliases names every alias this deployment actually routes through, once each.
//
// The extras fall back to the base when unset, exactly as cmd/server's cmp.Or does, so a
// deployment that names none of them still probes one alias rather than three copies of it —
// and a per-series alert is not handed three healthy-looking aliases that are one.
//
// The base goes first: it is what most of the product rides, so it is what a human scanning
// the log or the alert list should read first.
func probedAliases(base string, extra ...string) []string {
	out := make([]string, 0, 1+len(extra))
	seen := make(map[string]bool, 1+len(extra))
	for _, alias := range append([]string{base}, extra...) {
		alias = strings.TrimSpace(alias)
		// A blank is not an alias. Publishing one would put an empty model label on a series
		// the alert then divides — a measurement about nothing, indistinguishable from a
		// measurement about something.
		if alias == "" || seen[alias] {
			continue
		}
		seen[alias] = true
		out = append(out, alias)
	}
	return out
}

// render writes the gauge set, one series per alias.
//
// The success RATE is not published: a rate over three probes is not a rate. The numerator
// and denominator go out separately and whoever writes the alert divides them, which also
// lets the alert choose its own window. The division is element-wise on the `model` label,
// so an alias added here becomes another alert instance rather than blending into the first.
//
// HELP and TYPE are written once per METRIC, not once per series: the textfile collector
// skips a file it cannot parse, and a repeated HELP for one metric is exactly that — a
// metric that quietly stops existing, which reads like healthy silence.
func render(snaps []snapshot) string {
	var b strings.Builder
	for _, m := range []struct {
		name, help string
		value      func(snapshot) string
	}{
		{"freehire_llm_probe_attempts", "Questions asked of each model alias in the last run.",
			func(s snapshot) string { return fmt.Sprintf("%d", s.attempts) }},
		{"freehire_llm_probe_ok", "How many of those the gateway answered.",
			func(s snapshot) string { return fmt.Sprintf("%d", s.ok) }},
		{"freehire_llm_probe_slowest_seconds", "The longest single probe, answered or not.",
			func(s snapshot) string { return fmt.Sprintf("%g", s.slowest.Seconds()) }},
	} {
		fmt.Fprintf(&b, "# HELP %s %s\n# TYPE %s gauge\n", m.name, m.help, m.name)
		for _, s := range snaps {
			// %s, not %q: escapeLabel has already done the escaping, and %q would do it a
			// second time — turning one backslash into four and the label into something no
			// alert matches.
			fmt.Fprintf(&b, "%s{model=\"%s\"} %s\n", m.name, escapeLabel(s.model), m.value(s))
		}
	}
	return b.String()
}

// escapeLabel makes a configured value safe as a Prometheus label. The model id comes from
// the environment, and an unescaped quote in it would break the line and take the whole
// file's parse down with it.
func escapeLabel(v string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(v)
}
