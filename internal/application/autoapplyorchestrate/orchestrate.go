// Package autoapplyorchestrate holds the one Inngest function
// cmd/auto-apply-orchestrate serves: a durable, event-triggered sequence that calls
// freehire's own auto-apply tailoring trigger, then pauses for however long the
// candidate's review decision takes (surviving a process restart). It does NOT call the
// review endpoint itself: EventReviewDecided is published by PostAutoApplyReview only
// AFTER it has already durably recorded the decision (internal/api/handler's own
// publishReviewDecided runs after the write), so by the time this function's own
// WaitForEvent resumes, there is nothing left to record — a second call would always 409
// (already reviewed). The run simply completes with the decision the event carried.
//
// See openspec/changes/auto-apply-inngest-orchestration/design.md — in particular why this
// package calls the existing HTTP endpoints rather than importing internal/candidate/cv or
// internal/ai/assistant directly, and why it uses a plain http.Client rather than
// internal/platform/safehttp (that package's guard BLOCKS the internal address this client
// must reach; it exists for outbound fetches of attacker-influenced URLs, which this is
// not).
package autoapplyorchestrate

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/inngest/inngestgo"
	"github.com/inngest/inngestgo/step"
)

// EventSubmit starts one durable run per auto-apply queue entry submission. Not yet
// published anywhere in production — see auto-apply-tailored-resume's own tasks.md item
// 2.1, the still-deferred trigger.
const EventSubmit = "auto-apply/submit"

// EventReviewDecided resumes a run paused awaiting the candidate's decision.
// internal/api/handler's PostAutoApplyReview publishes it.
const EventReviewDecided = "auto-apply/review.decided"

// FunctionID names the one Inngest function this package registers.
const FunctionID = "auto-apply-tailor-and-review"

// ReviewWaitTimeout bounds how long a run may sit paused awaiting the candidate's
// decision before ending without one (spec: "without a wall-clock upper bound shorter
// than several days"). A candidate under no particular pressure still has real time to
// decide.
const ReviewWaitTimeout = 7 * 24 * time.Hour

// hireRequestTimeout bounds ONE call into hire's own API. Tailoring is a real,
// potentially slow LLM-backed run — internal/api/handler's own assistantLLMTimeout bounds
// a single model call at 180s, and a tailoring pass can make several; this is generous
// past the whole run.
const hireRequestTimeout = 5 * time.Minute

// SubmitEvent is EventSubmit's own data: which queue entry to run.
//
// QueueID is a STRING even though auto_apply_queue.id is bigint: it is compared inside a
// WaitForEvent CEL "if" expression as a quoted literal (see the wait step below), and this
// avoids the whole question of how that evaluator widens a JSON number — a question this
// session's own verified spike sidestepped the same way, deliberately kept rather than
// "fixed" to int64 without re-verifying that path.
type SubmitEvent struct {
	QueueID string `json:"queueId"`
}

// ReviewDecidedEvent is EventReviewDecided's own data: the queue entry a decision was
// just recorded for, and what it was ("approved" or "declined" — see
// internal/api/handler's own autoApplyReviewApproved/autoApplyReviewDeclined).
type ReviewDecidedEvent struct {
	QueueID  string `json:"queueId"`
	Decision string `json:"decision"`
}

// Config wires the function to freehire's own API.
type Config struct {
	// HireBaseURL is freehire's own API base, e.g. "http://127.0.0.1:8080/api/v1" — an
	// internal address.
	HireBaseURL string
	// Secret is presented as `Authorization: Bearer <Secret>` on every call into hire —
	// must match the server's own AUTO_APPLY_ORCHESTRATOR_SECRET
	// (internal/api/handler's autoApplyOrchestratorGate).
	Secret string
	// HTTPClient is the client used for calls into hire. Nil defaults to one bounded by
	// hireRequestTimeout.
	HTTPClient *http.Client
	// ReviewWaitTimeout overrides ReviewWaitTimeout's own default. Zero means the
	// default; the override exists so a test can shrink a multi-day wait to something
	// it can actually observe elapse.
	ReviewWaitTimeout time.Duration
}

