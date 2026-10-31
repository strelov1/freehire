package main

import (
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/search/serveprobe"
)

// Every (layer, field) pair is published even at zero, so a dashboard can tell "no
// gap" from "the probe never measured it".
func TestRender_PublishesEveryPairWithItsCount(t *testing.T) {
	rep := serveprobe.Report{
		Sampled: 3,
		Counts: map[serveprobe.Layer]map[string]serveprobe.Count{
			serveprobe.LayerProjection: {},
			serveprobe.LayerIndex:      {"salary": {Stored: 2, Gaps: 2}},
		},
	}
	out := render(rep)
	for _, want := range []string{
		`freehire_serve_probe_gap{layer="index",field="salary"} 2`,
		`freehire_serve_probe_stored{layer="index",field="salary"} 2`,
		`freehire_serve_probe_gap{layer="projection",field="salary"} 0`,
		`freehire_serve_probe_gap{layer="projection",field="requirements"} 0`,
		"freehire_serve_probe_sampled 3",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("render output lacks %q", want)
		}
	}
}
