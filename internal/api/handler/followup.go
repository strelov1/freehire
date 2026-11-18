package handler

import (
	"context"
	"encoding/json"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/application/appevent"
	"github.com/strelov1/freehire/internal/application/followup"
	"github.com/strelov1/freehire/internal/candidate/matchanalysis"
	"github.com/strelov1/freehire/internal/job/silence"
	"github.com/strelov1/freehire/internal/platform/db"
)

// GetApplicationFollowUp assembles a follow-up draft for a silent application.
//
// The gate is the verdict the board's badge renders (silence.StateFor says `silent`),
// so the offer and the badge cannot disagree. Nothing here sends mail.
func (h *inboxHandlers) GetApplicationFollowUp(ctx context.Context, params apigen.GetApplicationFollowUpParams) (*apigen.GetApplicationFollowUpOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	job, err := h.queries.GetJobBySlug(ctx, params.Slug)
	if err != nil {
		return nil, err // ErrNoRows → 404
	}
	app, err := h.queries.GetUserApplication(ctx, db.GetUserApplicationParams{UserID: userID, JobID: job.ID})
	if err != nil {
		return nil, err // ErrNoRows → 404 (caller does not track this job)
	}

	days := 0
	if app.LastActivityAt.Valid {
		days = silence.Days(time.Now(), app.LastActivityAt.Time)
	}
	if silence.StateFor(pgStr(app.Stage), days, app.HasPendingSuggestion) != silence.Silent {
		return nil, fiber.NewError(fiber.StatusConflict, "this application is not waiting on a reply")
	}

	addr, name := h.newestSender(ctx, userID, job.ID)
	msg := followup.Draft(followup.Input{
		Role:          job.Title,
		Company:       job.Company,
		DaysSilent:    days,
		Stage:         pgStr(app.Stage),
		Strength:      h.cachedStrength(ctx, userID, job.ID),
		RecipientName: name,
	})
	return &apigen.GetApplicationFollowUpOK{Data: apigen.MailFollowUpDraft{
		Subject: msg.Subject, Body: msg.Body,
		Recipient: optString(addr), RecipientName: optString(name), DaysSilent: int64(days),
	}}, nil
}

// RecordApplicationFollowUp records that the candidate chased. It does NOT touch the silence
// derivation: last activity is when the OTHER side moved, and a chase is not a reply.
// Idempotent: a double click overwrites the timestamp, and the ledger event is suppressed
// within the hour.
func (h *inboxHandlers) RecordApplicationFollowUp(ctx context.Context, params apigen.RecordApplicationFollowUpParams) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	job, err := h.queries.GetJobBySlug(ctx, params.Slug)
	if err != nil {
		return err
	}
	if _, err := h.queries.RecordApplicationFollowUp(ctx,
		db.RecordApplicationFollowUpParams{
			UserID: userID, JobID: job.ID, EventSource: appevent.SourceUser,
		}); err != nil {
		return err // ErrNoRows → 404: not an application of this caller's
	}
	return nil
}

// newestSender returns the address and name of the most recent incoming mail linked to the
// application, or empty strings when nobody wrote (the draft then has no recipient).
func (h *inboxHandlers) newestSender(ctx context.Context, userID, jobID int64) (addr, name string) {
	rows, err := h.queries.ListJobEmails(ctx, db.ListJobEmailsParams{
		UserID: userID, JobID: pgtype.Int8{Int64: jobID, Valid: true},
	})
	if err != nil {
		return "", ""
	}
	for _, row := range rows {
		if row.Direction == "incoming" {
			return row.FromAddr, row.FromName
		}
	}
	return "", ""
}

// cachedStrength reads one strength from the cached fit analysis, which derives it from the
// candidate's own CV. None yields "" and the draft omits the line, so it cannot invent a claim.
func (h *inboxHandlers) cachedStrength(ctx context.Context, userID, jobID int64) string {
	row, err := h.queries.GetUserJobAnalysis(ctx,
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
