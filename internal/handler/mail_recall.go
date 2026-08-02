package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/mailrecall"
)

// recalledEmail is one message the sweep proposes, in the same shape the application
// panel already draws its linked mail with. The rows come from the run itself rather than
// from a re-read: nothing in the schema fetches emails by an id list, and `GET
// /me/emails/:id` marks a message READ — a response assembled through it would zero its
// owner's unread count for mail no human has opened.
type recalledEmail struct {
	ID         int64     `json:"id"`
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
	if h.recall == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "mail recall is not configured")
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

	// The run goes out on the caller's own gateway credential. Attribution cannot fail: an
	// unresolvable one falls back to the service credential, still tagged.
	recall := h.recall.As(h.llm.bind(c.Context(), userID, tagMailRecall))
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
	case err != nil:
		return fiber.NewError(fiber.StatusBadGateway, "could not search your mail right now")
	}

	suggested := make([]recalledEmail, 0, len(result.Proposed))
	for _, p := range result.Proposed {
		suggested = append(suggested, recalledEmail{
			ID: p.Message.ID, FromAddr: p.Message.FromAddr, FromName: p.Message.FromName,
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
