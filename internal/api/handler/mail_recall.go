package handler

import (
	"context"
	"errors"
	"log"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/application/mailrecall"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// RecallApplicationMail sweeps the caller's mailbox for mail belonging to one of their
// applications and records the confident matches as suggestions.
//
// It PROPOSES and never links: the model reads attacker-controlled text, so its picks land
// in `suggested_job_id` and the caller resolves them through the confirm/reject endpoints
// that already exist. Nothing here writes the ledger, because an employer_reply is recorded
// on a link.
//
// A model failure is a 502, not an empty success: the user pressed this, and "your mailbox
// holds nothing" is wrong when the gateway is down.
func (h *inboxHandlers) RecallApplicationMail(ctx context.Context, params apigen.RecallApplicationMailParams) (*apigen.RecallApplicationMailOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	job, err := h.queries.GetJobBySlug(ctx, params.Slug)
	if err != nil {
		return nil, err // ErrNoRows → 404
	}
	app, err := h.queries.GetUserApplication(ctx,
		db.GetUserApplicationParams{UserID: userID, JobID: job.ID})
	if err != nil {
		return nil, err // ErrNoRows → 404 (caller does not track this job)
	}
	// After the ownership reads, not before: a deployment with no model should still answer
	// "no such application" for one that is not the caller's.
	if h.recall == nil {
		return nil, fiber.NewError(fiber.StatusServiceUnavailable, "mail recall is not configured")
	}

	// The run goes out on the caller's own gateway credential. Attribution cannot fail: an
	// unresolvable one falls back to the service credential, still tagged.
	//
	// The mailbox is resolved per caller and may be nil — a hosted address or a harness
	// tier — in which case the service reads stored mail instead.
	model, err := h.llm.bind(ctx, userID, llm.Feature(tagMailRecall))
	if err != nil {
		return nil, err
	}
	recall := h.recall.As(model)
	if h.mailboxes != nil {
		recall = recall.WithMailbox(h.mailboxes.For(ctx, userID))
	}
	result, err := recall.Recall(ctx, userID, mailrecall.Application{
		JobID:     job.ID,
		Company:   job.Company,
		Role:      job.Title,
		AppliedAt: app.AppliedAt.Time,
	})
	switch {
	case errors.Is(err, mailrecall.ErrNotAnApplication):
		// A tracked job nobody applied to is not an application. The verdict is the
		// service's so in-process callers meet it too; the handler only words it.
		return nil, fiber.NewError(fiber.StatusNotFound, "no application recorded for this job")
	case errors.Is(err, mailrecall.ErrModel), errors.Is(err, mailrecall.ErrSearch):
		// Somebody else's outage (gateway or Google). Logged, because classify() treats every
		// *fiber.Error as routine and the exception log would never see a 502.
		// One branch for both sentinels: the caller's message is the same.
		log.Printf("mail-recall: user %d: %v", userID, err)
		return nil, fiber.NewError(fiber.StatusBadGateway, "could not search your mail right now")
	case err != nil:
		// Anything else is ours (dead pool, lock timeout, cancelled request). Returned bare so
		// classify() yields 499 or a reported 500; a 502 would blame the model for a DB error.
		return nil, err
	}

	suggested := make([]apigen.MailRecalledEmail, 0, len(result.Proposed))
	for _, p := range result.Proposed {
		suggested = append(suggested, apigen.MailRecalledEmail{
			ID: p.Message.ID, ProviderID: optString(p.Message.ProviderID),
			FromAddr: p.Message.FromAddr, FromName: p.Message.FromName,
			Subject: p.Message.Subject, ReceivedAt: p.Message.ReceivedAt,
			Invitation: p.Message.ICalUID != "",
		})
	}
	return &apigen.RecallApplicationMailOK{Data: apigen.MailRecallResult{
		Scanned:     int64(result.Scanned),
		Suggested:   suggested,
		Invitations: int64(result.Invitations),
	}}, nil
}

// LinkRecalledMail imports a message the sweep proposed and links it to the application.
//
// A searched message is not ours yet: the sweep keeps nothing, so this is when it arrives.
// Import first, then link through internal/application/inbox so the ledger reconcile runs
// as for synced mail. The import is idempotent on (source, external_id), so an already
// stored message is linked, not duplicated.
func (h *inboxHandlers) LinkRecalledMail(ctx context.Context, req apigen.OptMailRecalledPick, params apigen.LinkRecalledMailParams) (*apigen.LinkRecalledMailOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	providerID, _ := req.Value.ProviderID.Get()
	if providerID == "" {
		return nil, fiber.NewError(fiber.StatusBadRequest, "provider_id is required")
	}
	job, err := h.queries.GetJobBySlug(ctx, params.Slug)
	if err != nil {
		return nil, err // ErrNoRows → 404
	}
	if _, err := h.queries.GetUserApplication(ctx,
		db.GetUserApplicationParams{UserID: userID, JobID: job.ID}); err != nil {
		return nil, err // ErrNoRows → 404 (caller does not track this job)
	}

	if h.mailboxes == nil {
		return nil, fiber.NewError(fiber.StatusServiceUnavailable, "no mailbox to import from")
	}
	box := h.mailboxes.For(ctx, userID)
	importer, ok := box.(interface {
		Import(ctx context.Context, userID int64, providerID string) error
	})
	if !ok {
		return nil, fiber.NewError(fiber.StatusServiceUnavailable, "no mailbox to import from")
	}
	if err := importer.Import(ctx, userID, providerID); err != nil {
		log.Printf("mail-recall link: user %d: %v", userID, err)
		return nil, fiber.NewError(fiber.StatusBadGateway, "could not read that message right now")
	}

	// Resolves to the just-imported row, or to the existing one for already-synced mail.
	id, err := h.queries.GetEmailIDByExternalID(ctx,
		db.GetEmailIDByExternalIDParams{UserID: userID, ExternalID: providerID})
	if err != nil {
		return nil, err
	}
	msg, err := h.inbox.Link(ctx, userID, id, params.Slug)
	if err != nil {
		return nil, err
	}
	return &apigen.LinkRecalledMailOK{Data: bodyOf(msg)}, nil
}
