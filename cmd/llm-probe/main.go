// Command llm-probe asks the configured model alias a trivial question a few times and
// publishes what came back as Prometheus gauges through the node_exporter textfile
// collector. Schedule it every few minutes.
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
// share of requests through the alias this deployment uses are being served at all. On
// 2026-09-08 that share was 23%.
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

	client, flush, err := llm.NewClient(cfg.Settings(cfg.Model), "llm-probe")
	if err != nil {
		log.Printf("llm-probe: build client: %v", err)
		return 1
	}
	defer flush()

	snap := collect(context.Background(), client.WithTimeout(probeTimeout), cfg.Model)

	if err := worker.WriteTextfile(dir, textfileName, render(snap)); err != nil {
		log.Printf("llm-probe: %v", err)
		return 1
	}

	log.Printf("llm-probe: model=%s ok=%d/%d slowest=%s", snap.model, snap.ok, snap.attempts, snap.slowest)
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

// render writes the gauge set.
//
// The success RATE is not published: a rate over three probes is not a rate. The numerator
// and denominator go out separately and whoever writes the alert divides them, which also
// lets the alert choose its own window.
func render(s snapshot) string {
	var b strings.Builder
	// %s, not %q: escapeLabel has already done the escaping, and %q would do it a second
	// time — turning one backslash into four and the label into something no alert matches.
	label := fmt.Sprintf(`{model="%s"}`, escapeLabel(s.model))

	gauge(&b, "freehire_llm_probe_attempts", "Questions asked of the configured model alias in the last run.", label, fmt.Sprintf("%d", s.attempts))
	gauge(&b, "freehire_llm_probe_ok", "How many of those the gateway answered.", label, fmt.Sprintf("%d", s.ok))
	gauge(&b, "freehire_llm_probe_slowest_seconds", "The longest single probe, answered or not.", label, fmt.Sprintf("%g", s.slowest.Seconds()))

	return b.String()
}

// gauge writes one HELP/TYPE/value trio. Every gauge carries both, because the textfile
// collector SKIPS a file it cannot parse: a malformed payload is not a loud failure, it is
// a metric that quietly stops existing — which reads exactly like a healthy silence.
func gauge(b *strings.Builder, name, help, labels, value string) {
	fmt.Fprintf(b, "# HELP %s %s\n# TYPE %s gauge\n%s%s %s\n", name, help, name, name, labels, value)
}

// escapeLabel makes a configured value safe as a Prometheus label. The model id comes from
// the environment, and an unescaped quote in it would break the line and take the whole
// file's parse down with it.
func escapeLabel(v string) string {
	return strings.NewReplacer(`\`, `\\`, `"`, `\"`, "\n", `\n`).Replace(v)
}
