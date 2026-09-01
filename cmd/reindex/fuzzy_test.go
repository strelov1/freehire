package main

import (
	"reflect"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/job/jobhash"
	"github.com/strelov1/freehire/internal/platform/db"
)

// posting builds a bucket member from an id and a description.
func posting(id int64, description string) fuzzyPosting {
	return fuzzyPosting{ID: id, Signature: jobhash.DescriptionSignature(description)}
}

const (
	// A prod-like body: the localized block that differs between cities is a small fraction of it.
	bodyA = "Build and operate the payments platform with Golang, TypeScript and Postgres. " +
		"Design resilient APIs, review pull requests, instrument services with metrics and traces, " +
		"run blameless incident retrospectives, mentor junior engineers, shape the quarterly roadmap, " +
		"migrate legacy batch pipelines towards streaming, tighten database indexes and query plans, " +
		"automate deployments through continuous delivery, write architectural decision records, " +
		"improve developer tooling, reduce build times, keep documentation accurate as the platform evolves. " +
		"Our workloads run containerised across several availability zones with asynchronous messaging, " +
		"idempotent consumers and careful backpressure handling whenever traffic surges unexpectedly. " +
		"We practise trunk based development, keep feature flags short lived, prefer boring technology " +
		"where reliability matters more than novelty, and rotate engineers through customer support weeks " +
		"so operational pain reaches whoever can remove it. Compensation reviews happen twice yearly, " +
		"equipment budgets renew annually, conference attendance receives support, remote colleagues gather " +
		"quarterly for planning workshops, and internal mobility between squads remains actively encouraged."
	// A different role at the same company: shares the employer boilerplate, not the specialty.
	bodyB = "Advise retail clients on supply chain strategy, run stakeholder workshops, " +
		"prepare board level presentations, build financial models, benchmark competitors, " +
		"lead procurement negotiations, design operating models, coach client teams through change, " +
		"quantify savings opportunities, prioritise transformation initiatives, report to steering committees."
)

// The whole point: reposts whose descriptions differ only by a localized block collapse onto one
// canon, and the canon is the deterministic min(id) so re-runs agree.
func TestClusterBucket_CollapsesNearIdenticalOntoLowestID(t *testing.T) {
	got := clusterBucket([]fuzzyPosting{
		posting(42, bodyA+" Salary in Poland is quoted gross per month under Polish labour law."),
		posting(17, bodyA),
		posting(99, bodyA+" Salary in Austria is quoted gross per month under Austrian labour law."),
	}, fuzzyThreshold)

	want := map[int64]int64{42: 17, 99: 17}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("clusterBucket = %v, want %v (min id is canon, canon itself absent)", got, want)
	}
}

// Distinct roles sharing a generic title must survive as separate cards — the amazon-SDE case.
func TestClusterBucket_LeavesDistinctRolesAlone(t *testing.T) {
	got := clusterBucket([]fuzzyPosting{posting(1, bodyA), posting(2, bodyB)}, fuzzyThreshold)

	if len(got) != 0 {
		t.Errorf("clusterBucket = %v, want no merges", got)
	}
}

// Similarity is not transitive, so a chain must not drag two dissimilar ends together: only
// postings that each match the canon join its cluster.
func TestClusterBucket_DoesNotChainThroughAnIntermediate(t *testing.T) {
	mid := bodyA + " " + bodyB

	got := clusterBucket([]fuzzyPosting{posting(1, bodyA), posting(2, mid), posting(3, bodyB)}, fuzzyThreshold)

	if _, merged := got[3]; merged {
		t.Errorf("clusterBucket = %v: id 3 must not merge into id 1's cluster through the intermediate", got)
	}
}

// A description that normalizes to nothing carries no evidence, so it merges with nothing.
func TestClusterBucket_IgnoresEmptyDescriptions(t *testing.T) {
	got := clusterBucket([]fuzzyPosting{posting(1, "<p></p>"), posting(2, "<div>  </div>")}, fuzzyThreshold)

	if len(got) != 0 {
		t.Errorf("clusterBucket = %v, want no merges for empty descriptions", got)
	}
}

