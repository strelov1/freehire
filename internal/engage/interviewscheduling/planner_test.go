package interviewscheduling

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/platform/llm"
)

func TestSelectedTimeMustBeSupportedByRecruiterEvidence(t *testing.T) {
	first := time.Date(2026, 12, 4, 11, 0, 0, 0, time.UTC)
	second := first.Add(time.Hour)
	offers := []time.Time{first, second}
	cases := []struct {
		body     string
		decision Decision
		want     time.Time
	}{
		{"Thanks. The first option works for me. Please confirm the interview.",
			Decision{Action: Accept, Choice: 1, Evidence: "The first option works for me."}, first},
		{"Option 1 works for me.", Decision{Action: Accept, Choice: 1, Evidence: "Option 1 works for me."}, first},
		{"Option 1 does not work for me!", Decision{Action: Accept, Choice: 1, Evidence: "Option 1 does not work for me!"}, time.Time{}},
		{"The first option works for you. I cannot take the first option!", Decision{Action: Accept, Choice: 1, Evidence: "first option!"}, time.Time{}},
		{"Option 1 works at 11 a.m.", Decision{Action: Accept, Choice: 1, Evidence: "Option 1 works at 11 a.m."}, first},
		{"Option 1 does not work at 11 a.m.", Decision{Action: Accept, Choice: 1, Evidence: "Option 1 does not work at 11 a.m."}, time.Time{}},
		{"Does option 1 work?", Decision{Action: Accept, Choice: 1, Evidence: "Does option 1 work?"}, time.Time{}},
		{"The second option works for me.", Decision{Action: Accept, Choice: 2, Evidence: "second option"}, second},
		{"The second option does not work for me. The first option works.", Decision{Action: Accept, Choice: 2, Evidence: "second option"}, time.Time{}},
		{"The first option works for me.", Decision{Action: Accept, Choice: 2, Evidence: "first option"}, time.Time{}},
		{"Friday Dec 4 at 11:00 UTC works.", Decision{
			Action: Propose, StartsAt: first, Evidence: "Dec 4 at 11:00 UTC",
		}, first},
		{"Friday Dec 4 at 11:00 UTC works.", Decision{
			Action: Propose, StartsAt: second, Evidence: "Dec 4 at 11:00 UTC",
		}, time.Time{}},
		{"Friday Dec 4 at 11:00 UTC works.", Decision{
			Action: Propose, StartsAt: first, Evidence: "Dec 11 at 11:00 UTC",
		}, time.Time{}},
	}
	for _, tc := range cases {
		if got := selectedTime(tc.body, tc.decision, offers, time.UTC); !got.Equal(tc.want) {
			t.Errorf("%q: selected %v, want %v", tc.body, got, tc.want)
		}
	}
}

type decisionGenerator func(context.Context, string, string, ...llm.GenOption) (string, error)

func (f decisionGenerator) GenerateJSON(ctx context.Context, system, user string, opts ...llm.GenOption) (string, error) {
	return f(ctx, system, user, opts...)
}

func TestPlannerChecksModelEvidenceAgainstLatestEmail(t *testing.T) {
	input := plannerInput{Subject: "Interview invitation", Body: "Can we schedule an interview?"}
	for _, tc := range []struct {
		name     string
		response string
		modelErr error
		want     Action
		wantErr  bool
	}{
		{name: "verbatim evidence", response: `{"action":"offer","evidence":"schedule an interview","language":"en"}`, want: Offer},
		{name: "unsupported evidence", response: `{"action":"offer","evidence":"Monday morning"}`, want: Review},
		{name: "missing evidence", response: `{"action":"offer"}`, want: Review},
		{name: "invalid output", response: `{"action":`, wantErr: true},
		{name: "model unavailable", modelErr: errors.New("unavailable"), wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			model := decisionGenerator(func(context.Context, string, string, ...llm.GenOption) (string, error) {
				return tc.response, tc.modelErr
			})
			got, err := readDecision(t.Context(), model, input)
			if got.Action != tc.want || (err != nil) != tc.wantErr {
				t.Fatalf("decision = %v, err = %v, want action %q and error %t", got, err, tc.want, tc.wantErr)
			}
		})
	}
}
