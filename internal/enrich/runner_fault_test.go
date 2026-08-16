package enrich

import (
	"context"
	"errors"
	"testing"
)

// recordingStore captures the policy each failure was recorded under, so the runner's
// classification is observable rather than inferred from a dead-letter count.
type recordingStore struct {
	fakeStore
	policies []FailurePolicy
}

func (s *recordingStore) Fail(ctx context.Context, outboxID int64, msg string, policy FailurePolicy) (bool, error) {
	s.policies = append(s.policies, policy)
	return s.fakeStore.Fail(ctx, outboxID, msg, policy)
}

// The runner must tell the store WHO is at fault, because that is what decides whether
// the entry spends its attempt budget or waits out the grace window. A gateway failure
// recorded as the posting's fault is exactly the bug that cost 172,875 postings.
func TestRunnerReportsFaultToTheStore(t *testing.T) {
	for _, tc := range []struct {
		name        string
		providerErr error
		wantAtFault bool
	}{
		{
			name:        "gateway error is not the posting's fault",
			providerErr: errors.New("API returned unexpected status code: 502"),
			wantAtFault: false,
		},
		{
			name:        "unparseable response is the posting's fault",
			providerErr: errUnparseableResponse,
			wantAtFault: true,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &recordingStore{fakeStore: fakeStore{
				claims: [][]Claimed{{{OutboxID: 1, JobID: 100, TargetVersion: Version}}},
				jobs:   map[int64]JobInput{100: {Title: "Go dev"}},
			}}
			prov := &funcProvider{fn: func(JobInput) (Enrichment, error) {
				return Enrichment{}, tc.providerErr
			}}

			o := opts()
			if _, err := (Runner{Provider: prov, Store: store}).Run(context.Background(), o); err != nil {
				t.Fatalf("Run: %v", err)
			}

			if len(store.policies) == 0 {
				t.Fatal("no failure was recorded")
			}
			got := store.policies[0]
			if got.PostingAtFault != tc.wantAtFault {
				t.Errorf("PostingAtFault = %v, want %v for %v", got.PostingAtFault, tc.wantAtFault, tc.providerErr)
			}
			if got.UpstreamGraceDays != o.UpstreamGraceDays {
				t.Errorf("UpstreamGraceDays = %d, want the configured %d", got.UpstreamGraceDays, o.UpstreamGraceDays)
			}
			if got.MaxAttempts != o.MaxAttempts {
				t.Errorf("MaxAttempts = %d, want the configured %d", got.MaxAttempts, o.MaxAttempts)
			}
		})
	}
}

// There is deliberately no runner test for a payload that fails Validate: Sanitize
// clears exactly the fields Validate checks, so after it Validate is a guard that
// cannot fire through the real path. Asserting on it would mean contriving an input
// production never produces. The classification itself is covered in fault_test.go.
