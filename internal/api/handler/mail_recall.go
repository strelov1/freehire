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
// A model failure is a 502 rather than an empty success. Unlike the assistant's follow-up
// strip, which answers an empty list on every failure path because it is decoration, this
// is what somebody pressed — and "your mailbox holds nothing" is the wrong thing to tell
// them about a gateway being down.
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
		// A tracked job nobody applied to is not an application and has no mail to find.
		// The verdict is the service's, so the in-process caller meets it too; the handler
		// only chooses how to say it.
		return nil, fiber.NewError(fiber.StatusNotFound, "no application recorded for this job")
	case errors.Is(err, mailrecall.ErrModel), errors.Is(err, mailrecall.ErrSearch):
		// Somebody else's outage: the gateway let us down, or Google did. Logged rather
		// than reported, because classify() marks every *fiber.Error routine and the
		// exception log would never see it — and a 502 nobody records is how "the model had a bad day"
		// and "the model has been down since Tuesday" come to look identical.
		//
		// One branch for both because the sentence to the caller is the same one, and it
		// is already this branch's message. The two sentinels stay separate upstream, where
		// the distinction is between "we could not look" and "there was nothing to find".
		log.Printf("mail-recall: user %d: %v", userID, err)
		return nil, fiber.NewError(fiber.StatusBadGateway, "could not search your mail right now")
	case err != nil:
		// Anything else is ours: a dead pool, a lock timeout, a cancelled request. Returned
		// bare so classify() can do its job — 499 for a closed tab, 500 AND a report for a
		// fault. Wrapping these in a 502 would blame the model for a database error and
		// silence the only signal that there was one.
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
// It exists because a searched message is not ours yet: the sweep shows what the mailbox
// holds and keeps nothing, so this is the moment the message arrives. Import first, then
// link through internal/application/inbox so the ledger reconcile runs exactly as it does for a message
// the sync had fetched — the import is idempotent on (source, external_id), so a message
// already stored is linked rather than duplicated.
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

	// The import just stored it, so this resolves; a message the sync had already fetched
	// resolves to that same row rather than a second copy.
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
