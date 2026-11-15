// Command llm-probe asks EVERY model alias this deployment routes through a trivial question
// a few times and publishes what came back as Prometheus gauges through the node_exporter
// textfile collector, one series per alias. Schedule it every few minutes.
//
// It exists because nothing measured what a provider ANSWERS: a provider behind the gateway
// can stop serving (empty wallet 402, rate limit 429, revoked key 401) while the gateway
// keeps reporting it active, and the failure is otherwise found by a person noticing a
// broken feature.
//
// The alias is what it probes, not individual keys: only the gateway host holds the raw
// keys, so a probe elsewhere cannot say WHICH key is dead. It says what share of requests
// through the aliases this deployment uses are being served at all.
//
// It watches every alias, not just LLM_MODEL: `flagship` carries the assistant, fit analysis
// and enrichment, while `fast` (SEARCH_INTENT_MODEL) carries the AI search filter, whose
// handler turns a gateway refusal into a 500. A dead `fast` would otherwise go unseen.
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

// textfileName is the collector file this worker owns. It must not be named after the binary:
// worker.Main writes the run outcome to <binary>.prom after run() returns and would overwrite
// these gauges.
const textfileName = "freehire-llm.prom"

// probes is how many questions one run asks: enough to tell a dead alias from a flaky one,
// at a handful of tokens each.
const probes = 3

// probeTimeout bounds one question. It is far below the callers' own budgets: a five-token
// answer that approaches them is already a fault worth reporting.
const probeTimeout = 30 * time.Second

func main() { worker.Main(run) }

func run() int {
	// Gate first: with nowhere to publish there is nothing worth spending a token on.
	dir := os.Getenv(worker.PromTextfileDirEnv)
	if dir == "" {
		log.Printf("llm-probe: %s is unset, nothing to publish", worker.PromTextfileDirEnv)
		return worker.Skip(worker.PromTextfileDirEnv)
	}

	cfg := config.LoadLLM()
	if !cfg.Settings(cfg.Model).Enabled() {
		log.Printf("llm-probe: LLM is unconfigured, nothing to probe")
		return worker.Skip("LLM_BASE_URL+LLM_API_KEY+LLM_MODEL")
	}

	// Every alias the deployment routes through; cmd/server reads these through cmp.Or too.
	aliases := probedAliases(cfg.Model, os.Getenv("ASSISTANT_MODEL"), os.Getenv("SEARCH_INTENT_MODEL"))

	snaps := make([]snapshot, 0, len(aliases))
	for _, alias := range aliases {
		client, flush, err := llm.NewClient(cfg.Settings(alias), "llm-probe")
		if err != nil {
			// Don't return early: the file is written whole, so that would erase every
			// alias's numbers and leave the collector reading a stale file.
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
	// A failing alias is not this worker's failure: a red unit must keep meaning "I am broken".
	return 0
}

// snapshot is one run's measurement.
type snapshot struct {
	model    string
	attempts int
	ok       int
	// slowest is the longest single probe, successful or not. It separates a fast refusal
	// from a stall without parsing an error string.
	slowest time.Duration
}

// collect asks the alias the same trivial question `probes` times.
//
// Failures are counted, not classified: a class would come from matching error strings,
// whose wording changes silently (see internal/platform/llm). The slowest-probe gauge
// carries the distinction that matters, and the log has the exact text.
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
// The extras fall back to the base when unset, as cmd/server's cmp.Or does, so a deployment
// naming none of them probes one alias, not three copies. The base goes first because most
// of the product rides it.
func probedAliases(base string, extra ...string) []string {
	out := make([]string, 0, 1+len(extra))
	seen := make(map[string]bool, 1+len(extra))
	for _, alias := range append([]string{base}, extra...) {
		alias = strings.TrimSpace(alias)
		// A blank is not an alias: it would publish a series with an empty model label.
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
// No success rate is published: numerator and denominator go out separately and the alert
// divides them (element-wise on `model`, so a new alias becomes its own alert instance) and
// picks its own window.
//
// HELP and TYPE are written once per metric, not per series: the textfile collector skips a
// file it cannot parse, and a repeated HELP is exactly that.
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
			// %s, not %q: escapeLabel already escaped, and %q would double-escape.
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
