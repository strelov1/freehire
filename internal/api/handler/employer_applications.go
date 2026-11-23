package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"mime"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/ingest/directapply"
	"github.com/strelov1/freehire/internal/search/search"
)

// The applicant inbox: the applications sent on freehire to the caller's own vacancies
// (internal/ingest/directapply.Inbox). Cookie-only like every employer route; the use case
// itself refuses an account that is not active.

func (h *employerHandlers) requireInbox() error {
	if h.inbox == nil {
		return fiber.NewError(fiber.StatusNotImplemented, "the applicant inbox is not available")
	}
	return nil
}

// inboxError maps the inbox's refusals: an application it cannot reach is named as such,
// an invalid request is rendered as directApplyError renders it, and the employer
// account's own refusals fall through to employerError.
func inboxError(err error) error {
	switch {
	case errors.Is(err, directapply.ErrNotFound):
		return fiber.NewError(fiber.StatusNotFound, "application not found")
	case errors.Is(err, directapply.ErrInvalid):
		return directApplyError(err)
	}
	return employerError(err)
}

func employerApplicationOut(a directapply.InboxApplication) apigen.EmployerApplication {
	out := apigen.EmployerApplication{
		ID:             a.ID,
		JobSlug:        a.JobSlug,
		JobTitle:       a.JobTitle,
		CandidateEmail: a.CandidateEmail,
		CvKind:         a.CVKind,
		HasCv:          a.HasCV,
		Note:           a.Note,
		Answers:        make([]apigen.EmployerApplicationAnswer, len(a.Answers)),
		Stage:          apigen.EmployerApplicationStage(a.Stage),
		SentAt:         a.SentAt,
	}
	for i, ans := range a.Answers {
		out.Answers[i] = apigen.EmployerApplicationAnswer{ID: ans.ID, Label: ans.Label, Type: string(ans.Type), Answer: ans.Answer}
	}
	out.EmployerNotes = apigen.NewOptString(a.EmployerNotes)
	if a.ViewedAt != nil {
		out.ViewedAt = apigen.NewOptDateTime(*a.ViewedAt)
	}
	if a.StageChangedAt != nil {
		out.StageChangedAt = apigen.NewOptDateTime(*a.StageChangedAt)
	}
	return out
}

// employerApplicationParams are the query params ListEmployerApplications reads.
var employerApplicationParams = []string{"job", "stage", "limit", "offset"}

// ListEmployerApplications lists one page of the caller's delivered applications.
func (h *employerHandlers) ListEmployerApplications(ctx context.Context, params apigen.ListEmployerApplicationsParams) (*apigen.EmployerApplicationsPage, error) {
	return h.employerApplicationPage(ctx, params.Job.Value, params.Stage.Value, "")
}

func (h *employerHandlers) SearchEmployerApplications(ctx context.Context, req *apigen.EmployerApplicationSearch, params apigen.SearchEmployerApplicationsParams) (*apigen.EmployerApplicationsPage, error) {
	return h.employerApplicationPage(ctx, params.Job.Value, params.Stage.Value, req.Query)
}

func (h *employerHandlers) employerApplicationPage(ctx context.Context, job, stage, query string) (*apigen.EmployerApplicationsPage, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.requireInbox(); err != nil {
		return nil, err
	}
	limit, offset, err := contractPageWindowed(ctx, directapply.DefaultInboxLimit, directapply.MaxInboxLimit)
	if err != nil {
		return nil, err
	}
	page, err := h.inbox.List(ctx, userID, directapply.InboxFilter{
		JobSlug: job,
		Stage:   directapply.Stage(stage),
		Search:  query,
		Limit:   int32(limit),  //nolint:gosec // bounded by MaxInboxLimit
		Offset:  int32(offset), //nolint:gosec // bounded by maxPageWindow
	})
	if err != nil {
		return nil, inboxError(err)
	}
	out := &apigen.EmployerApplicationsPage{
		Data: make([]apigen.EmployerApplication, len(page.Applications)),
		Meta: apigen.EmployerApplicationsMeta{
			Total:       page.Total,
			Limit:       int64(limit),
			Offset:      int64(offset),
			StageCounts: apigen.EmployerApplicationsMetaStageCounts{},
			// The listing reads only these; anything else widens the answer, so it is named.
			IgnoredParams: contractIgnoredParams(search.UnknownParamsAgainst(contractQuery(ctx), employerApplicationParams)),
		},
	}
	for i, a := range page.Applications {
		out.Data[i] = employerApplicationOut(a)
	}
	for st, n := range page.Counts {
		out.Meta.StageCounts[string(st)] = n
	}
	return out, nil
}

// GetEmployerApplicationCV serves the CV an application sent, as it was emailed. Inline,
// so the employer reads it in a tab; private and unstored, since it is a candidate's CV.
func (h *employerHandlers) GetEmployerApplicationCV(ctx context.Context, params apigen.GetEmployerApplicationCVParams) (*apigen.GetEmployerApplicationCVOKHeaders, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.requireInbox(); err != nil {
		return nil, err
	}
	id, err := contractPathID(params.ID)
	if err != nil {
		return nil, err
	}
	cv, err := h.inbox.CV(ctx, userID, id)
	if err != nil {
		return nil, inboxError(err)
	}
	return &apigen.GetEmployerApplicationCVOKHeaders{
		ContentType:        cv.ContentType,
		ContentDisposition: mime.FormatMediaType("inline", map[string]string{"filename": fmt.Sprintf("application-%d-%s", id, cv.Filename)}),
		CacheControl:       "private, no-store",
		Response:           apigen.GetEmployerApplicationCVOK{Data: bytes.NewReader(cv.Content)},
	}, nil
}

// MoveEmployerApplication moves an application to a stage of the caller's pipeline.
func (h *employerHandlers) MoveEmployerApplication(ctx context.Context, req *apigen.EmployerApplicationMove, params apigen.MoveEmployerApplicationParams) (*apigen.MoveEmployerApplicationOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.requireInbox(); err != nil {
		return nil, err
	}
	id, err := contractPathID(params.ID)
	if err != nil {
		return nil, err
	}
	res, err := h.inbox.Move(ctx, userID, id, directapply.Stage(req.Stage.Value), req.Message.Value)
	if err != nil {
		return nil, inboxError(err)
	}
	return &apigen.MoveEmployerApplicationOK{Data: apigen.EmployerApplicationMoved{
		Application:       employerApplicationOut(res.Application),
		CandidateNotified: res.Notified,
	}}, nil
}

func (h *employerHandlers) UpdateEmployerApplicationWorkspace(ctx context.Context, req *apigen.EmployerApplicationWorkspace, params apigen.UpdateEmployerApplicationWorkspaceParams) (*apigen.UpdateEmployerApplicationWorkspaceOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.requireInbox(); err != nil {
		return nil, err
	}
	id, err := contractPathID(params.ID)
	if err != nil {
		return nil, err
	}
	var notes *string
	if req.EmployerNotes.Set {
		notes = &req.EmployerNotes.Value
	}
	app, err := h.inbox.UpdateWorkspace(ctx, userID, id, req.MarkViewed.Value, notes)
	if err != nil {
		return nil, inboxError(err)
	}
	return &apigen.UpdateEmployerApplicationWorkspaceOK{Data: employerApplicationOut(app)}, nil
}
