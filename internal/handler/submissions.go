package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/moderation"
	"github.com/strelov1/freehire/internal/submission"
)

// submissionResponse is the public shape of a job submission. submitted_by is omitted
// (ownership, internal); submitter_email is set only on the moderator queue. The content
// fields mirror the submit body so a user can see exactly what they sent.
type submissionResponse struct {
	ID             int64      `json:"id"`
	URL            string     `json:"url"`
	Source         string     `json:"source,omitempty"`
	Title          string     `json:"title"`
	Company        string     `json:"company"`
	Location       string     `json:"location,omitempty"`
	Remote         bool       `json:"remote"`
	Description    string     `json:"description,omitempty"`
	PostedAt       *time.Time `json:"posted_at,omitempty"`
	Status         string     `json:"status"`
	ReviewReason   string     `json:"review_reason,omitempty"`
	ReviewedAt     *time.Time `json:"reviewed_at,omitempty"`
	CreatedAt      *time.Time `json:"created_at"`
	SubmitterEmail string     `json:"submitter_email,omitempty"`
	// JobSlug is the public slug of the minted live vacancy, set only on an approved
	// submission in the "my submissions" view, so the UI can link to /jobs/<slug>.
	JobSlug string `json:"job_slug,omitempty"`
}

// toSubmissionResponse maps a stored submission to its wire shape (no submitter email).
func toSubmissionResponse(s db.JobSubmission) submissionResponse {
	return submissionResponse{
		ID:           s.ID,
		URL:          s.URL,
		Source:       s.Source,
		Title:        s.Title,
		Company:      s.Company,
		Location:     s.Location,
		Remote:       s.Remote,
		Description:  s.Description,
		PostedAt:     timePtr(s.PostedAt),
		Status:       s.Status,
		ReviewReason: s.ReviewReason,
		ReviewedAt:   timePtr(s.ReviewedAt),
		CreatedAt:    timePtr(s.CreatedAt),
	}
}

// toPendingSubmissionResponse maps a moderator-queue row, adding the submitter's email.
func toPendingSubmissionResponse(r db.ListPendingSubmissionsRow) submissionResponse {
	return submissionResponse{
		ID:             r.ID,
		URL:            r.URL,
		Source:         r.Source,
		Title:          r.Title,
		Company:        r.Company,
		Location:       r.Location,
		Remote:         r.Remote,
		Description:    r.Description,
		PostedAt:       timePtr(r.PostedAt),
		Status:         r.Status,
		ReviewReason:   r.ReviewReason,
		ReviewedAt:     timePtr(r.ReviewedAt),
		CreatedAt:      timePtr(r.CreatedAt),
		SubmitterEmail: r.SubmitterEmail,
	}
}

// toMySubmissionResponse maps a "my submissions" row, adding the minted job's slug when
// the submission was approved (job_slug is NULL otherwise).
func toMySubmissionResponse(r db.ListSubmissionsByUserRow) submissionResponse {
	return submissionResponse{
		ID:           r.ID,
		URL:          r.URL,
		Source:       r.Source,
		Title:        r.Title,
		Company:      r.Company,
		Location:     r.Location,
		Remote:       r.Remote,
		Description:  r.Description,
		PostedAt:     timePtr(r.PostedAt),
		Status:       r.Status,
		ReviewReason: r.ReviewReason,
		ReviewedAt:   timePtr(r.ReviewedAt),
		CreatedAt:    timePtr(r.CreatedAt),
		JobSlug:      r.JobSlug.String,
	}
}

// submissionError maps the submission sentinels onto HTTP statuses. moderation.ErrInvalid
// (raised by the shared content validation) carries a user-facing 400 message; the rest
// map to not-found / conflict. Anything else falls through to RenderError as a 500.
func submissionError(err error) error {
	switch {
	case errors.Is(err, submission.ErrSubmissionNotFound):
		return fiber.NewError(fiber.StatusNotFound, "submission not found")
	case errors.Is(err, submission.ErrDuplicatePending):
		return fiber.NewError(fiber.StatusConflict, "a pending submission for this URL already exists")
	case errors.Is(err, submission.ErrAlreadyDecided):
		return fiber.NewError(fiber.StatusConflict, "submission already decided")
	case errors.Is(err, moderation.ErrInvalid):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	default:
		return err
	}
}

// CreateSubmission queues a user-contributed vacancy for moderation. Authenticated by
// cookie or API key; the content is validated by the service (a bad body is a 400 before
// any write), and a duplicate pending URL is a 409. Returns the pending submission with 201.
func (a *API) CreateSubmission(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	var in createJobRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}

	sub, err := a.submission.Submit(c.Context(), userID, in.toCreateInput())
	if err != nil {
		return submissionError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": toSubmissionResponse(sub)})
}

// ListMySubmissions returns the caller's own submissions with their status and any
// rejection reason. Scoped to the authenticated user, so it never reveals another user's.
func (a *API) ListMySubmissions(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}

	subs, err := a.submission.ListMine(c.Context(), userID)
	if err != nil {
		return err
	}
	out := make([]submissionResponse, len(subs))
	for i, s := range subs {
		out[i] = toMySubmissionResponse(s)
	}
	return c.JSON(fiber.Map{"data": out})
}

// ListPendingSubmissions returns the moderator review queue (with submitter emails). The
// route is role-gated, so reaching this handler already implies a moderator.
func (a *API) ListPendingSubmissions(c *fiber.Ctx) error {
	rows, err := a.submission.ListPending(c.Context())
	if err != nil {
		return err
	}
	out := make([]submissionResponse, len(rows))
	for i, r := range rows {
		out[i] = toPendingSubmissionResponse(r)
	}
	return c.JSON(fiber.Map{"data": out})
}

// ApproveSubmission mints a live vacancy from a pending submission and marks it approved.
// Role-gated. An unknown id is a 404; a submission already decided is a 409.
func (a *API) ApproveSubmission(c *fiber.Ctx) error {
	reviewerID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}

	sub, err := a.submission.Approve(c.Context(), reviewerID, id)
	if err != nil {
		return submissionError(err)
	}
	return c.JSON(fiber.Map{"data": toSubmissionResponse(sub)})
}

// rejectRequest is the optional rejection reason body.
type rejectRequest struct {
	Reason string `json:"reason"`
}

// RejectSubmission marks a pending submission rejected with an optional reason. Role-gated.
// The reason body is optional, so a parse failure (e.g. empty body) leaves the reason blank
// rather than rejecting the request.
func (a *API) RejectSubmission(c *fiber.Ctx) error {
	reviewerID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}

	var in rejectRequest
	_ = c.BodyParser(&in)

	sub, err := a.submission.Reject(c.Context(), reviewerID, id, in.Reason)
	if err != nil {
		return submissionError(err)
	}
	return c.JSON(fiber.Map{"data": toSubmissionResponse(sub)})
}

// timePtr maps a nullable DB timestamp to an optional time for the wire shape.
func timePtr(t pgtype.Timestamptz) *time.Time {
	if !t.Valid {
		return nil
	}
	return &t.Time
}
