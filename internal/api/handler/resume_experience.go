package handler

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/candidate/experience"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
)

// experienceBank is what the résumé surfaces need from the experience bank: the upload
// feeds it, and the status read serves the work history back.
type experienceBank interface {
	Import(ctx context.Context, userID int64, entries []experience.ImportEntry, sourceRef string) (experience.ImportResult, error)
	// SeedHistory keeps jobs and projects apart for the Profile / CV-seed shape.
	SeedHistory(ctx context.Context, userID int64) (experience.SeedHistory, error)
}

// importExperience banks the work history of a freshly-extracted résumé. Best-effort: an
// unconfigured bank, no work history, or a failing import is logged (never the CV text) and
// swallowed, leaving the upload and the structured résumé untouched.
func (h *resumeHandlers) importExperience(ctx context.Context, userID int64, st resumeextract.Structured, sourceRef string) {
	if h.bank == nil {
		return
	}
	entries := experience.EntriesFromResume(st)
	if len(entries) == 0 {
		return
	}
	result, err := h.bank.Import(ctx, userID, entries, sourceRef)
	if err != nil {
		log.Printf("experience import: user %d: %v", userID, err)
		return
	}
	log.Printf("experience import: user %d: %d employments created, %d filled, %d atoms created, %d already banked",
		userID, result.EmploymentsCreated, result.EmploymentsFilled, result.AtomsCreated, result.AtomsSkipped)
}
