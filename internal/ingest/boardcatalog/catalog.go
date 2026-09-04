// Package boardcatalog is the board catalog: which company crawls on which ATS, under
// what board id, and whether that board is proven to work. It replaces sources/*.yml
// (git) as what cmd/ingest reads, and absorbs the recognized (non-review) half of
// internal/ingest/contribution's lifecycle — a crowdsourced board starts pending and is
// proven by its first successful crawl.
package boardcatalog

import (
	"time"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// Status is a board's lifecycle stage.
type Status string

const (
	// StatusPending is a board recorded but not yet proven by a successful crawl. It is
	// crawled exactly like an active board.
	StatusPending Status = "pending"
	// StatusActive is a board whose first crawl completed without a board-level error.
	StatusActive Status = "active"
	// StatusRejected is a board that failed insert-time validation.
	StatusRejected Status = "rejected"
	// StatusRetired is a board a curator removed via cmd/add-board. Its row is kept,
	// not deleted.
	StatusRetired Status = "retired"
)

// InsertInput is a candidate board, before validation and persistence.
type InsertInput struct {
	Provider string
	Board    string
	Region   string
	Company  string
	Hub      bool
	Tenants  map[string]string
	// URL is the submitted link for a crowdsourced board, empty for one added by
	// cmd/add-board.
	URL string
	// SubmittedBy is the crowdsourced submitter, nil for one added by cmd/add-board.
	SubmittedBy *int64
	Surface     string
	// CreatedAt overrides the row's submission time. Zero — every live caller — lets the
	// column default to now(), which is what a board being added right now means.
	//
	// It exists for one caller: cmd/backfill-link-contributions, carrying submissions
	// made weeks before the catalog existed. Without it that worker had to INSERT into
	// boards directly, which skipped every check this package's whole point is to apply
	// (see Insert). One optional field is a smaller thing to carry than a fourth writer.
	CreatedAt time.Time
}

// Board is a stored catalog row.
type Board struct {
	ID             int64
	Provider       string
	Board          string
	Region         string
	Company        string
	Hub            bool
	Tenants        map[string]string
	URL            string
	Status         Status
	SubmittedBy    *int64
	Surface        string
	RejectedReason string
	CreatedAt      time.Time
	ActivatedAt    *time.Time
}

// Validate checks a candidate board against the adapter registry: the provider must be
// registered, and board must be non-empty unless the provider is boardless — the same
// checks cmd/validate-sources ran against a YAML entry.
func Validate(in InsertInput, registry map[string]sources.Source) error {
	cfg := sources.Config{
		Provider: in.Provider,
		Sources: []sources.CompanyEntry{{
			Company:  in.Company,
			Provider: in.Provider,
			Board:    in.Board,
			Region:   in.Region,
			Hub:      in.Hub,
			Tenants:  in.Tenants,
		}},
	}
	return cfg.Validate(registry)
}
