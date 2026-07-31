package handler

import (
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
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
	// FollowedUpAt is when the caller last recorded chasing this application, or null
	// for never. It says nothing about whether anybody replied.
	FollowedUpAt *time.Time         `json:"followed_up_at"`
	Emails       []applicationEmail `json:"emails"`
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
		Job:          jv,
		ViewedAt:     tsPtr(app.ViewedAt),
		SavedAt:      tsPtr(app.SavedAt),
		AppliedAt:    tsPtr(app.AppliedAt),
		Stage:        pgStr(app.Stage),
		Notes:        pgStr(app.Notes),
		FollowedUpAt: tsPtr(app.FollowedUpAt),
		Emails:       emails,
	}})
}

// ConfirmEmailLink promotes an email's pending suggestion to a confirmed manual
// link. A 404 when the email is not the caller's or carries no suggestion.
func (h *inboxHandlers) ConfirmEmailLink(c *fiber.Ctx) error {
	return h.mutateEmailLink(c, func(userID, id int64) (int64, error) {
		return h.queries.ConfirmEmailLink(c.Context(), db.ConfirmEmailLinkParams{ID: id, UserID: userID})
	})
}

// RejectEmailLink dismisses an email's pending suggestion without linking.
func (h *inboxHandlers) RejectEmailLink(c *fiber.Ctx) error {
	return h.mutateEmailLink(c, func(userID, id int64) (int64, error) {
		return h.queries.RejectEmailLink(c.Context(), db.RejectEmailLinkParams{ID: id, UserID: userID})
	})
}

// UnlinkEmail clears an email's application link.
func (h *inboxHandlers) UnlinkEmail(c *fiber.Ctx) error {
	return h.mutateEmailLink(c, func(userID, id int64) (int64, error) {
		return h.queries.UnlinkEmail(c.Context(), db.UnlinkEmailParams{ID: id, UserID: userID})
	})
}

// LinkEmail manually links an email to the application named by {"slug": …}.
func (h *inboxHandlers) LinkEmail(c *fiber.Ctx) error {
	var body struct {
		Slug string `json:"slug"`
	}
	if err := c.BodyParser(&body); err != nil || body.Slug == "" {
		return fiber.NewError(fiber.StatusBadRequest, "slug required")
	}
	job, err := h.queries.GetJobBySlug(c.Context(), body.Slug)
	if err != nil {
		return err // ErrNoRows → 404
	}
	return h.mutateEmailLink(c, func(userID, id int64) (int64, error) {
		return h.queries.LinkEmailToJob(c.Context(), db.LinkEmailToJobParams{
			ID: id, UserID: userID, JobID: pgtype.Int8{Int64: job.ID, Valid: true},
		})
	})
}

// CreateApplicationFromEmail records a tracked application from one of the
// caller's emails and links the email to it in a single call, given {"slug": …}
// naming a catalog job. It is the path for mail about an application the caller
// never recorded: the employer is known and the job is in the catalog, but there
// is no application to attach the mail to, so plain linking has nothing to point
// at.
//
// The application is dated by the email, not by now() — see
// jobtracking.MarkAppliedAt. Mail still carrying a pending suggestion is refused
// (409): the matcher has already proposed an answer, and letting this path
// overwrite it silently would leave the link's provenance ambiguous. Confirming
// or rejecting the suggestion first resolves that.
func (h *inboxHandlers) CreateApplicationFromEmail(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "not found")
	}
	var body struct {
		Slug string `json:"slug"`
	}
	if err := c.BodyParser(&body); err != nil || body.Slug == "" {
		return fiber.NewError(fiber.StatusBadRequest, "slug required")
	}
	email, err := h.queries.GetEmail(c.Context(), db.GetEmailParams{ID: int64(id), UserID: userID})
	if err != nil {
		return err // ErrNoRows → 404 (not the caller's message)
	}
	if email.SuggestedJobID.Valid && !email.JobID.Valid {
		return fiber.NewError(fiber.StatusConflict,
			"email has a pending suggestion ("+pgStr(email.SuggestedSlug)+"); confirm or reject it first")
	}
	job, err := h.queries.GetJobBySlug(c.Context(), body.Slug)
	if err != nil {
		return err // ErrNoRows → 404
	}
	if _, err := h.tracking.MarkAppliedAt(c.Context(), userID, body.Slug, email.ReceivedAt.Time); err != nil {
		return err
	}
	rows, err := h.queries.LinkEmailToJob(c.Context(), db.LinkEmailToJobParams{
		ID: int64(id), UserID: userID, JobID: pgtype.Int8{Int64: job.ID, Valid: true},
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		return fiber.NewError(fiber.StatusNotFound, "not found")
	}
	return h.renderEmail(c, userID, int64(id))
}

// mutateEmailLink runs one email-link mutation (scoped to the caller by id) and
// returns the refreshed email, or 404 when the mutation matched no row.
func (h *inboxHandlers) mutateEmailLink(c *fiber.Ctx, do func(userID, id int64) (int64, error)) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "not found")
	}
	rows, err := do(userID, int64(id))
	if err != nil {
		return err
	}
	if rows == 0 {
		return fiber.NewError(fiber.StatusNotFound, "not found")
	}
	return h.renderEmail(c, userID, int64(id))
}

// renderEmail responds with one message in the single-message wire shape, freshly
// read. It is the shared tail of every mutation that returns the message it just
// changed (link, unlink, confirm, reject, triage), so those cannot drift from one
// another or from GetEmail. Unlike GetEmail it does not mark the message read: the
// caller is acting on the message, not opening it.
func (h *inboxHandlers) renderEmail(c *fiber.Ctx, userID, id int64) error {
	row, err := h.queries.GetEmail(c.Context(), db.GetEmailParams{ID: id, UserID: userID})
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": emailBody{
		ID: row.ID, Source: row.Source, ExternalID: row.ExternalID,
		FromAddr: row.FromAddr, FromName: row.FromName, Subject: row.Subject,
		BodyText: row.BodyText, BodyHTML: row.BodyHtml,
		ReceivedAt: row.ReceivedAt.Time, Read: row.Read,
		emailLinking: emailLinking{
			StatusSignal:     pgStr(row.StatusSignal),
			LinkSource:       pgStr(row.LinkSource),
			LinkedSlug:       pgStr(row.LinkedSlug),
			LinkedCompany:    pgStr(row.LinkedCompany),
			SuggestedSlug:    pgStr(row.SuggestedSlug),
			SuggestedCompany: pgStr(row.SuggestedCompany),
		},
	}})
}

// tsPtr unwraps a nullable timestamp to *time.Time (nil when NULL).
func tsPtr(t pgtype.Timestamptz) *time.Time {
	if t.Valid {
		return &t.Time
	}
	return nil
}
