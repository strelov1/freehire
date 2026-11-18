package handler

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/ingest/linkimport"
	"github.com/strelov1/freehire/internal/ingest/moderation"
	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/ingest/submission"
	"github.com/strelov1/freehire/internal/platform/db"
)

// submissionHandlers serves the public job-submission queue: any authenticated user submits
// a vacancy and reads their own queue; the review actions are moderator-gated. Approval
// mints a live job via moderation. Prefill reuses the paste-a-link importer but never writes.
type submissionHandlers struct {
	submission *submission.Service
	importer   *linkimport.Importer
	// queries and postings answer "is this vacancy already in the catalogue" via
	// catalogSlugForURL. Zero-value postings = the offline rewrite only.
	queries  *db.Queries
	postings sources.PostingURLResolver
}

func newSubmissionHandlers(queries *db.Queries, pool *pgxpool.Pool, moderation *moderation.Service, importer *linkimport.Importer, postings sources.PostingURLResolver) *submissionHandlers {
	// Approval mints through moderation, reusing derivation, dedup and the enrichment enqueue.
	return &submissionHandlers{
		submission: submission.New(submission.NewQueriesRepository(queries, pool), moderation),
		importer:   importer,
		queries:    queries,
		postings:   postings,
	}
}

func (h *submissionHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	// Submit and read own queue: cookie or API key. Pending queue, approve and reject are
	// moderator-gated. Approval takes the CreateJob path, so it equals a hand-curated job.
	api.Post("/submissions", mw.key, contract)
	api.Post("/submissions/prefill", mw.key, contract)
	api.Get("/me/submissions", mw.key, contract)
	api.Get("/submissions", mw.key, mw.moderator, contract)
	api.Post("/submissions/:id/approve", mw.key, mw.moderator, contract)
	api.Post("/submissions/:id/reject", mw.key, mw.moderator, contract)
}

// withStructured copies the submission's structured facets onto the response (shared tail of
// the three mappers). submitter_email is set only on the moderator queue; submitted_by is never sent.
func withStructured(resp apigen.Submission, s submission.Submission) apigen.Submission {
	resp.Skills = nilIfEmpty(s.Skills)
	resp.Regions = nilIfEmpty(s.Regions)
	resp.Cities = nilIfEmpty(s.Cities)
	resp.WorkMode = optString(s.WorkMode)
	resp.EmploymentType = optString(s.EmploymentType)
	resp.Seniority = optString(s.Seniority)
	if s.SalaryMin != nil {
		resp.SalaryMin = apigen.NewOptInt64(int64(*s.SalaryMin))
	}
	if s.SalaryMax != nil {
		resp.SalaryMax = apigen.NewOptInt64(int64(*s.SalaryMax))
	}
	resp.SalaryCurrency = optString(s.SalaryCurrency)
	resp.SalaryPeriod = optString(s.SalaryPeriod)
	return resp
}

// timeString renders a time the way time.Time marshalled to JSON.
func timeString(t *time.Time) apigen.OptString {
	if t == nil {
		return apigen.OptString{}
	}
	return apigen.NewOptString(t.Format(time.RFC3339Nano))
}

// toSubmissionResponse maps a stored submission to its wire shape (no submitter email).
func toSubmissionResponse(s submission.Submission) apigen.Submission {
	created := apigen.NilString{Null: true}
	if s.CreatedAt != nil {
		created = apigen.NewNilString(s.CreatedAt.Format(time.RFC3339Nano))
	}
	return withStructured(apigen.Submission{
		ID:           s.ID,
		URL:          s.URL,
		Source:       optString(s.Source),
		Title:        s.Title,
		Company:      s.Company,
		Location:     optString(s.Location),
		Remote:       s.Remote,
		Description:  optString(s.Description),
		PostedAt:     timeString(s.PostedAt),
		Status:       apigen.SubmissionStatus(s.Status),
		ReviewReason: optString(s.ReviewReason),
		ReviewedAt:   timeString(s.ReviewedAt),
		CreatedAt:    created,
	}, s)
}