// Re-running over an already-collapsed bucket yields the same assignment.
func TestClusterBucket_IsIdempotent(t *testing.T) {
	in := []fuzzyPosting{posting(5, bodyA), posting(3, bodyA+" Extra local block about payroll.")}

	first := clusterBucket(in, fuzzyThreshold)
	second := clusterBucket(in, fuzzyThreshold)

	if !reflect.DeepEqual(first, second) {
		t.Errorf("re-run differs: %v vs %v", first, second)
	}
}

// A single-member bucket is the common case (median size 2, but plenty of 1s after the exact pass).
func TestClusterBucket_SingletonIsNoop(t *testing.T) {
	if got := clusterBucket([]fuzzyPosting{posting(1, bodyA)}, fuzzyThreshold); len(got) != 0 {
		t.Errorf("clusterBucket = %v, want empty", got)
	}
}

// Bucketing reuses jobhash.RoleKey, the same normalization the rest of the codebase groups roles
// by, so a city suffix or a parenthesised qualifier lands in the bucket it belongs to.
func TestBucketByRole_GroupsOnNormalizedTitle(t *testing.T) {
	judged, comparable := bucketByRole([]db.FuzzyDedupCandidateTitlesForCompanyRow{
		{ID: 1, Title: "Senior Fullstack Engineer"},
		{ID: 2, Title: "senior fullstack engineer - Kraków"},
		{ID: 3, Title: "Data Specialist"},
	})

	// One bucket is worth comparing: the two fullstack rows.
	if len(comparable) != 1 {
		t.Fatalf("got %d comparable buckets, want 1: %v", len(comparable), comparable)
	}
	for key, ids := range comparable {
		if !reflect.DeepEqual(ids, []int64{1, 2}) {
			t.Errorf("bucket %q = %v, want [1 2] (the city suffix normalizes onto the base role)", key, ids)
		}
	}

	// All three are JUDGED. "Data Specialist" has nobody to compare against, and that is a
	// verdict — "no cluster" — not a refusal: it is what releases a marker whose canon closed.
	if got := sortedIDs(judged); !reflect.DeepEqual(got, []int64{1, 2, 3}) {
		t.Errorf("judged = %v, want [1 2 3] — a singleton is decided, not skipped", got)
	}
}

// The two ways a row leaves the comparison, which must NOT be confused: a singleton is judged
// (nothing to merge with), while a bucket past the cap is skipped on cost — it is
// generic-title-by-location, which must not collapse and would cost 92% of the run. Releasing
// the latter would un-collapse the largest groups in the catalogue over a compute decision.
func TestBucketByRole_JudgesSingletonsButRefusesOversizedBuckets(t *testing.T) {
	rows := []db.FuzzyDedupCandidateTitlesForCompanyRow{{ID: 1, Title: "Solo Role"}}
	for i := 0; i < fuzzyMaxBucket+1; i++ {
		rows = append(rows, db.FuzzyDedupCandidateTitlesForCompanyRow{ID: int64(100 + i), Title: "Customer Service Associate"})
	}

	judged, comparable := bucketByRole(rows)

	if len(comparable) != 0 {
		t.Errorf("got %d comparable buckets, want 0: %v", len(comparable), comparable)
	}
	if got := sortedIDs(judged); !reflect.DeepEqual(got, []int64{1}) {
		t.Errorf("judged = %v, want only the singleton [1] — the oversized bucket keeps its markers", got)
	}
}

// A title that normalizes to nothing is not a bucket key — every blank title would otherwise
// group together and merge unrelated postings. It is still JUDGED: it cannot cluster with
// anyone, so a marker it carries must be released rather than left forever.
func TestBucketByRole_IgnoresTitlesThatNormalizeToNothing(t *testing.T) {
	judged, comparable := bucketByRole([]db.FuzzyDedupCandidateTitlesForCompanyRow{
		{ID: 1, Title: "   "},
		{ID: 2, Title: "!!!"},
	})

	if len(comparable) != 0 {
		t.Errorf("got %v, want no comparable buckets for unnormalizable titles", comparable)
	}
	if got := sortedIDs(judged); !reflect.DeepEqual(got, []int64{1, 2}) {
		t.Errorf("judged = %v, want [1 2] — unbucketable rows cannot cluster, which is a verdict", got)
	}
}

// sortedIDs makes the judged set comparable: it is built by iterating a map, so its order is
// not stable across runs.
func sortedIDs(ids []int64) []int64 {
	out := append([]int64(nil), ids...)
	slices.Sort(out)
	return out
}
