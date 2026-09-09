package main

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

// The gauge set is what an alert is written against, so its shape is the contract. A
// success RATE cannot be published directly — a rate over one run is not a rate — so the
// numerator and denominator go out separately and the alert divides them.
func TestRenderPublishesAttemptsAndSuccessesSeparately(t *testing.T) {
	out := render([]snapshot{
		{model: "flagship", attempts: 3, ok: 1, slowest: 90*time.Second + 4*time.Millisecond},
	})

	for _, want := range []string{
		`freehire_llm_probe_attempts{model="flagship"} 3`,
		`freehire_llm_probe_ok{model="flagship"} 1`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing gauge %q in:\n%s", want, out)
		}
	}
}

// The slowest probe separates a refusal from a stall without any error-string parsing: a
// gateway that says no answers in milliseconds, one that is deliberating burns the whole
// deadline. That distinction is what took hours to reach by hand on 2026-09-08.
func TestRenderPublishesTheSlowestProbeInSeconds(t *testing.T) {
	out := render([]snapshot{{model: "m", attempts: 2, ok: 0, slowest: 1500 * time.Millisecond}})
	if !strings.Contains(out, `freehire_llm_probe_slowest_seconds{model="m"} 1.5`) {
		t.Errorf("slowest gauge missing or not in seconds:\n%s", out)
	}
}

// Every gauge carries HELP and TYPE. The textfile collector skips a file it cannot parse,
// so a malformed payload is not a loud failure — it is a metric that quietly stops
// existing, which reads exactly like a healthy silence.
func TestRenderCarriesHelpAndTypeForEveryGauge(t *testing.T) {
	out := render([]snapshot{{model: "m", attempts: 1, ok: 1, slowest: time.Second}})
	gauges := 0
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "# TYPE ") {
			gauges++
			if !strings.HasSuffix(line, " gauge") {
				t.Errorf("not a gauge: %q", line)
			}
		}
	}
	if help := strings.Count(out, "# HELP "); help != gauges || gauges == 0 {
		t.Errorf("%d HELP lines for %d TYPE lines, want them equal and non-zero", help, gauges)
	}
}

// A model id is a label value and arrives from configuration, so it is escaped rather than
// trusted. An unescaped quote would break the line and take the whole file's parse with it.
func TestRenderEscapesTheModelLabel(t *testing.T) {
	out := render([]snapshot{{model: `we"ird\one`, attempts: 1, ok: 1}})
	if !strings.Contains(out, `model="we\"ird\\one"`) {
		t.Errorf("model label not escaped:\n%s", out)
	}
}

// Zero successes is a measurement and must be published as one. Omitting the gauge would
// let an alert on it go stale-but-quiet rather than firing — the exact failure mode that
// let a 77%-failing alias look healthy for hours.
func TestRenderPublishesZeroSuccessesRatherThanOmittingThem(t *testing.T) {
	out := render([]snapshot{{model: "m", attempts: 4, ok: 0}})
	if !strings.Contains(out, `freehire_llm_probe_ok{model="m"} 0`) {
		t.Errorf("a zero success count was not published:\n%s", out)
	}
}

// The deployment routes through more than one alias, and until 2026-09-09 this worker
// watched exactly one of them.
//
// Measured that day: `flagship` (LLM_MODEL) answers from z.ai and is what the assistant, the
// fit analysis and enrichment ride; `fast` (SEARCH_INTENT_MODEL) answers from Gemini and is
// what the AI search filter rides — 10 of 10 probes each, to different providers. A dead
// `fast` is a 500 on every AI search, and nothing would have said so: the probe only ever
// asked `flagship`, which would have gone on answering.
func TestProbedAliasesNamesEveryAliasTheDeploymentUses(t *testing.T) {
	got := probedAliases("flagship", "fast", "")
	want := []string{"flagship", "fast"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("probedAliases = %v, want %v", got, want)
	}
}

// An alias named twice is probed once. Every extra alias falls back to LLM_MODEL when unset
// (cmd/server does the same with cmp.Or), so a deployment that sets none of them would
// otherwise pay for three identical probes and publish three identical series — which a
// per-series alert would then count as three healthy aliases.
func TestProbedAliasesProbesOneAliasOnce(t *testing.T) {
	got := probedAliases("flagship", "flagship", "flagship")
	if !reflect.DeepEqual(got, []string{"flagship"}) {
		t.Errorf("probedAliases = %v, want the alias once", got)
	}
}

// Order is the base first. The base is the alias most of the product rides, so it is the one
// a human scanning the log or the alert list should see first.
func TestProbedAliasesPutsTheBaseFirst(t *testing.T) {
	got := probedAliases("flagship", "fast", "other")
	if len(got) == 0 || got[0] != "flagship" {
		t.Errorf("probedAliases = %v, want the base alias first", got)
	}
}

// A blank base with extras still probes the extras rather than publishing an empty label: an
// empty model label reads as a series about nothing, and the alert would divide it.
func TestProbedAliasesDropsBlanks(t *testing.T) {
	got := probedAliases("", "fast", "  ")
	if !reflect.DeepEqual(got, []string{"fast"}) {
		t.Errorf("probedAliases = %v, want only the named alias", got)
	}
}

// Every probed alias gets its own series, keyed by the label the alert already groups on.
// One file carries them all, because the textfile collector reads whole files: publishing
// per-alias files would leave a retired alias's last numbers on disk forever.
func TestRenderPublishesOneSeriesPerAlias(t *testing.T) {
	out := render([]snapshot{
		{model: "flagship", attempts: 3, ok: 3, slowest: 2 * time.Second},
		{model: "fast", attempts: 3, ok: 0, slowest: 200 * time.Millisecond},
	})
	for _, want := range []string{
		`freehire_llm_probe_ok{model="flagship"} 3`,
		`freehire_llm_probe_ok{model="fast"} 0`,
		`freehire_llm_probe_attempts{model="fast"} 3`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("render is missing %q:\n%s", want, out)
		}
	}
	// HELP/TYPE once per metric, not once per alias: the textfile collector SKIPS a file it
	// cannot parse, and a repeated HELP for the same metric is exactly that — a file that
	// quietly stops existing, which reads like healthy silence.
	if n := strings.Count(out, "# HELP freehire_llm_probe_ok"); n != 1 {
		t.Errorf("HELP for freehire_llm_probe_ok appears %d times, want 1:\n%s", n, out)
	}
}