// toPendingSubmissionResponse maps a moderator-queue row, adding the submitter's email.
func toPendingSubmissionResponse(r submission.PendingSubmission) apigen.Submission {
	resp := toSubmissionResponse(r.Submission)
	resp.SubmitterEmail = optString(r.SubmitterEmail)
	return resp
}

// toMySubmissionResponse maps a "my submissions" row, adding the minted job's slug when
// the submission was approved (empty otherwise).
func toMySubmissionResponse(r submission.UserSubmission) apigen.Submission {
	resp := toSubmissionResponse(r.Submission)
	resp.JobSlug = optString(r.JobSlug)
	return resp
}

// submissionError maps the submission sentinels onto HTTP statuses. moderation.ErrInvalid
// carries a user-facing 400 message; anything unmapped falls through as a 500.
func submissionError(err error) error {
	switch {
	case errors.Is(err, submission.ErrSubmissionNotFound):
		return fiber.NewError(fiber.StatusNotFound, "submission not found")
	case errors.Is(err, submission.ErrDuplicatePending):
		return fiber.NewError(fiber.StatusConflict, "a pending submission for this URL already exists")
	case errors.Is(err, submission.ErrAlreadyDecided):
		return fiber.NewError(fiber.StatusConflict, "submission already decided")
	case errors.Is(err, submission.ErrBlockedDomain):
		return fiber.NewError(fiber.StatusForbidden, "this domain is not accepted")
	case errors.Is(err, moderation.ErrInvalid):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	default:
		return err
	}
}

// submissionInput maps the submit body onto the moderation create input. An omitted or null
// field is the zero value; the service refuses a missing required one.
func submissionInput(in *apigen.SubmissionRequest) (moderation.CreateInput, error) {
	var postedAt *time.Time
	if v := in.PostedAt.Value; v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return moderation.CreateInput{}, fiber.NewError(fiber.StatusBadRequest, "invalid request body")
		}
		postedAt = &t
	}
	return moderation.CreateInput{
		URL:            in.URL.Value,
		Source:         in.Source.Value,
		Title:          in.Title.Value,
		Company:        in.Company.Value,
		Location:       in.Location.Value,
		Remote:         in.Remote.Value,
		Description:    in.Description.Value,
		PostedAt:       postedAt,
		Skills:         in.Skills.Value,
		Regions:        in.Regions.Value,
		Cities:         in.Cities.Value,
		WorkMode:       in.WorkMode.Value,
		EmploymentType: in.EmploymentType.Value,
		Seniority:      in.Seniority.Value,
		SalaryMin:      intPtrOf(in.SalaryMin.Get),
		SalaryMax:      intPtrOf(in.SalaryMax.Get),
		SalaryCurrency: in.SalaryCurrency.Value,
		SalaryPeriod:   in.SalaryPeriod.Value,
	}, nil
}

// CreateSubmission queues a user-contributed vacancy for moderation (cookie or API key). A bad
// body is a 400 before any write; a duplicate pending URL is a 409, and so is a URL the
// catalogue already carries, since approving it would mint a second "manual" row that ingest
// dedup does not collapse. Returns the pending submission with 201.
func (h *submissionHandlers) CreateSubmission(ctx context.Context, req *apigen.SubmissionRequest) (*apigen.CreateSubmissionCreated, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	input, err := submissionInput(req)
	if err != nil {
		return nil, err
	}

	slug, err := catalogSlugForURL(ctx, h.queries, h.postings, strings.TrimSpace(input.URL))
	if err != nil {
		return nil, err
	}
	if slug != "" {
		return nil, fiber.NewError(fiber.StatusConflict, "this job is already on freehire: /jobs/"+slug)
	}

	sub, err := h.submission.Submit(ctx, userID, input)
	if err != nil {
		return nil, submissionError(err)
	}
	return &apigen.CreateSubmissionCreated{Data: toSubmissionResponse(sub)}, nil
}

