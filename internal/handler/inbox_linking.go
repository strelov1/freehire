package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/inbox"
	"github.com/strelov1/freehire/internal/jobview"
)

// applicationEmail is one linked email on the application detail page.
type applicationEmail struct {
	ID           int64     `json:"id"`
	Source       string    `json:"source"`
	FromAddr     string    `json:"from_addr"`
	FromName     string    `json:"from_name"`
	Subject      string    `json:"subject"`
	StatusSignal string    `json:"status_signal,omitempty"`
	LinkSource   string    `json:"link_source,omitempty"`
	ReceivedAt   time.Time `json:"received_at"`
	Read         bool      `json:"read"`
}

// applicationDetail is the wire shape for GET /me/tracking/:slug — the job in the
// shared jobview shape, the caller's interaction, and the emails linked to it.
type applicationDetail struct {
	Job       jobview.Job        `json:"job"`
	ViewedAt  *time.Time         `json:"viewed_at"`
	SavedAt   *time.Time         `json:"saved_at"`
	AppliedAt *time.Time         `json:"applied_at"`
	Stage     string             `json:"stage,omitempty"`
	Notes     string             `json:"notes,omitempty"`
	Emails    []applicationEmail `json:"emails"`
}

// GetTrackedApplication returns the caller's application for a job slug together
// with the emails linked to it. A slug the caller does not track is a 404.
func (h *inboxHandlers) GetTrackedApplication(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	job, err := h.queries.GetJobBySlug(c.Context(), c.Params("slug"))
	if err != nil {
		return err // ErrNoRows → 404
	}
	app, err := h.queries.GetUserApplication(c.Context(), db.GetUserApplicationParams{UserID: userID, JobID: job.ID})
	if err != nil {
		return err // ErrNoRows → 404 (caller does not track this job)
	}
	rows, err := h.queries.ListJobEmails(c.Context(), db.ListJobEmailsParams{
		UserID: userID, JobID: pgtype.Int8{Int64: job.ID, Valid: true},
	})
	if err != nil {
		return err
	}
	jv, err := jobview.FromRow(job)
	if err != nil {
		return err
	}

	emails := make([]applicationEmail, 0, len(rows))
	for _, r := range rows {
		emails = append(emails, applicationEmail{
			ID: r.ID, Source: r.Source, FromAddr: r.FromAddr, FromName: r.FromName,
			Subject: r.Subject, StatusSignal: pgStr(r.StatusSignal), LinkSource: pgStr(r.LinkSource),
			ReceivedAt: r.ReceivedAt.Time, Read: r.Read,
		})
	}
	return c.JSON(fiber.Map{"data": applicationDetail{
		Job:       jv,
		ViewedAt:  tsPtr(app.ViewedAt),
		SavedAt:   tsPtr(app.SavedAt),
		AppliedAt: tsPtr(app.AppliedAt),
		Stage:     pgStr(app.Stage),
		Notes:     pgStr(app.Notes),
		Emails:    emails,
	}})
}

// ConfirmEmailLink promotes an email's pending suggestion to a confirmed manual
// link. A 404 when the email is not the caller's or carries no suggestion.
func (h *inboxHandlers) ConfirmEmailLink(c *fiber.Ctx) error {
	return h.renderMutation(c, func(userID, id int64) (inbox.Message, error) {
		return h.inbox.ResolveSuggestion(c.Context(), userID, id, true)
	})
}

// RejectEmailLink dismisses an email's pending suggestion without linking.
func (h *inboxHandlers) RejectEmailLink(c *fiber.Ctx) error {
	return h.renderMutation(c, func(userID, id int64) (inbox.Message, error) {
		return h.inbox.ResolveSuggestion(c.Context(), userID, id, false)
	})
}

// UnlinkEmail clears an email's application link.
func (h *inboxHandlers) UnlinkEmail(c *fiber.Ctx) error {
	return h.renderMutation(c, func(userID, id int64) (inbox.Message, error) {
		return h.inbox.Unlink(c.Context(), userID, id)
	})
}

// LinkEmail manually links an email to the application named by {"slug": …}.
func (h *inboxHandlers) LinkEmail(c *fiber.Ctx) error {
	slug, err := slugFromBody(c)
	if err != nil {
		return err
	}
	return h.renderMutation(c, func(userID, id int64) (inbox.Message, error) {
		return h.inbox.Link(c.Context(), userID, id, slug)
	})
}

// CreateApplicationFromEmail records a tracked application from one of the
// caller's emails and links the email to it in a single call, given {"slug": …}
// naming a catalog job. It is the path for mail about an application the caller
// never recorded: the employer is known and the job is in the catalog, but there
// is no application to attach the mail to, so plain linking has nothing to point
// at.
//
// The application is dated by the email, not by now(). Mail still carrying a
// pending suggestion is refused (409) — see inbox.Service.RecordApplication.
func (h *inboxHandlers) CreateApplicationFromEmail(c *fiber.Ctx) error {
	slug, err := slugFromBody(c)
	if err != nil {
		return err
	}
	return h.renderMutation(c, func(userID, id int64) (inbox.Message, error) {
		return h.inbox.RecordApplication(c.Context(), userID, id, slug)
	})
}

// slugFromBody reads the {"slug": …} body shared by the linking endpoints.
func slugFromBody(c *fiber.Ctx) (string, error) {
	var body struct {
		Slug string `json:"slug"`
	}
	if err := c.BodyParser(&body); err != nil || body.Slug == "" {
		return "", fiber.NewError(fiber.StatusBadRequest, "slug required")
	}
	return body.Slug, nil
}

// renderMutation runs one email mutation for the caller and renders the message it
// changed. It is the shared tail of link, unlink, confirm, reject, triage and
// application-from-mail, so none of them can drift from another or from GetEmail —
// and unlike GetEmail it does not mark the message read: the caller is acting on
// the message, not opening it.
func (h *inboxHandlers) renderMutation(c *fiber.Ctx, do func(userID, id int64) (inbox.Message, error)) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "not found")
	}
	msg, err := do(userID, int64(id))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": bodyOf(msg)})
}

// tsPtr unwraps a nullable timestamp to *time.Time (nil when NULL).
func tsPtr(t pgtype.Timestamptz) *time.Time {
	if t.Valid {
		return &t.Time
	}
	return nil
}
