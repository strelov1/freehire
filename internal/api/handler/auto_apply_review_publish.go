package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/application/autoapplyorchestrate"
)

// autoApplyEventPublisher publishes the two events that drive the durable orchestrator
// run (internal/application/autoapplyorchestrate): PublishSubmit starts one
// (openspec/changes/auto-apply-submit-trigger), PublishReviewDecided resumes one paused
// awaiting the candidate's decision. Nil (the assistantHandlers.events zero value) is the
// unconfigured deployment — see publishSubmit/publishReviewDecided, the only callers.
type autoApplyEventPublisher interface {
	PublishSubmit(ctx context.Context, queueID int64) error
	PublishReviewDecided(ctx context.Context, queueID int64, decision string) error
}

// inngestEventPublisher is the real autoApplyEventPublisher: a plain http.Client POST to
// a self-hosted Inngest server's own event API — NOT internal/platform/safehttp, whose
// guard specifically blocks the internal address this call must reach (see
// openspec/changes/auto-apply-inngest-orchestration/design.md's own Decisions).
type inngestEventPublisher struct {
	baseURL    string
	eventKey   string
	httpClient *http.Client
}

// inngestPublishTimeout bounds one event-publish call. Short: this is a best-effort,
// fire-and-log side effect of a request that has already done its real work (recording
// the candidate's decision), so it must never hold that response open for long.
const inngestPublishTimeout = 5 * time.Second

// newInngestEventPublisher builds the real publisher, or returns nil when either setting
// is empty — the same "absence is the off switch" convention every other optional
// dependency in this handler package follows.
func newInngestEventPublisher(baseURL, eventKey string) *inngestEventPublisher {
	if baseURL == "" || eventKey == "" {
		return nil
	}
	return &inngestEventPublisher{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		eventKey:   eventKey,
		httpClient: &http.Client{Timeout: inngestPublishTimeout},
	}
}

func (p *inngestEventPublisher) PublishSubmit(ctx context.Context, queueID int64) error {
	return p.post(ctx, autoapplyorchestrate.EventSubmit, autoapplyorchestrate.SubmitEvent{
		QueueID: strconv.FormatInt(queueID, 10),
	})
}

func (p *inngestEventPublisher) PublishReviewDecided(ctx context.Context, queueID int64, decision string) error {
	return p.post(ctx, autoapplyorchestrate.EventReviewDecided, autoapplyorchestrate.ReviewDecidedEvent{
		QueueID:  strconv.FormatInt(queueID, 10),
		Decision: decision,
	})
}

// post sends one event to the self-hosted Inngest event API. Shared by both publish
// methods — they differ only in event name and data shape.
func (p *inngestEventPublisher) post(ctx context.Context, name string, data any) error {
	body, err := json.Marshal(map[string]any{"name": name, "data": data})
	if err != nil {
		return fmt.Errorf("encode event: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/e/"+p.eventKey, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("publish event: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("publish event: status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

// publishSubmit best-effort publishes auto-apply/submit so the orchestrator starts a
// fresh run — same "log and continue" convention as publishReviewDecided below: the
// enqueue itself already succeeded and committed, so this must never fail, or hold open,
// the response. Detached from the request's own context (which is cancelled the moment
// the handler returns, before a goroutine started from it would get to run) onto a fresh
// one bounded by inngestPublishTimeout instead.
func (h *assistantHandlers) publishSubmit(ctx context.Context, queueID int64) {
	if h.events == nil {
		return
	}
	go func() {
		ctx, cancel := detachedPublishContext(ctx)
		defer cancel()
		if err := h.events.PublishSubmit(ctx, queueID); err != nil {
			log.Printf("auto-apply: publishing submit event for queue entry %d: %v", queueID, err)
		}
	}()
}

// publishReviewDecided best-effort publishes auto-apply/review.decided so a paused
// orchestrator run resumes — the same "log and continue" convention
// notifyTailoredCVReady (auto_apply_tailor.go) already follows: recording the candidate's
// decision must never depend on, wait for, or fail because of this. Detached the same way
// publishSubmit is, for the same reason.
func (h *assistantHandlers) publishReviewDecided(ctx context.Context, queueID int64, decision string) {
	if h.events == nil {
		return
	}
	go func() {
		ctx, cancel := detachedPublishContext(ctx)
		defer cancel()
		if err := h.events.PublishReviewDecided(ctx, queueID, decision); err != nil {
			log.Printf("auto-apply: publishing review-decided event for queue entry %d: %v", queueID, err)
		}
	}()
}

// detachedPublishContext carries the request context's values (for tracing/logging) but
// none of its cancellation, since the request itself will have finished responding before
// the publish goroutine runs — only inngestPublishTimeout bounds it.
func detachedPublishContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), inngestPublishTimeout)
}
