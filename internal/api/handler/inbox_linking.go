package handler

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/application/appevent"
	"github.com/strelov1/freehire/internal/application/autoapply"
	"github.com/strelov1/freehire/internal/application/inbox"
	"github.com/strelov1/freehire/internal/application/jobtracking"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
)

// GetTrackedApplication returns the caller's application for a job slug together
// with the emails linked to it. A slug the caller does not track is a 404.
func (h *inboxHandlers) GetTrackedApplication(ctx context.Context, params apigen.GetTrackedApplicationParams) (*apigen.GetTrackedApplicationOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	job, err := h.queries.GetJobBySlug(ctx, params.Slug)
	if err != nil {
		return nil, err
	}
	if err := jobVisibleToCaller(job, userID, true); err != nil {
		return nil, err
	}
	app, err := h.queries.GetUserApplication(ctx, db.GetUserApplicationParams{UserID: userID, JobID: job.ID})
	if err != nil {
		return nil, err // ErrNoRows → 404 (caller does not track this job)
	}
	rows, err := h.queries.ListJobEmails(ctx, db.ListJobEmailsParams{
		UserID: userID, JobID: pgtype.Int8{Int64: job.ID, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	jv, err := jobview.FromRow(job)
	if err != nil {
		return nil, err
	}
	jobJSON, err := json.Marshal(jv)
	if err != nil {
		return nil, err
	}

	emails := make([]apigen.InboxApplicationEmail, 0, len(rows))
	forSuggestion := make([]jobtracking.SuggestionEmail, 0, len(rows))
	for _, r := range rows {
		emails = append(emails, apigen.InboxApplicationEmail{
			ID: r.ID, Source: r.Source, FromAddr: r.FromAddr, FromName: r.FromName,
			Subject: r.Subject, StatusSignal: optString(pgStr(r.StatusSignal)), LinkSource: optString(pgStr(r.LinkSource)),
			ReceivedAt: r.ReceivedAt.Time, Read: r.Read,
		})
		forSuggestion = append(forSuggestion, jobtracking.SuggestionEmail{
			ID: r.ID, Signal: pgStr(r.StatusSignal), ReceivedAt: r.ReceivedAt.Time,
		})
	}

	// The candidate's own last word on the stage, which silences an offer they have already
	// answered. Mail-sourced stage_set rows are excluded because the mail path records its
	// own auto-advances there, and one of those silencing a suggestion computed from the
	// same messages would answer the question on the candidate's behalf. A failure here
	// must not cost them the application: the ledger read is an input to an optional
	// prompt, not to the record itself, so it degrades to "never set" — the direction that
	// shows the offer rather than hiding it.
	lastStageSet, err := h.queries.LastStageSetAt(ctx, db.LastStageSetAtParams{
		UserID: userID, JobID: pgtype.Int8{Int64: job.ID, Valid: true},
		ExcludeSources: appevent.MailSources,
	})
	if err != nil {
		lastStageSet = pgtype.Timestamptz{}
	}

	// The history. A failure here must not cost the caller their application: it is one
	// section of a panel whose other four tabs do not depend on it, so it degrades to an
	// empty history rather than a 500 on a page that was otherwise ready to render.
	events, err := h.timeline.ForApplication(ctx, userID, job.ID)
	if err != nil {
		events = nil
	}

	// A failure here must not cost the caller the rest of the panel either — the same
	// degrade-to-absent convention the history (events) and the stage-set read
	// (lastStageSet) above already follow.
	autoApply, err := autoApplyReviewInfoForJob(ctx, h.queries, userID, job.ID)
	if err != nil {
		log.Printf("auto-apply: reading review info for user %d job %d: %v", userID, job.ID, err)
		autoApply = nil
	}

	detail := apigen.InboxApplicationDetail{
		Job:          jobJSON,
		ViewedAt:     nilDateTime(tsPtr(app.ViewedAt)),
		SavedAt:      nilDateTime(tsPtr(app.SavedAt)),
		AppliedAt:    nilDateTime(tsPtr(app.AppliedAt)),
		Stage:        optString(pgStr(app.Stage)),
		Notes:        optString(pgStr(app.Notes)),
		FollowedUpAt: nilDateTime(tsPtr(app.FollowedUpAt)),
		Emails:       emails,
		Events:       timelineWire(events),
	}
	if s := jobtracking.SuggestStage(pgStr(app.Stage), forSuggestion, lastStageSet.Time); s != nil {
		detail.StageSuggestion = apigen.NewOptStageSuggestion(apigen.StageSuggestion{Stage: s.Stage, Signal: s.Signal, EmailID: s.EmailID})
	}
	if autoApply != nil {
		detail.AutoApply = apigen.NewOptAutoApplyReviewInfo(autoApplyReviewInfoOut(autoApply))
	}
	return &apigen.GetTrackedApplicationOK{Data: detail}, nil
}

// autoApplyReviewInfoOut maps the application layer's view of an attempt onto the contract's.
func autoApplyReviewInfoOut(info *autoapply.AutoApplyReviewInfo) apigen.AutoApplyReviewInfo {
	out := apigen.AutoApplyReviewInfo{
		Status:  apigen.AutoApplyReviewStatus(info.Status),
		QueueID: info.QueueID,
	}
	if info.MayHaveReachedEmployer {
		out.MayHaveReachedEmployer = apigen.NewOptBool(true)
	}
	if p := info.ResolvedPreview; p != nil {
		preview := apigen.AutoApplyResolvedPreview{Fields: make([]apigen.AutoApplyPreviewField, len(p.Fields))}
		for i, f := range p.Fields {
			preview.Fields[i] = apigen.AutoApplyPreviewField{Label: f.Label, Value: f.Value}
		}
		for _, q := range p.Pending {
			preview.Pending = append(preview.Pending, apigen.AutoApplyPreviewPending{Label: q.Label, WillDraftAtSubmission: q.WillDraftAtSubmission})
		}
		out.ResolvedPreview = apigen.NewOptAutoApplyResolvedPreview(preview)
	}
	for _, u := range info.Unmapped {
		out.Unmapped = append(out.Unmapped, apigen.AutoApplyUnmappedField{ID: u.ID, Label: u.Label, Required: u.Required, Reason: u.Reason})
	}
	return out
}

// ConfirmEmailLink promotes an email's pending suggestion to a confirmed manual
// link. A 404 when the email is not the caller's or carries no suggestion.
func (h *inboxHandlers) ConfirmEmailLink(ctx context.Context, params apigen.ConfirmEmailLinkParams) (*apigen.ConfirmEmailLinkOK, error) {
	msg, err := h.renderMutation(ctx, params.ID, func(userID, id int64) (inbox.Message, error) {
		return h.inbox.ResolveSuggestion(ctx, userID, id, true)
	})
	if err != nil {
		return nil, err
	}
	return &apigen.ConfirmEmailLinkOK{Data: msg}, nil
}

// RejectEmailLink dismisses an email's pending suggestion without linking.
func (h *inboxHandlers) RejectEmailLink(ctx context.Context, params apigen.RejectEmailLinkParams) (*apigen.RejectEmailLinkOK, error) {
	msg, err := h.renderMutation(ctx, params.ID, func(userID, id int64) (inbox.Message, error) {
		return h.inbox.ResolveSuggestion(ctx, userID, id, false)
	})
	if err != nil {
		return nil, err
	}
	return &apigen.RejectEmailLinkOK{Data: msg}, nil
}

// UnlinkEmail clears an email's application link.
func (h *inboxHandlers) UnlinkEmail(ctx context.Context, params apigen.UnlinkEmailParams) (*apigen.UnlinkEmailOK, error) {
	msg, err := h.renderMutation(ctx, params.ID, func(userID, id int64) (inbox.Message, error) {
		return h.inbox.Unlink(ctx, userID, id)
	})
	if err != nil {
		return nil, err
	}
	return &apigen.UnlinkEmailOK{Data: msg}, nil
}

// LinkEmail manually links an email to the application named by {"slug": …}.
func (h *inboxHandlers) LinkEmail(ctx context.Context, req apigen.OptInboxLinkTarget, params apigen.LinkEmailParams) (*apigen.LinkEmailOK, error) {
	slug, err := slugFromBody(req)
	if err != nil {
		return nil, err
	}
	msg, err := h.renderMutation(ctx, params.ID, func(userID, id int64) (inbox.Message, error) {
		return h.inbox.Link(ctx, userID, id, slug)
	})
	if err != nil {
		return nil, err
	}
	return &apigen.LinkEmailOK{Data: msg}, nil
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
func (h *inboxHandlers) CreateApplicationFromEmail(ctx context.Context, req apigen.OptInboxLinkTarget, params apigen.CreateApplicationFromEmailParams) (*apigen.CreateApplicationFromEmailOK, error) {
	slug, err := slugFromBody(req)
	if err != nil {
		return nil, err
	}
	msg, err := h.renderMutation(ctx, params.ID, func(userID, id int64) (inbox.Message, error) {
		return h.inbox.RecordApplication(ctx, userID, id, slug)
	})
	if err != nil {
		return nil, err
	}
	return &apigen.CreateApplicationFromEmailOK{Data: msg}, nil
}

// slugFromBody reads the {"slug": …} body shared by the linking endpoints.
func slugFromBody(req apigen.OptInboxLinkTarget) (string, error) {
	slug, _ := req.Value.Slug.Get()
	if slug == "" {
		return "", fiber.NewError(fiber.StatusBadRequest, "slug required")
	}
	return slug, nil
}

// renderMutation runs one email mutation for the caller and returns the message it
// changed. It is the shared tail of link, unlink, confirm, reject, triage and
// application-from-mail, so none of them can drift from another or from GetEmail —
// and unlike GetEmail it does not mark the message read: the caller is acting on
// the message, not opening it.
func (h *inboxHandlers) renderMutation(ctx context.Context, rawID string, do func(userID, id int64) (inbox.Message, error)) (apigen.InboxEmail, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return apigen.InboxEmail{}, err
	}
	id, err := emailPathID(rawID)
	if err != nil {
		return apigen.InboxEmail{}, err
	}
	msg, err := do(userID, id)
	if err != nil {
		return apigen.InboxEmail{}, err
	}
	return bodyOf(msg), nil
}

// tsPtr unwraps a nullable timestamp to *time.Time (nil when NULL).
func tsPtr(t pgtype.Timestamptz) *time.Time {
	if t.Valid {
		return &t.Time
	}
	return nil
}
