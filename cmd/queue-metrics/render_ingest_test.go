package main

import (
	"strings"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

// The two counts must be published SEPARATELY, never as a ratio: the alert rule owns the
// threshold (see render.go on freehire_provider_boards), and a ratio would also have to
// invent an answer for a source that wrote nothing.
func TestRenderIngestDuplication(t *testing.T) {
	s := fullSnapshot()
	s.ingest = []ingestVolume{
		// The apploi shape: many rows standing for few postings.
		{provider: "apploi", rowsWritten: 15120, distinctPostings: 3024},
		// The ordinary shape: one row per posting.
		{provider: "greenhouse", rowsWritten: 412, distinctPostings: 412},
	}
	got := render(s)
	for _, want := range []string{
		"# HELP freehire_ingest_rows_written",
		"# TYPE freehire_ingest_rows_written gauge",
		`freehire_ingest_rows_written{provider="apploi"} 15120`,
		`freehire_ingest_rows_written{provider="greenhouse"} 412`,
		"# HELP freehire_ingest_distinct_postings",
		"# TYPE freehire_ingest_distinct_postings gauge",
		`freehire_ingest_distinct_postings{provider="apploi"} 3024`,
		`freehire_ingest_distinct_postings{provider="greenhouse"} 412`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("exposition missing %q\ngot:\n%s", want, got)
		}
	}
	if strings.Contains(got, "freehire_ingest_duplication_ratio") {
		t.Error("exposition publishes a baked ratio; the threshold belongs to the alert rule")
	}
}

// A source that wrote nothing in the window produces no row at all, so the family must be
// omitted head and all rather than emitted as a bare HELP/TYPE pair — the same rule the
// provider timestamp family follows.
func TestRenderOmitsIngestFamiliesWhenNothingWasWritten(t *testing.T) {
	got := render(fullSnapshot())
	for _, name := range []string{"freehire_ingest_rows_written", "freehire_ingest_distinct_postings"} {
		if strings.Contains(got, name) {
			t.Errorf("exposition carries %s with no sources measured", name)
		}
	}
}

// Both families must parse, and they are absent from fullSnapshot(), so the contract test
// above never validates them.
func TestRenderIngestFamiliesAreWellFormed(t *testing.T) {
	s := fullSnapshot()
	s.ingest = []ingestVolume{
		{provider: "apploi", rowsWritten: 15120, distinctPostings: 3024},
		{provider: "greenhouse", rowsWritten: 412, distinctPostings: 412},
	}
	parser := expfmt.NewTextParser(model.UTF8Validation)
	families, err := parser.TextToMetricFamilies(strings.NewReader(render(s)))
	if err != nil {
		t.Fatalf("rendered exposition does not parse: %v", err)
	}
	for _, name := range []string{"freehire_ingest_rows_written", "freehire_ingest_distinct_postings"} {
		family, ok := families[name]
		if !ok {
			t.Fatalf("parsed exposition is missing family %s", name)
		}
		if got, want := len(family.GetMetric()), len(s.ingest); got != want {
			t.Errorf("%s has %d samples, want %d", name, got, want)
		}
		if family.GetType() != dto.MetricType_GAUGE {
			t.Errorf("%s parsed as %v, want GAUGE", name, family.GetType())
		}
	}
}

// A provider that has never succeeded has no timestamp, so first-listed is the only age it
// has — and it must be published, or every age-based rule loses it exactly when it is most
// broken. gupy, 2026-10-19: 3,324 freshly re-keyed boards, none crawled, no series at all.
func TestRenderFirstListedCoversNeverSucceeded(t *testing.T) {
	s := fullSnapshot()
	s.providers = []providerHealth{
		{name: "gupy", firstListed: time.Unix(1792436951, 0), uncrawled: 3324},
	}
	got := render(s)
	if strings.Contains(got, "freehire_provider_last_success_timestamp_seconds") {
		t.Errorf("a provider with no success must publish no timestamp\ngot:\n%s", got)
	}
	for _, want := range []string{
		`freehire_provider_first_listed_timestamp_seconds{provider="gupy"} 1792436951`,
		`freehire_provider_boards{provider="gupy",state="uncrawled"} 3324`,
		`freehire_boards_total{state="uncrawled"} 3324`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("exposition missing %q\ngot:\n%s", want, got)
		}
	}
}

