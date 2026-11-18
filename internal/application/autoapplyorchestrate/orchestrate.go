// Package autoapplyorchestrate holds the one Inngest function cmd/auto-apply-orchestrate
// serves: a durable, event-triggered sequence that calls freehire's auto-apply tailoring
// trigger, then pauses (surviving a restart) for the candidate's review decision. It does
// NOT call the review endpoint: PostAutoApplyReview publishes EventReviewDecided only after
// durably recording the decision, so a second call would always 409. The run completes with
// the decision the event carried.
//
// It calls the existing HTTP endpoints rather than importing internal/candidate/cv or
// internal/ai/assistant, and uses a plain http.Client because internal/platform/safehttp
// would BLOCK the internal address this client must reach.
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
	inngesterrors "github.com/inngest/inngestgo/errors"
	"github.com/inngest/inngestgo/step"
)

// EventSubmit starts one durable run per auto-apply queue entry submission.
const EventSubmit = "auto-apply/submit"

// EventReviewDecided resumes a run paused awaiting the candidate's decision.
// internal/api/handler's PostAutoApplyReview publishes it.
const EventReviewDecided = "auto-apply/review.decided"

// FunctionID names the one Inngest function this package registers.
const FunctionID = "auto-apply-tailor-and-review"

// ReviewWaitTimeout bounds how long a run may sit paused awaiting the candidate's decision
// before ending without one; several days, so a candidate under no pressure can decide.
const ReviewWaitTimeout = 7 * 24 * time.Hour

// hireRequestTimeout bounds ONE call into hire's own API. It must sit ABOVE the tailoring
// endpoint's run budget (internal/api/handler's autoApplyTailorBudget, 10 minutes) so a run
// always ends on its own terms: a tailoring pass can make up to autopilotMaxSteps (30) model
// calls of up to assistantLLMTimeout (180s) each, and a shorter timeout abandons a run that
// keeps working, leaving the entry never told. The layering guard forbids importing the
// handler package to state this in code: raise the budget and this must follow.
const hireRequestTimeout = 12 * time.Minute

// SubmitEvent is EventSubmit's data: which queue entry to run.
//
// QueueID is a STRING though auto_apply_queue.id is bigint: it is compared in a WaitForEvent
// CEL "if" expression as a quoted literal, which avoids how that evaluator widens a JSON
// number. Do not change it to int64 without re-verifying that path.
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
		return responseError(resp.StatusCode, path, string(b))
	}
	return nil
}

// busyRetryDelay is how long a step waits after hire says the session is busy: long enough
// to outlast the run it collided with (a tailoring run can take several minutes), short
// enough that a candidate is not left on a stalled queue.
const busyRetryDelay = 8 * time.Minute

// responseError turns a non-2xx into the error the step returns, and decides whether
// Inngest treats it as a failure or as "come back later".
//
// 409 from the tailoring route is hire's guard against two tailoring runs on one session:
// the earlier run is still going, so a retry only fails again at once and burns the
// attempts. Every other status keeps its ordinary meaning: a 500 is a failed run and spends
// a retry; a 404 is a wrong route retrying cannot fix.
func responseError(status int, path, body string) error {
	err := fmt.Errorf("POST %s: status %d: %s", path, status, body)
	if status == http.StatusConflict {
		return inngestgo.RetryAtError(err, time.Now().Add(busyRetryDelay))
	}
	return err
}

// retryAt reports the time a wrapped error asks to be retried at, if it asks at all. It asks
// the library's own reader rather than matching a type here, so a test asserts what the
// runtime will actually do with the error.
func retryAt(err error) (time.Time, bool) {
	at := inngesterrors.GetRetryAtTime(err)
	if at == nil {
		return time.Time{}, false
	}
	return *at, true
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
					// async (not event) is the CEL variable Inngest's executor binds the candidate event to in
					// a WaitForEvent "if"; "event.data..." silently matches EVERY event of the right name
					// instead of failing to parse (orchestrate_integration_test.go covers it).
					If: inngestgo.StrPtr(fmt.Sprintf("async.data.queueId == %q", queueID)),
				},
			)
			if err != nil {
				if errors.Is(err, step.ErrEventNotReceived) {
					// The pause exceeded its bound: end the run without a review call or a tailor retry, as a
					// clean completion, since nobody responding in time is an expected outcome.
					return map[string]any{"status": "review_wait_timed_out", "queueId": queueID}, nil
				}
				return nil, fmt.Errorf("wait for review decision on queue entry %s: %w", queueID, err)
			}

			// No further HTTP call: PostAutoApplyReview publishes EventReviewDecided only after
			// recording the decision (see the package doc), so this run only notes the outcome it was
			// told.
			return map[string]any{"status": "reviewed", "queueId": queueID, "decision": decided.Data.Decision}, nil
		},
	)
}
