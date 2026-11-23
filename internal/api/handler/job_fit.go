package handler

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/ai/enrich"
	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
)

type toolJobListing struct {
	PublicSlug string `json:"public_slug"`
	Title      string `json:"title"`
	Company    string `json:"company"`
	Location   string `json:"location"`
}

func toolListing(job db.Job) toolJobListing {
	return toolJobListing{
		PublicSlug: job.PublicSlug,
		Title:      job.Title,
		Company:    job.Company,
		Location:   job.Location,
	}
}

type resumeReviewPosting struct {
	toolJobListing
	Skills       []string             `json:"skills"`
	Requirements []enrich.Requirement `json:"requirements,omitempty"`
}

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
	data, err := json.Marshal(resumeReviewPosting{
		toolJobListing: toolListing(posting),
		Skills:         job.Skills,
		Requirements:   job.Enrichment.Requirements,
	})
	if err != nil {
		return nil, err
	}
	return &apigen.GetResumeReviewPostingOK{Data: data}, nil
}
