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

// autoApplyEventPublisher publishes the review-decision event that resumes a paused
// auto-apply orchestrator run (internal/application/autoapplyorchestrate). Nil (the
// assistantHandlers.events zero value) is the unconfigured deployment — see
// publishReviewDecided, the only caller.
type autoApplyEventPublisher interface {
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

func (p *inngestEventPublisher) PublishReviewDecided(ctx context.Context, queueID int64, decision string) error {
	body, err := json.Marshal(map[string]any{
		"name": autoapplyorchestrate.EventReviewDecided,
		"data": autoapplyorchestrate.ReviewDecidedEvent{
			QueueID:  strconv.FormatInt(queueID, 10),
			Decision: decision,
		},
	})
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

// publishReviewDecided best-effort publishes auto-apply/review.decided so a paused
// orchestrator run resumes — the same "log and continue" convention
// notifyTailoredCVReady already follows in this same file: recording the candidate's
// decision must never depend on, wait for, or fail because of this.
func (h *assistantHandlers) publishReviewDecided(ctx context.Context, queueID int64, decision string) {
	if h.events == nil {
		return
	}
	if err := h.events.PublishReviewDecided(ctx, queueID, decision); err != nil {
		log.Printf("auto-apply: publishing review-decided event for queue entry %d: %v", queueID, err)
	}
}