func (cfg Config) httpClient() *http.Client {
	if cfg.HTTPClient != nil {
		return cfg.HTTPClient
	}
	return &http.Client{Timeout: hireRequestTimeout}
}

func (cfg Config) reviewWaitTimeout() time.Duration {
	if cfg.ReviewWaitTimeout > 0 {
		return cfg.ReviewWaitTimeout
	}
	return ReviewWaitTimeout
}

// callHire POSTs to one of the two auto-apply routes and reports a non-2xx status as an
// error, so a refusal ends the calling step (and, for the tailor call, the whole run)
// rather than being treated as success.
func callHire(ctx context.Context, cfg Config, path string, body any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cfg.HireBaseURL+path, reader)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+cfg.Secret)

	resp, err := cfg.httpClient().Do(req)
	if err != nil {
		return fmt.Errorf("POST %s: %w", path, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("POST %s: status %d: %s", path, resp.StatusCode, string(b))
	}
	return nil
}

// Register creates the durable tailor-then-review Inngest function on client, wired to
// call cfg.HireBaseURL's own two auto-apply routes. client.Serve() (or ServeWithOpts)
// picks up every function created against it, so the caller need only call Register once
// per client before serving.
func Register(client inngestgo.Client, cfg Config) (inngestgo.ServableFunction, error) {
	return inngestgo.CreateFunction(
		client,
		inngestgo.FunctionOpts{ID: FunctionID, Name: "Auto-apply tailor and review"},
		inngestgo.EventTrigger(EventSubmit, nil),
		func(ctx context.Context, in inngestgo.Input[SubmitEvent]) (any, error) {
			queueID := in.Event.Data.QueueID

			_, err := step.Run(ctx, "tailor", func(ctx context.Context) (any, error) {
				return nil, callHire(ctx, cfg, "/me/auto-apply/"+queueID+"/tailor", nil)
			})
			if err != nil {
				// The entry stays exactly where the tailor endpoint itself already left
				// it on failure — no review call, no retry from here.
				return nil, fmt.Errorf("tailor queue entry %s: %w", queueID, err)
			}

			decided, err := step.WaitForEvent[inngestgo.GenericEvent[ReviewDecidedEvent]](
				ctx, "wait-for-review-decision", step.WaitForEventOpts{
					Event:   EventReviewDecided,
					Timeout: cfg.reviewWaitTimeout(),
					// "async" (not "event") is the CEL variable Inngest's own executor
					// binds the candidate event to when evaluating a WaitForEvent "if" —
					// confirmed against a real dev server: "event.data..." (this
					// session's own earlier spike, and this package's own first draft)
					// silently matched EVERY event of the right name rather than failing
					// to parse, which this package's own negative-case integration test
					// (orchestrate_integration_test.go) caught.
					If: inngestgo.StrPtr(fmt.Sprintf("async.data.queueId == %q", queueID)),
				},
			)
			if err != nil {
				if errors.Is(err, step.ErrEventNotReceived) {
					// The pause exceeded its own bound: end the run without a review call
					// and without retrying tailor (spec's own "failed or timed-out step"
					// requirement) — a clean completion, not a failure, since nobody
					// responding in time is an expected outcome, not a bug.
					return map[string]any{"status": "review_wait_timed_out", "queueId": queueID}, nil
				}
				return nil, fmt.Errorf("wait for review decision on queue entry %s: %w", queueID, err)
			}

			// No further HTTP call: PostAutoApplyReview only ever publishes
			// EventReviewDecided after its own write already recorded the decision (see
			// this package's own doc comment), so this run's only remaining job is to
			// note the outcome it was told.
			return map[string]any{"status": "reviewed", "queueId": queueID, "decision": decided.Data.Decision}, nil
		},
	)
}
