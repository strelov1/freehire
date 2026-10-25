package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/platform/db"
)

// PostAssistantSync catches an apply conversation up on its application (openspec/changes/
// auto-apply-chat). The client calls it when the chat opens and periodically while it stays
// open. When the attempt has events the conversation has not narrated, it streams one turn
// over them; when it has none it answers 204 and the model is never asked.
//
// The turn's input is a status note, not a message from the candidate, so it is not charged
// to their assistant allowance — they did not ask for it, and it is bounded by the attempt's
// own handful of events. The cursor moves only after the turn finished, so a narration that
// failed is narrated again by the next sync instead of being skipped. A backlog longer than a
// page is narrated a page per sync.
func (h *assistantHandlers) PostAssistantSync(c *fiber.Ctx) error {
	sess, err := h.ownedSession(c)
	if err != nil {
		return err
	}
	if sess.Preset != assistant.PresetApply || sess.JobID == nil {
		return fiber.NewError(fiber.StatusConflict, "this conversation is not about an application")
	}
	if h.runner == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "the assistant is not available")
	}
	// A cheap look first, so a sync with nothing new answers 204 without taking the
	// session's turn slot — which is what every fifteen-second check of a quiet application is.
	ctx := c.Context()
	if _, unseen, err := h.unnarrated(ctx, sess); err != nil {
		return err
	} else if len(unseen) == 0 {
		return c.SendStatus(fiber.StatusNoContent)
	}
	return h.streamSSE(c, sess, unmetered, func(ctx context.Context, runner *assistant.Runner, reg *assistant.Registry, system string, emit func(assistant.Event)) error {
		// Read again now that the slot is ours: a sync that queued behind another turn — a
		// second tab's sync, the candidate's own reply — must narrate what is unseen NOW, not
		// what was unseen when it arrived, or it narrates the same events twice.
		cursor, unseen, err := h.unnarrated(ctx, sess)
		if err != nil {
			return err
		}
		if len(unseen) == 0 {
			emit(assistant.Event{Kind: assistant.EventResult, StopReason: assistant.StopEndTurn})
			return nil
		}
		note, err := h.applyStatusNote(ctx, sess.UserID, *sess.JobID, unseen)
		if err != nil {
			return err
		}
		if err := runner.RunNote(ctx, sess, reg, system, note, assistant.TurnConfig{}, emit); err != nil {
			return err
		}
		moved, err := h.queries.AdvanceAssistantNarratedEvent(context.WithoutCancel(ctx), db.AdvanceAssistantNarratedEventParams{
			NarratedEventID: pgtype.Int8{Int64: unseen[len(unseen)-1].ID, Valid: true},
			ID:              sess.ID,
			UserID:          sess.UserID,
			Previous:        cursor,
		})
		if err == nil && moved == 0 {
			log.Printf("assistant: apply session %s cursor moved under a running narration", sess.ID)
		}
		return err
	})
}

// unnarrated reads the session's cursor and the application's events after it, oldest
// first, one page at a time.
func (h *assistantHandlers) unnarrated(ctx context.Context, sess assistant.Session) (pgtype.Int8, []db.ListAutoApplyEventsAfterRow, error) {
	cursor, err := h.queries.GetAssistantNarratedEvent(ctx, db.GetAssistantNarratedEventParams{ID: sess.ID, UserID: sess.UserID})
	if err != nil {
		return pgtype.Int8{}, nil, err
	}
	events, err := h.queries.ListAutoApplyEventsAfter(ctx, db.ListAutoApplyEventsAfterParams{
		UserID: sess.UserID, JobID: *sess.JobID, AfterID: cursor.Int64,
	})
	return cursor, events, err
}

// unmetered is the meter for a turn the candidate did not ask for.
func unmetered(*fiber.Ctx, assistant.Session) (turnCharge, bool, error) {
	return turnCharge{}, false, nil
}

// applyStatusNote is what the model is told: the events since it last spoke, and where the
// attempt stands now — the same view auto_apply_get returns, so the open questions it is to
// list arrive with the note rather than after a tool call.
func (h *assistantHandlers) applyStatusNote(ctx context.Context, userID, jobID int64, events []db.ListAutoApplyEventsAfterRow) (string, error) {
	var b strings.Builder
	b.WriteString("Since you last spoke to the candidate, the application's history recorded:\n")
	for _, e := range events {
		fmt.Fprintf(&b, "- %s %s", e.CreatedAt.Time.UTC().Format("2006-01-02 15:04 UTC"), e.Kind)
		if string(e.Detail) != "{}" {
			fmt.Fprintf(&b, " %s", e.Detail)
		}
		b.WriteString("\n")
	}
	job, err := h.queries.GetJob(ctx, jobID)
	if err != nil {
		return "", err
	}
	view, err := h.autoApplyView(ctx, userID, job.PublicSlug)
	if err != nil {
		// A cancelled or never-started attempt has no live view; the events say enough.
		return b.String(), nil
	}
	raw, err := json.Marshal(view)
	if err != nil {
		return "", err
	}
	b.WriteString("\nWhere it stands now (as auto_apply_get returns it):\n")
	b.Write(raw)
	return b.String(), nil
}
