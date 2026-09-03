package boardcatalog

import (
	"context"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// CompanyEntry projects a stored board into the shape cmd/ingest feeds to
// pipeline.Runner — the same CompanyEntry a parsed sources/*.yml entry used to produce.
func (b Board) CompanyEntry() sources.CompanyEntry {
	return sources.CompanyEntry{
		Company:  b.Company,
		Provider: b.Provider,
		Board:    b.Board,
		Region:   b.Region,
		Hub:      b.Hub,
		Tenants:  b.Tenants,
	}
}

// LoadForProvider returns the boards cmd/ingest should crawl for one provider (pending
// and active, per Repository.ListActiveForProvider), as sources.CompanyEntry — cmd/ingest's
// replacement for parsing a sources/<provider>.yml file.
func LoadForProvider(ctx context.Context, repo Repository, provider string) ([]sources.CompanyEntry, error) {
	boards, err := repo.ListActiveForProvider(ctx, provider)
	if err != nil {
		return nil, err
	}
	entries := make([]sources.CompanyEntry, len(boards))
	for i, b := range boards {
		entries[i] = b.CompanyEntry()
	}
	return entries, nil
}
