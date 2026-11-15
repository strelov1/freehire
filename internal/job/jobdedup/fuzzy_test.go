package jobdedup

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/job/jobhash"
	"github.com/strelov1/freehire/internal/platform/db"
)

// posting builds a bucket member from an id and a description.
func posting(id int64, description string) FuzzyPosting {
	return FuzzyPosting{ID: id, Signature: jobhash.DescriptionSignature(description)}
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
	got := ClusterBucket([]FuzzyPosting{
		posting(42, bodyA+" Salary in Poland is quoted gross per month under Polish labour law."),
		posting(17, bodyA),
		posting(99, bodyA+" Salary in Austria is quoted gross per month under Austrian labour law."),
	}, FuzzyThreshold)

	want := map[int64]int64{42: 17, 99: 17}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ClusterBucket = %v, want %v (min id is canon, canon itself absent)", got, want)
	}
}

// Distinct roles sharing a generic title must survive as separate cards — the amazon-SDE case.
func TestClusterBucket_LeavesDistinctRolesAlone(t *testing.T) {
	got := ClusterBucket([]FuzzyPosting{posting(1, bodyA), posting(2, bodyB)}, FuzzyThreshold)

	if len(got) != 0 {
		t.Errorf("ClusterBucket = %v, want no merges", got)
	}
}

// Similarity is not transitive, so a chain must not drag two dissimilar ends together: only
// postings that each match the canon join its cluster.
func TestClusterBucket_DoesNotChainThroughAnIntermediate(t *testing.T) {
	mid := bodyA + " " + bodyB

	got := ClusterBucket([]FuzzyPosting{posting(1, bodyA), posting(2, mid), posting(3, bodyB)}, FuzzyThreshold)

	if _, merged := got[3]; merged {
		t.Errorf("ClusterBucket = %v: id 3 must not merge into id 1's cluster through the intermediate", got)
	}
}

// A description that normalizes to nothing carries no evidence, so it merges with nothing.
func TestClusterBucket_IgnoresEmptyDescriptions(t *testing.T) {
	got := ClusterBucket([]FuzzyPosting{posting(1, "<p></p>"), posting(2, "<div>  </div>")}, FuzzyThreshold)

	if len(got) != 0 {
		t.Errorf("ClusterBucket = %v, want no merges for empty descriptions", got)
	}
}

// Re-running over an already-collapsed bucket yields the same assignment.
func TestClusterBucket_IsIdempotent(t *testing.T) {
	in := []FuzzyPosting{posting(5, bodyA), posting(3, bodyA+" Extra local block about payroll.")}

	first := ClusterBucket(in, FuzzyThreshold)
	second := ClusterBucket(in, FuzzyThreshold)

	if !reflect.DeepEqual(first, second) {
		t.Errorf("re-run differs: %v vs %v", first, second)
	}
}

// A single-member bucket is the common case (median size 2, but plenty of 1s after the exact pass).
func TestClusterBucket_SingletonIsNoop(t *testing.T) {
	if got := ClusterBucket([]FuzzyPosting{posting(1, bodyA)}, FuzzyThreshold); len(got) != 0 {
		t.Errorf("ClusterBucket = %v, want empty", got)
	}
}

