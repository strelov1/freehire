package main

import (
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/job/qualityaudit"
	"github.com/strelov1/freehire/internal/platform/db"
)

// snapshot is what one run publishes: this run's liveness tally, and the remote judgements of
// the trailing window as the database holds them.
type snapshot struct {
	liveness   map[string]qualityaudit.LivenessCounts
	livenessAt time.Time
	remote     []db.TallyRemoteAuditJudgementsRow
	// remoteAt is when the remote check last judged anything; zero when it never has.
	remoteAt time.Time
}

// render turns a snapshot into the Prometheus text exposition format.
//
// Every value is a raw count. The share that counts as "too many dead" or "too few honest" is
// the alert rule's to decide, and so is the smallest sample worth deciding on — a source whose
// live + dead is 0 was not measurable this week, and only a rule that reads both counts can
// tell that apart from a healthy one.
//
// The names and labels are a cross-repository contract (the alert rules live in freehire-ops),
// which render_test.go pins verbatim.
func render(s snapshot) string {
	var b strings.Builder

	sources := slices.Sorted(maps.Keys(s.liveness))
	liveness := []struct {
		name, help string
		value      func(qualityaudit.LivenessCounts) int64
	}{
		{"freehire_audit_liveness_sampled", "Postings the latest liveness audit sampled, per source.",
			func(c qualityaudit.LivenessCounts) int64 { return c.Sampled }},
		{"freehire_audit_liveness_live", "Sampled postings the source answered live, on a host proven to 404 a missing posting.",
			func(c qualityaudit.LivenessCounts) int64 { return c.Live }},
		{"freehire_audit_liveness_dead", "Sampled postings the source answered gone (404/410), on a host proven to 404 a missing posting.",
			func(c qualityaudit.LivenessCounts) int64 { return c.Dead }},
	}
	if len(sources) > 0 {
		for _, f := range liveness {
			header(&b, f.name, f.help)
			for _, src := range sources {
				fmt.Fprintf(&b, "%s{source=%q} %d\n", f.name, src, f.value(s.liveness[src]))
			}
		}
	}

	// A source with no judgement in the window is absent, not zero: zero confirmed out of zero
	// sampled is no measurement, and a zero series would read as one.
	remote := []struct {
		name, help string
		value      func(db.TallyRemoteAuditJudgementsRow) int64
	}{
		{"freehire_audit_remote_sampled", "Remote-tagged postings judged in the last 56 days, per source.",
			func(r db.TallyRemoteAuditJudgementsRow) int64 { return r.Sampled }},
		{"freehire_audit_remote_confirmed", "Judged postings whose own text says remote.",
			func(r db.TallyRemoteAuditJudgementsRow) int64 { return r.Confirmed }},
		{"freehire_audit_remote_contradicted", "Judged postings whose own text says hybrid or onsite.",
			func(r db.TallyRemoteAuditJudgementsRow) int64 { return r.Contradicted }},
		{"freehire_audit_global_sampled", "Judged postings that were also tagged regions global.",
			func(r db.TallyRemoteAuditJudgementsRow) int64 { return r.GlobalSampled }},
		{"freehire_audit_global_confirmed", "Global-tagged judged postings whose own text opens them to any country.",
			func(r db.TallyRemoteAuditJudgementsRow) int64 { return r.GlobalConfirmed }},
		{"freehire_audit_global_contradicted", "Global-tagged judged postings whose own text restricts where the candidate may work.",
			func(r db.TallyRemoteAuditJudgementsRow) int64 { return r.GlobalContradicted }},
	}
	if len(s.remote) > 0 {
		for _, f := range remote {
			header(&b, f.name, f.help)
			for _, r := range s.remote {
				fmt.Fprintf(&b, "%s{source=%q} %d\n", f.name, r.Source, f.value(r))
			}
		}
	}

	const ts = "freehire_audit_measured_timestamp_seconds"
	header(&b, ts, "Unix time each audit check last measured.")
	fmt.Fprintf(&b, "%s{check=%q} %d\n", ts, "liveness", s.livenessAt.Unix())
	if !s.remoteAt.IsZero() {
		fmt.Fprintf(&b, "%s{check=%q} %d\n", ts, "remote", s.remoteAt.Unix())
	}
	return b.String()
}

func header(b *strings.Builder, name, help string) {
	fmt.Fprintf(b, "# HELP %s %s\n# TYPE %s gauge\n", name, help, name)
}
