package handler

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/job/jobview"
)

func (h *jobsHandlers) GetResumeReviewPosting(ctx context.Context, params apigen.GetResumeReviewPostingParams) (*apigen.GetResumeReviewPostingOK, error) {
	posting, err := jobBySlugOrCode(ctx, h.queries, params.Slug)
	if err != nil {
		return nil, err
	}
	if posting.IsPrivate || posting.ClosedAt.Valid {
		return nil, pgx.ErrNoRows
	}
	job, err := jobview.FromRow(posting)
	if err != nil {
		return nil, err
	}
	requirements := make([]apigen.ResumeReviewRequirement, 0, len(job.Enrichment.Requirements))
	for _, requirement := range job.Enrichment.Requirements {
		requirements = append(requirements, apigen.ResumeReviewRequirement(requirement))
	}
	return &apigen.GetResumeReviewPostingOK{Data: apigen.ResumeReviewPosting{
		PublicSlug: posting.PublicSlug, Title: posting.Title, Company: posting.Company, Location: optString(posting.Location),
		Skills: job.Skills, Requirements: requirements,
	}}, nil
}
