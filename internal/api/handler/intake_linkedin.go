package handler

import (
	"context"
	"log"

	"github.com/strelov1/freehire/internal/ingest/atsboard"
	"github.com/strelov1/freehire/internal/ingest/contribution"
	"github.com/strelov1/freehire/internal/ingest/linkimport"
)

func (s *intakeService) resolveLinkedIn(ctx context.Context, userID int64, raw, surface string) (intakeOutcome, error) {
	prepared, readErr := s.imports.PrepareLinkedIn(ctx, raw)
	if readErr != nil {
		log.Printf("intake: LinkedIn discovery %s: %v", raw, readErr)
	}
	candidate := contribution.Intake{Source: prepared.Board.Source, Board: prepared.Board.Board,
		Canonical: prepared.BoardURL, Region: atsboard.Region(prepared.BoardURL)}
	if prepared.VerifiedBoard != nil {
		entry := *prepared.VerifiedBoard
		candidate = contribution.Intake{Source: entry.Provider, Board: entry.Board,
			Region: entry.Region, Canonical: prepared.BoardURL, Company: entry.Company}
	}
	if prepared.Native != nil {
		candidate.Canonical = prepared.Native.Job.URL
	}
	intake, err := s.contribution.InspectResolved(ctx, candidate)
	if err != nil {
		return intakeOutcome{}, err
	}
	if readErr == nil && intake.Company == "" {
		intake.Company = prepared.Posting.Company
	}
	// Recording now creates the native board. Do it before WriteNative so its
	// submitter and surface are retained instead of losing attribution to discovery.
	companySlug, novel, err := s.record(ctx, userID, raw, surface, intake)
	if err != nil {
		return intakeOutcome{}, err
	}
	if readErr != nil {
		if err := s.imports.AdoptLinkedInBoard(ctx, prepared.Board); err != nil {
			return intakeOutcome{}, err
		}
	}
	var res linkimport.Result
	var imported bool
	if readErr == nil {
		var err error
		res, imported, err = s.imports.WriteLinkedIn(ctx, prepared)
		if err != nil {
			log.Printf("intake: LinkedIn import %s: %v", raw, err)
		}
	}
	return s.intakeResult(ctx, intake, res, imported, companySlug, novel), nil
}
