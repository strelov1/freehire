package handler

import (
	"encoding/json"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/appevent"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/followup"
	"github.com/strelov1/freehire/internal/matchanalysis"
	"github.com/strelov1/freehire/internal/userjob"
)

// followUpDraftResponse is the wire shape for a chase the candidate sends themselves. Recipient is
// omitted when nobody ever replied — the commonest silent application — rather than guessed.
type followUpDraftResponse struct {
	Subject       string `json:"subject"`
	Body          string `json:"body"`
	Recipient     string `json:"recipient,omitempty"`
	RecipientName string `json:"recipient_name,omitempty"`
	DaysSilent    int    `json:"days_silent"`
}

// GetApplicationFollowUp assembles a follow-up draft for a silent application.
//
// The gate is the same verdict the board's badge renders: a draft is offered only where
// userjob.SilenceStateFor says `silent`, so the offer and the badge cannot disagree about which
// applications qualify. Nothing here sends mail — the draft is handed to the candidate.
func (h *inboxHandlers) GetApplicationFollowUp(c *fiber.Ctx) error {
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

	days := 0
	if app.LastActivityAt.Valid {
		days = daysSilent(time.Now(), app.LastActivityAt.Time)
	}
	if userjob.SilenceStateFor(pgStr(app.Stage), days, app.HasPendingSuggestion) != userjob.SilenceSilent {
		return fiber.NewError(fiber.StatusConflict, "this application is not waiting on a reply")
	}

	addr, name := h.newestSender(c, userID, job.ID)
	msg := followup.Draft(followup.Input{
		Role:          job.Title,
		Company:       job.Company,
		DaysSilent:    days,
		Stage:         pgStr(app.Stage),
		Strength:      h.cachedStrength(c, userID, job.ID),
		RecipientName: name,
	})
	return c.JSON(fiber.Map{"data": followUpDraftResponse{
		Subject: msg.Subject, Body: msg.Body,
		Recipient: addr, RecipientName: name, DaysSilent: days,
	}})
}

// RecordApplicationFollowUp records that the candidate chased. It does NOT touch the silence
// derivation: last activity is when the OTHER side moved, and a chase is not a reply (see the
// column comment in 0059). Idempotent — a double click overwrites the timestamp, and the
// ledger event it writes is suppressed within the hour for the same reason: a resubmit is
// seconds apart, a genuine second chase days apart.
func (h *inboxHandlers) RecordApplicationFollowUp(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	job, err := h.queries.GetJobBySlug(c.Context(), c.Params("slug"))
	if err != nil {
		return err
	}
	if _, err := h.queries.RecordApplicationFollowUp(c.Context(),
		db.RecordApplicationFollowUpParams{
			UserID: userID, JobID: job.ID, EventSource: appevent.SourceUser,
		}); err != nil {
		return err // ErrNoRows → 404: not an application of this caller's
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// daysSilent is whole days since the application last moved, floored at zero so a clock skew
// cannot report a negative silence.
func daysSilent(now, last time.Time) int {
	if d := int(now.Sub(last).Hours() / 24); d > 0 {
		return d
	}
	return 0
}

// newestSender returns the address and display name of the most recent mail linked to the
// application, or empty strings when nobody ever wrote — which is the normal case for an
// application that went unanswered, and why the draft is issued without a recipient.
func (h *inboxHandlers) newestSender(c *fiber.Ctx, userID, jobID int64) (addr, name string) {
	rows, err := h.queries.ListJobEmails(c.Context(), db.ListJobEmailsParams{
		UserID: userID, JobID: pgtype.Int8{Int64: jobID, Valid: true},
	})
	if err != nil || len(rows) == 0 {
		return "", ""
	}
	return rows[0].FromAddr, rows[0].FromName
}

// cachedStrength reads one concrete strength from the cached fit analysis — text the analysis
// derived from the candidate's own CV. No analysis, or one naming none, yields "" and the draft
// simply omits the line: this is the only source, so the draft cannot invent a claim.
func (h *inboxHandlers) cachedStrength(c *fiber.Ctx, userID, jobID int64) string {
	row, err := h.queries.GetUserJobAnalysis(c.Context(),
		db.GetUserJobAnalysisParams{UserID: userID, JobID: jobID})
	if err != nil || len(row.Analysis) == 0 {
		return ""
	}
	var a matchanalysis.Analysis
	if err := json.Unmarshal(row.Analysis, &a); err != nil || len(a.Strengths) == 0 {
		return ""
	}
	return a.Strengths[0]
}