// The fleet-wide family is the per-provider one summed, so a dashboard stacking the two
// cannot disagree with itself. Checked through the parser over the published text, which is
// what a consumer actually adds up.
func TestRenderFleetIsTheSumOfProviders(t *testing.T) {
	parser := expfmt.NewTextParser(model.UTF8Validation)
	families, err := parser.TextToMetricFamilies(strings.NewReader(render(fullSnapshot())))
	if err != nil {
		t.Fatalf("rendered exposition does not parse: %v", err)
	}
	perState := map[string]float64{}
	for _, m := range families["freehire_provider_boards"].GetMetric() {
		for _, l := range m.GetLabel() {
			if l.GetName() == "state" {
				perState[l.GetValue()] += m.GetGauge().GetValue()
			}
		}
	}
	fleet := families["freehire_boards_total"].GetMetric()
	if len(fleet) != 4 {
		t.Fatalf("freehire_boards_total has %d samples, want 4", len(fleet))
	}
	for _, m := range fleet {
		state := m.GetLabel()[0].GetValue()
		if got, want := m.GetGauge().GetValue(), perState[state]; got != want {
			t.Errorf("freehire_boards_total{state=%q} = %v, providers sum to %v", state, got, want)
		}
	}
}

// No provider at all still publishes the fleet family as explicit zeros: the rules read an
// absent series as a dead exporter, and an empty catalogue is not one.
func TestRenderPublishesAnEmptyFleetAsZeroes(t *testing.T) {
	s := fullSnapshot()
	s.providers = nil
	got := render(s)
	for _, st := range []string{"healthy", "failing", "cooled", "uncrawled"} {
		if want := `freehire_boards_total{state="` + st + `"} 0`; !strings.Contains(got, want) {
			t.Errorf("exposition missing %q\ngot:\n%s", want, got)
		}
	}
	if strings.Contains(got, "freehire_provider_first_listed_timestamp_seconds") {
		t.Errorf("first-listed family must be omitted with no providers\ngot:\n%s", got)
	}
}

func qualitySnapshot() snapshot {
	s := fullSnapshot()
	measured := time.Unix(1792000000, 0)
	s.sources = []sourceQuality{
		// The spike's two findings: a source with no geography at all, and one whose
		// crawl stopped listing anything while its postings stayed open.
		{source: "hrmdirect", openJobs: 90000, gaps: [len(qualityChecks)]int64{80000, 3000, 0, 90000, 1200}, measuredAt: measured},
		{source: "successfactors", openJobs: 488000, gaps: [len(qualityChecks)]int64{400000, 476000, 0, 5800, 0}, measuredAt: measured},
	}
	return s
}

// Counts, never shares, and one family keyed by check: the alert rule divides by
// freehire_source_open_jobs and owns the threshold, the argument
// TestRenderIngestDuplication makes for the ingest pair.
func TestRenderSourceQuality(t *testing.T) {
	got := render(qualitySnapshot())
	for _, want := range []string{
		`freehire_source_open_jobs{source="successfactors"} 488000`,
		`freehire_source_quality_gap{source="hrmdirect",check="missing_work_mode"} 80000`,
		`freehire_source_quality_gap{source="successfactors",check="missing_geo"} 476000`,
		`freehire_source_quality_gap{source="successfactors",check="missing_company"} 0`,
		`freehire_source_quality_gap{source="hrmdirect",check="unseen_7d"} 90000`,
		`freehire_source_quality_gap{source="hrmdirect",check="posted_over_60d"} 1200`,
		"freehire_source_stats_measured_timestamp_seconds 1792000000",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("exposition missing %q\ngot:\n%s", want, got)
		}
	}
}

// No snapshot yet (a fresh install, or the migration landed before the first rollup):
// nothing was measured, so nothing is published — not a fleet of zeros.
func TestRenderOmitsSourceQualityWithoutASnapshot(t *testing.T) {
	got := render(fullSnapshot())
	for _, name := range []string{"freehire_source_open_jobs", "freehire_source_quality_gap", "freehire_source_stats_measured"} {
		if strings.Contains(got, name) {
			t.Errorf("exposition carries %s with no snapshot", name)
		}
	}
}

func TestRenderSourceQualityIsWellFormed(t *testing.T) {
	s := qualitySnapshot()
	parser := expfmt.NewTextParser(model.UTF8Validation)
	families, err := parser.TextToMetricFamilies(strings.NewReader(render(s)))
	if err != nil {
		t.Fatalf("rendered exposition does not parse: %v", err)
	}
	for name, samples := range map[string]int{
		"freehire_source_open_jobs":                        len(s.sources),
		"freehire_source_quality_gap":                      len(s.sources) * len(qualityChecks),
		"freehire_source_stats_measured_timestamp_seconds": 1,
	} {
		family, ok := families[name]
		if !ok {
			t.Fatalf("parsed exposition is missing family %s", name)
		}
		if got := len(family.GetMetric()); got != samples {
			t.Errorf("%s has %d samples, want %d", name, got, samples)
		}
		if family.GetType() != dto.MetricType_GAUGE {
			t.Errorf("%s parsed as %v, want GAUGE", name, family.GetType())
		}
	}
}
