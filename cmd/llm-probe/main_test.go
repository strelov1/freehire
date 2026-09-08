package main

import (
	"strings"
	"testing"
	"time"
)

// The gauge set is what an alert is written against, so its shape is the contract. A
// success RATE cannot be published directly — a rate over one run is not a rate — so the
// numerator and denominator go out separately and the alert divides them.
func TestRenderPublishesAttemptsAndSuccessesSeparately(t *testing.T) {
	out := render(snapshot{
		model: "flagship", attempts: 3, ok: 1, slowest: 90*time.Second + 4*time.Millisecond,
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
	out := render(snapshot{model: "m", attempts: 2, ok: 0, slowest: 1500 * time.Millisecond})
	if !strings.Contains(out, `freehire_llm_probe_slowest_seconds{model="m"} 1.5`) {
		t.Errorf("slowest gauge missing or not in seconds:\n%s", out)
	}
}

// Every gauge carries HELP and TYPE. The textfile collector skips a file it cannot parse,
// so a malformed payload is not a loud failure — it is a metric that quietly stops
// existing, which reads exactly like a healthy silence.
func TestRenderCarriesHelpAndTypeForEveryGauge(t *testing.T) {
	out := render(snapshot{model: "m", attempts: 1, ok: 1, slowest: time.Second})
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
	out := render(snapshot{model: `we"ird\one`, attempts: 1, ok: 1})
	if !strings.Contains(out, `model="we\"ird\\one"`) {
		t.Errorf("model label not escaped:\n%s", out)
	}
}

// Zero successes is a measurement and must be published as one. Omitting the gauge would
// let an alert on it go stale-but-quiet rather than firing — the exact failure mode that
// let a 77%-failing alias look healthy for hours.
func TestRenderPublishesZeroSuccessesRatherThanOmittingThem(t *testing.T) {
	out := render(snapshot{model: "m", attempts: 4, ok: 0})
	if !strings.Contains(out, `freehire_llm_probe_ok{model="m"} 0`) {
		t.Errorf("a zero success count was not published:\n%s", out)
	}
}
