package assistant

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/tmc/langchaingo/llms"
)

func TestRepeatedRejectedEditStopsWithoutExecutingItAgain(t *testing.T) {
	calls := 0
	reg := NewRegistry(Tool{Name: "cv_edit", Run: func(context.Context, int64, json.RawMessage) (any, error) {
		calls++
		return nil, errors.New("ops must contain an edit; use tailor_report")
	}})
	model := &scriptedModel{replies: []*llms.ContentChoice{
		callReply("cv_edit", `{"note":"Leave Terraform open"}`),
		callReply("cv_edit", `{"note":"No Terraform evidence"}`),
		textReply("No CV changes were applied; Terraform remains unverified."),
	}}
	r := testRunner(model, &fakeQueries{})
	events, err := collect(t, r, Session{ID: sessionID, UserID: 3}, reg, "tailor my CV")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Fatalf("executed %d identical rejected operations, want 1", calls)
	}
	if got := lastResult(t, events).StopReason; got != "tool_loop" {
		t.Fatalf("stop reason = %q", got)
	}
	if !model.gotTextOnly[len(model.gotTextOnly)-1] {
		t.Fatal("loop must finish with a text-only request")
	}
}

func TestFailedCallCanBeCorrectedOrRetriedAfterStateChanges(t *testing.T) {
	for _, afterStateChange := range []bool{false, true} {
		t.Run(map[bool]string{false: "corrected arguments", true: "changed state"}[afterStateChange], func(t *testing.T) {
			calls := 0
			reg := NewRegistry(
				Tool{Name: "cv_edit", Run: func(context.Context, int64, json.RawMessage) (any, error) {
					calls++
					if calls == 1 {
						return nil, errors.New("missing evidence")
					}
					return "applied", nil
				}},
				Tool{Name: "experience_add", Run: func(context.Context, int64, json.RawMessage) (any, error) {
					return "banked", nil
				}},
			)
			replies := []*llms.ContentChoice{callReply("cv_edit", `{"ops":[{}]}`)}
			if afterStateChange {
				replies = append(replies, callReply("experience_add", `{}`), callReply("cv_edit", `{"ops":[{}]}`))
			} else {
				replies = append(replies, callReply("cv_edit", `{"ops":[{"evidence_id":"new"}]}`))
			}
			model := &scriptedModel{replies: append(replies, textReply("Applied the supported edit."))}
			runner := testRunner(model, &fakeQueries{})
			runner.cfg.MaxSteps = 5
			events, err := collect(t, runner, Session{ID: sessionID, UserID: 3}, reg, "edit my CV")
			if err != nil {
				t.Fatal(err)
			}
			if calls != 2 || lastResult(t, events).StopReason != StopEndTurn {
				t.Fatalf("calls = %d, result = %+v", calls, lastResult(t, events))
			}
		})
	}
}
