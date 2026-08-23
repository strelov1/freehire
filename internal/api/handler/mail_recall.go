package handler

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/application/mailrecall"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/llm"
)

// recalledEmail is one message the sweep proposes, in the same shape the application
// panel already draws its linked mail with. The rows come from the run itself rather than
// from a re-read: nothing in the schema fetches emails by an id list, and `GET
// /me/emails/:id` marks a message READ — a response assembled through it would zero its
// owner's unread count for mail no human has opened.
type recalledEmail struct {
	// ID is our stored row's id, and is 0 for a message the search found. A proposal from
	// the search path is addressed by ProviderID until the caller links it.
	ID int64 `json:"id"`
	// ProviderID names the message at the mail provider. Empty on the stored-mail path.
	ProviderID string    `json:"provider_id,omitempty"`
	FromAddr   string    `json:"from_addr"`
	FromName   string    `json:"from_name"`
	Subject    string    `json:"subject"`
	ReceivedAt time.Time `json:"received_at"`
	// Invitation says this message carries a calendar invitation's identifier, so
	// confirming it is what brings the meeting in.
	Invitation bool `json:"invitation"`
}

// mailRecallResponse is the wire shape for POST /me/tracking/:slug/mail-recall.
type mailRecallResponse struct {
	// Scanned is how many messages the net examined, which is what makes an empty
	// Suggested legible: nothing found in forty is a different answer from nothing to look
	// at.
	Scanned   int             `json:"scanned"`
	Suggested []recalledEmail `json:"suggested"`
	// Invitations counts the proposed messages carrying an invitation identifier. The
	// meetings themselves arrive on the next calendar sync, which re-reads its whole
	// window and re-matches it against the caller's applications as they then stand.
	Invitations int `json:"invitations"`
}

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
func (h *inboxHandlers) RecallApplicationMail(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	job, err := h.queries.GetJobBySlug(c.Context(), c.Params("slug"))
	if err != nil {
		return err // ErrNoRows → 404
	}
	app, err := h.queries.GetUserApplication(c.Context(),
		db.GetUserApplicationParams{UserID: userID, JobID: job.ID})
	if err != nil {
		return err // ErrNoRows → 404 (caller does not track this job)
	}
	// After the ownership reads, not before: a deployment with no model should still answer
	// "no such application" for one that is not the caller's.
	if h.recall == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "mail recall is not configured")
	}

	// The run goes out on the caller's own gateway credential. Attribution cannot fail: an
	// unresolvable one falls back to the service credential, still tagged.
	//
	// The mailbox is resolved per caller and may be nil — a hosted address or a harness
	// tier — in which case the service reads stored mail instead.
	recall := h.recall.As(h.llm.bind(c.Context(), userID, llm.Feature(tagMailRecall)))
	if h.mailboxes != nil {
		recall = recall.WithMailbox(h.mailboxes.For(c.Context(), userID))
	}
	result, err := recall.Recall(c.Context(), userID, mailrecall.Application{
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
		return fiber.NewError(fiber.StatusNotFound, "no application recorded for this job")
	case errors.Is(err, mailrecall.ErrModel):
		// The gateway let us down. Logged rather than reported, because classify() marks
		// every *fiber.Error routine and Sentry would never see it — and a 502 nobody
		// records is how "the model had a bad day" and "the model has been down since
		// Tuesday" come to look identical.
		log.Printf("mail-recall: user %d: %v", userID, err)
		return fiber.NewError(fiber.StatusBadGateway, "could not search your mail right now")
	case err != nil:
		// Anything else is ours: a dead pool, a lock timeout, a cancelled request. Returned
		// bare so classify() can do its job — 499 for a closed tab, 500 AND Sentry for a
		// fault. Wrapping these in a 502 would blame the model for a database error and
		// silence the only signal that there was one.
		return err
	}

	suggested := make([]recalledEmail, 0, len(result.Proposed))
	for _, p := range result.Proposed {
		suggested = append(suggested, recalledEmail{
			ID: p.Message.ID, ProviderID: p.Message.ProviderID,
			FromAddr: p.Message.FromAddr, FromName: p.Message.FromName,
			Subject: p.Message.Subject, ReceivedAt: p.Message.ReceivedAt,
			Invitation: p.Message.ICalUID != "",
		})
	}
	return c.JSON(fiber.Map{"data": mailRecallResponse{
		Scanned:     result.Scanned,
		Suggested:   suggested,
		Invitations: result.Invitations,
	}})
}

// linkRecalledRequest names one message the caller picked out of a sweep.
type linkRecalledRequest struct {
	ProviderID string `json:"provider_id"`
}

// LinkRecalledMail imports a message the sweep proposed and links it to the application.
//
// It exists because a searched message is not ours yet: the sweep shows what the mailbox
// holds and keeps nothing, so this is the moment the message arrives. Import first, then
// link through internal/application/inbox so the ledger reconcile runs exactly as it does for a message
// the sync had fetched — the import is idempotent on (source, external_id), so a message
// already stored is linked rather than duplicated.
func (h *inboxHandlers) LinkRecalledMail(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var req linkRecalledRequest
	if err := c.BodyParser(&req); err != nil || req.ProviderID == "" {
		return fiber.NewError(fiber.StatusBadRequest, "provider_id is required")
	}
	job, err := h.queries.GetJobBySlug(c.Context(), c.Params("slug"))
	if err != nil {
		return err // ErrNoRows → 404
	}
	if _, err := h.queries.GetUserApplication(c.Context(),
		db.GetUserApplicationParams{UserID: userID, JobID: job.ID}); err != nil {
		return err // ErrNoRows → 404 (caller does not track this job)
	}

	if h.mailboxes == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "no mailbox to import from")
	}
	box := h.mailboxes.For(c.Context(), userID)
	importer, ok := box.(interface {
		Import(ctx context.Context, userID int64, providerID string) error
	})
	if !ok {
		return fiber.NewError(fiber.StatusServiceUnavailable, "no mailbox to import from")
	}
	if err := importer.Import(c.Context(), userID, req.ProviderID); err != nil {
		log.Printf("mail-recall link: user %d: %v", userID, err)
		return fiber.NewError(fiber.StatusBadGateway, "could not read that message right now")
	}

	// The import just stored it, so this resolves; a message the sync had already fetched
	// resolves to that same row rather than a second copy.
	id, err := h.queries.GetEmailIDByExternalID(c.Context(),
		db.GetEmailIDByExternalIDParams{UserID: userID, ExternalID: req.ProviderID})
	if err != nil {
		return err
	}
	msg, err := h.inbox.Link(c.Context(), userID, id, c.Params("slug"))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": bodyOf(msg)})
}