// Bucketing reuses jobhash.RoleKey, the same normalization the rest of the codebase groups roles
// by, so a city suffix or a parenthesised qualifier lands in the bucket it belongs to.
func TestBucketByRole_GroupsOnNormalizedTitle(t *testing.T) {
	judged, comparable := BucketByRole([]db.FuzzyDedupCandidateTitlesForCompanyRow{
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
	for i := 0; i < FuzzyMaxBucket+1; i++ {
		rows = append(rows, db.FuzzyDedupCandidateTitlesForCompanyRow{ID: int64(100 + i), Title: "Customer Service Associate"})
	}

	judged, comparable := BucketByRole(rows)

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
	judged, comparable := BucketByRole([]db.FuzzyDedupCandidateTitlesForCompanyRow{
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

// fakeFuzzyQuerier serves CanonicalForFuzzy from memory: rows are the candidate titles, bodies the descriptions.
type fakeFuzzyQuerier struct {
	rows      []db.FuzzyDedupCandidateTitlesForCompanyCappedRow
	bodies    map[int64]string
	rowsErr   error
	bodiesErr error
	asked     []int64
}

func (f *fakeFuzzyQuerier) FuzzyDedupCandidateTitlesForCompanyCapped(_ context.Context, arg db.FuzzyDedupCandidateTitlesForCompanyCappedParams) ([]db.FuzzyDedupCandidateTitlesForCompanyCappedRow, error) {
	if f.rowsErr != nil {
		return nil, f.rowsErr
	}
	if len(f.rows) > int(arg.MaxRows) {
		return f.rows[:arg.MaxRows], nil
	}
	return f.rows, nil
}

func (f *fakeFuzzyQuerier) GetJobDescriptionsByIDs(_ context.Context, ids []int64) ([]db.GetJobDescriptionsByIDsRow, error) {
	f.asked = ids
	if f.bodiesErr != nil {
		return nil, f.bodiesErr
	}
	out := make([]db.GetJobDescriptionsByIDsRow, 0, len(ids))
	for _, id := range ids {
		out = append(out, db.GetJobDescriptionsByIDsRow{ID: id, Description: f.bodies[id]})
	}
	return out, nil
}

func titled(title string, ids ...int64) []db.FuzzyDedupCandidateTitlesForCompanyCappedRow {
	rows := make([]db.FuzzyDedupCandidateTitlesForCompanyCappedRow, 0, len(ids))
	for _, id := range ids {
		rows = append(rows, db.FuzzyDedupCandidateTitlesForCompanyCappedRow{ID: id, Title: title})
	}
	return rows
}

func TestCanonicalForFuzzy(t *testing.T) {
	const title = "Senior Go Engineer"
	params := db.UpsertJobParams{CompanySlug: "acme", Title: title}
	repost := bodyA + " Salary in Poland is quoted gross per month under Polish labour law."
	boom := errors.New("boom")

	tests := []struct {
		name      string
		params    db.UpsertJobParams
		q         *fakeFuzzyQuerier
		id        int64
		wantCanon int64
		wantOK    bool
		wantErr   bool
	}{
		{
			name:   "joins the older canon",
			params: params,
			q:      &fakeFuzzyQuerier{rows: titled(title, 3, 7, 9), bodies: map[int64]string{3: bodyA, 7: repost}},
			id:     7, wantCanon: 3, wantOK: true,
		},
		{
			name:   "different description stays a canon",
			params: params,
			q:      &fakeFuzzyQuerier{rows: titled(title, 3, 7), bodies: map[int64]string{3: bodyB, 7: bodyA}},
			id:     7,
		},
		{name: "no company", params: db.UpsertJobParams{Title: title}, q: &fakeFuzzyQuerier{}, id: 7},
		{name: "title normalizes to nothing", params: db.UpsertJobParams{CompanySlug: "acme"}, q: &fakeFuzzyQuerier{}, id: 7},
		{
			name:   "no older sibling",
			params: params,
			q:      &fakeFuzzyQuerier{rows: append(titled(title, 7, 9), titled("Accountant", 1)...)},
			id:     7,
		},
		{
			name:   "oversized bucket is not judged",
			params: params,
			q:      &fakeFuzzyQuerier{rows: titled(title, seq(1, FuzzyMaxBucket+1)...)},
			id:     2,
		},
		{
			name:   "oversized company is left to the batch",
			params: params,
			q:      &fakeFuzzyQuerier{rows: titled("Accountant", seq(1, FuzzySyncMaxCandidates+1)...)},
			id:     2,
		},
		{name: "candidate read fails", params: params, q: &fakeFuzzyQuerier{rowsErr: boom}, id: 7, wantErr: true},
		{
			name:   "description read fails",
			params: params,
			q:      &fakeFuzzyQuerier{rows: titled(title, 3, 7), bodiesErr: boom},
			id:     7, wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			canon, ok, err := CanonicalForFuzzy(context.Background(), tt.q, tt.params, tt.id)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if canon != tt.wantCanon || ok != tt.wantOK {
				t.Errorf("CanonicalForFuzzy = (%d, %v), want (%d, %v)", canon, ok, tt.wantCanon, tt.wantOK)
			}
		})
	}
}

// A posting already claimed by an earlier canon is not re-offered to a later one.
func TestClusterBucket_SkipsPostingsAnEarlierCanonClaimed(t *testing.T) {
	got := ClusterBucket([]FuzzyPosting{posting(1, bodyA), posting(2, bodyB), posting(3, bodyA)}, FuzzyThreshold)
	if want := map[int64]int64{3: 1}; !reflect.DeepEqual(got, want) {
		t.Errorf("ClusterBucket = %v, want %v", got, want)
	}
}

func seq(from, to int64) []int64 {
	ids := make([]int64, 0, to-from+1)
	for id := from; id <= to; id++ {
		ids = append(ids, id)
	}
	return ids
}