// PrefillSubmission parses a job URL through the paste-a-link registry
// (internal/ingest/linksource) and writes nothing: no job, submission, credit, enrichment or
// search push. Skills is included only when the platform states it in a STRUCTURED field
// (sources.Job.Skills); Cities is never included. A URL the catalogue already carries answers
// with ExistingJobSlug only. An unrecognized URL, or one that is not a single vacancy page,
// is not an error: 200 with every field empty.
func (h *submissionHandlers) PrefillSubmission(ctx context.Context, req *apigen.PrefillRequest) (*apigen.PrefillSubmissionOK, error) {
	if _, err := requireUserIDFromContext(ctx); err != nil {
		return nil, err
	}

	// Asked before the outbound fetch, which a known vacancy does not need.
	slug, err := catalogSlugForURL(ctx, h.queries, h.postings, strings.TrimSpace(req.URL.Value))
	if err != nil {
		return nil, err
	}
	if slug != "" {
		return &apigen.PrefillSubmissionOK{Data: apigen.SubmissionPrefill{ExistingJobSlug: apigen.NewOptString(slug)}}, nil
	}

	resolved, ok, err := h.importer.Resolve(ctx, req.URL.Value, linkimport.Board{})
	if err != nil || !ok {
		return &apigen.PrefillSubmissionOK{}, nil
	}
	return &apigen.PrefillSubmissionOK{Data: apigen.SubmissionPrefill{
		Title:          optString(resolved.Job.Title),
		Company:        optString(resolved.Job.Company),
		Location:       optString(resolved.Job.Location),
		Description:    optString(resolved.Job.Description),
		WorkMode:       optString(resolved.Job.WorkMode),
		EmploymentType: optString(resolved.Job.EmploymentType),
		Seniority:      optString(resolved.Job.Seniority),
		Skills:         nilIfEmpty(resolved.Job.Skills),
		Source:         optString(resolved.Source),
	}}, nil
}

// ListMySubmissions returns the caller's own submissions with status and any rejection reason.
func (h *submissionHandlers) ListMySubmissions(ctx context.Context) (*apigen.ListMySubmissionsOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}

	subs, err := h.submission.ListMine(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]apigen.Submission, len(subs))
	for i, s := range subs {
		out[i] = toMySubmissionResponse(s)
	}
	return &apigen.ListMySubmissionsOK{Data: out}, nil
}

// ListPendingSubmissions returns the moderator review queue (with submitter emails); role-gated.
func (h *submissionHandlers) ListPendingSubmissions(ctx context.Context) (*apigen.ListPendingSubmissionsOK, error) {
	rows, err := h.submission.ListPending(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]apigen.Submission, len(rows))
	for i, r := range rows {
		out[i] = toPendingSubmissionResponse(r)
	}
	return &apigen.ListPendingSubmissionsOK{Data: out}, nil
}

// ApproveSubmission mints a live vacancy from a pending submission and marks it approved.
// Role-gated. An unknown id is a 404; a submission already decided is a 409.
func (h *submissionHandlers) ApproveSubmission(ctx context.Context, params apigen.ApproveSubmissionParams) (*apigen.ApproveSubmissionOK, error) {
	reviewerID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := contractPathID(params.ID)
	if err != nil {
		return nil, err
	}

	sub, err := h.submission.Approve(ctx, reviewerID, id)
	if err != nil {
		return nil, submissionError(err)
	}
	return &apigen.ApproveSubmissionOK{Data: toSubmissionResponse(sub)}, nil
}

// RejectSubmission marks a pending submission rejected with an optional reason. Role-gated.
// With block_domain set, the URL's host goes on the blocklist and every other pending
// submission on that host is rejected too (see submission.Service.Reject); the response is
// only the target submission.
func (h *submissionHandlers) RejectSubmission(ctx context.Context, req apigen.OptRejectSubmissionRequest, params apigen.RejectSubmissionParams) (*apigen.RejectSubmissionOK, error) {
	reviewerID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := contractPathID(params.ID)
	if err != nil {
		return nil, err
	}

	sub, err := h.submission.Reject(ctx, reviewerID, id, req.Value.Reason.Value, req.Value.BlockDomain.Value)
	if err != nil {
		return nil, submissionError(err)
	}
	return &apigen.RejectSubmissionOK{Data: toSubmissionResponse(sub)}, nil
}
