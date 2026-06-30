package userjob

import "testing"

func sumBuckets(b BucketCounts) int64 {
	return b.NoAnswer + b.InProgress + b.Interviewing + b.Offer + b.Accepted + b.Rejected + b.Declined
}

func TestAggregateMapsStagesToBuckets(t *testing.T) {
	got := Aggregate([]StageCount{
		{Stage: "applied", Count: 70},
		{Stage: "screening", Count: 5},
		{Stage: "responded", Count: 13},
		{Stage: "interview", Count: 20},
		{Stage: "offer", Count: 2},
		{Stage: "accepted", Count: 1},
		{Stage: "rejected", Count: 8},
		{Stage: "withdrawn", Count: 1},
	})

	want := BucketCounts{
		NoAnswer:     70,
		InProgress:   18, // screening + responded
		Interviewing: 20,
		Offer:        2,
		Accepted:     1,
		Rejected:     8,
		Declined:     1, // withdrawn
	}
	if got.Buckets != want {
		t.Errorf("Buckets = %+v, want %+v", got.Buckets, want)
	}
	if got.Applications != 120 {
		t.Errorf("Applications = %d, want 120", got.Applications)
	}
}

func TestAggregateBucketsSumToApplications(t *testing.T) {
	got := Aggregate([]StageCount{
		{Stage: "applied", Count: 3},
		{Stage: "interview", Count: 4},
		{Stage: "rejected", Count: 5},
	})
	if sum := sumBuckets(got.Buckets); sum != got.Applications {
		t.Errorf("buckets sum = %d, want Applications = %d", sum, got.Applications)
	}
}

func TestAggregateNullStageIsNoAnswer(t *testing.T) {
	// An applied row with no explicit stage arrives as an empty Stage.
	got := Aggregate([]StageCount{{Stage: "", Count: 6}})
	if got.Buckets.NoAnswer != 6 {
		t.Errorf("NoAnswer = %d, want 6", got.Buckets.NoAnswer)
	}
	if got.Applications != 6 {
		t.Errorf("Applications = %d, want 6", got.Applications)
	}
}

func TestAggregateEmptyInput(t *testing.T) {
	got := Aggregate(nil)
	if got != (Pipeline{}) {
		t.Errorf("Aggregate(nil) = %+v, want zero Pipeline", got)
	}
}

func TestAggregateUnknownStageIsCountedAndDeterministic(t *testing.T) {
	// An out-of-vocabulary stage must still be counted as an application and land
	// in a bucket, so the buckets always sum to Applications.
	got := Aggregate([]StageCount{{Stage: "bogus", Count: 4}})
	if got.Applications != 4 {
		t.Errorf("Applications = %d, want 4", got.Applications)
	}
	if sum := sumBuckets(got.Buckets); sum != 4 {
		t.Errorf("buckets sum = %d, want 4", sum)
	}
}
