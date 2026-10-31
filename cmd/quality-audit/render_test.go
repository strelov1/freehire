package main

import (
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/job/qualityaudit"
	"github.com/strelov1/freehire/internal/platform/db"
)

// The names and labels are a cross-repository contract — the alert rules in freehire-ops select
// on them — so the exposition is pinned verbatim.
func TestRender(t *testing.T) {
	s := snapshot{
		liveness: map[string]qualityaudit.LivenessCounts{
			"lever":   {Sampled: 30, Live: 27, Dead: 2},
			"jobtech": {Sampled: 30},
		},
		livenessAt: time.Unix(1761900000, 0),
		remote: []db.TallyRemoteAuditJudgementsRow{
			{Source: "manatal", Sampled: 40, Confirmed: 30, Contradicted: 6, GlobalSampled: 10, GlobalConfirmed: 6, GlobalContradicted: 3},
		},
		remoteAt: time.Unix(1761903600, 0),
	}
	want := `# HELP freehire_audit_liveness_sampled Postings the latest liveness audit sampled, per source.
# TYPE freehire_audit_liveness_sampled gauge
freehire_audit_liveness_sampled{source="jobtech"} 30
freehire_audit_liveness_sampled{source="lever"} 30
# HELP freehire_audit_liveness_live Sampled postings the source answered live, on a host proven to 404 a missing posting.
# TYPE freehire_audit_liveness_live gauge
freehire_audit_liveness_live{source="jobtech"} 0
freehire_audit_liveness_live{source="lever"} 27
# HELP freehire_audit_liveness_dead Sampled postings the source answered gone (404/410), on a host proven to 404 a missing posting.
# TYPE freehire_audit_liveness_dead gauge
freehire_audit_liveness_dead{source="jobtech"} 0
freehire_audit_liveness_dead{source="lever"} 2
# HELP freehire_audit_remote_sampled Remote-tagged postings judged in the last 56 days, per source.
# TYPE freehire_audit_remote_sampled gauge
freehire_audit_remote_sampled{source="manatal"} 40
# HELP freehire_audit_remote_confirmed Judged postings whose own text says remote.
# TYPE freehire_audit_remote_confirmed gauge
freehire_audit_remote_confirmed{source="manatal"} 30
# HELP freehire_audit_remote_contradicted Judged postings whose own text says hybrid or onsite.
# TYPE freehire_audit_remote_contradicted gauge
freehire_audit_remote_contradicted{source="manatal"} 6
# HELP freehire_audit_global_sampled Judged postings that were also tagged regions global.
# TYPE freehire_audit_global_sampled gauge
freehire_audit_global_sampled{source="manatal"} 10
# HELP freehire_audit_global_confirmed Global-tagged judged postings whose own text opens them to any country.
# TYPE freehire_audit_global_confirmed gauge
freehire_audit_global_confirmed{source="manatal"} 6
# HELP freehire_audit_global_contradicted Global-tagged judged postings whose own text restricts where the candidate may work.
# TYPE freehire_audit_global_contradicted gauge
freehire_audit_global_contradicted{source="manatal"} 3
# HELP freehire_audit_measured_timestamp_seconds Unix time each audit check last measured.
# TYPE freehire_audit_measured_timestamp_seconds gauge
freehire_audit_measured_timestamp_seconds{check="liveness"} 1761900000
freehire_audit_measured_timestamp_seconds{check="remote"} 1761903600
`
	if got := render(s); got != want {
		t.Errorf("render mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// A remote check that has never judged anything publishes no remote family and no remote
// timestamp: absent, not zero — a zero would read as "measured, and found nothing".
func TestRenderWithoutJudgements(t *testing.T) {
	s := snapshot{
		liveness:   map[string]qualityaudit.LivenessCounts{"lever": {Sampled: 1, Live: 1}},
		livenessAt: time.Unix(10, 0),
	}
	want := `# HELP freehire_audit_liveness_sampled Postings the latest liveness audit sampled, per source.
# TYPE freehire_audit_liveness_sampled gauge
freehire_audit_liveness_sampled{source="lever"} 1
# HELP freehire_audit_liveness_live Sampled postings the source answered live, on a host proven to 404 a missing posting.
# TYPE freehire_audit_liveness_live gauge
freehire_audit_liveness_live{source="lever"} 1
# HELP freehire_audit_liveness_dead Sampled postings the source answered gone (404/410), on a host proven to 404 a missing posting.
# TYPE freehire_audit_liveness_dead gauge
freehire_audit_liveness_dead{source="lever"} 0
# HELP freehire_audit_measured_timestamp_seconds Unix time each audit check last measured.
# TYPE freehire_audit_measured_timestamp_seconds gauge
freehire_audit_measured_timestamp_seconds{check="liveness"} 10
`
	if got := render(s); got != want {
		t.Errorf("render mismatch\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}
