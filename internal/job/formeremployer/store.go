package formeremployer

import (
	"context"
	"fmt"

	"github.com/strelov1/freehire/internal/platform/db"
)

// queries is the slice of *db.Queries the lookup reads: the caller's own banked employers,
// the alias registry, and the hiring companies its keys can land on.
type queries interface {
	ListPastEmployerNames(ctx context.Context, userID int64) ([]db.ListPastEmployerNamesRow, error)
	ResolveCompanySlugAliases(ctx context.Context, foldedKeys []string) ([]db.ResolveCompanySlugAliasesRow, error)
	ListHiringCompaniesByFoldedSlug(ctx context.Context, arg db.ListHiringCompaniesByFoldedSlugParams) ([]db.ListHiringCompaniesByFoldedSlugRow, error)
}

// Store answers the filter for one signed-in person.
type Store struct {
	q queries
}

// NewStore wraps the queries the lookup reads. *db.Queries satisfies it.
func NewStore(q queries) *Store { return &Store{q: q} }

// Slugs returns the hiring companies the user has worked at and left, resolved by Resolve's
// rules. An empty answer is a real one — nothing banked, or nothing that resolves — and the
// caller must narrow to nothing rather than drop the filter. Three small reads, each
// index-backed: the bank by user, the registry by folded key, companies by folded slug.
func (s *Store) Slugs(ctx context.Context, userID int64) ([]string, error) {
	rows, err := s.q.ListPastEmployerNames(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("former employers: bank: %w", err)
	}
	emps := make([]Employment, len(rows))
	for i, r := range rows {
		emps[i] = Employment{Company: r.Company, Location: r.Location}
	}
	keys := Keys(emps)
	if len(keys) == 0 {
		return nil, nil
	}

	aliasRows, err := s.q.ResolveCompanySlugAliases(ctx, keys)
	if err != nil {
		return nil, fmt.Errorf("former employers: aliases: %w", err)
	}
	canons := make(map[string][]string, len(aliasRows))
	var canonSlugs []string
	for _, r := range aliasRows {
		canons[r.FoldedKey] = append(canons[r.FoldedKey], r.CanonicalSlug)
		canonSlugs = append(canonSlugs, r.CanonicalSlug)
	}

	companyRows, err := s.q.ListHiringCompaniesByFoldedSlug(ctx, db.ListHiringCompaniesByFoldedSlugParams{
		FoldedKeys: keys,
		Slugs:      canonSlugs,
	})
	if err != nil {
		return nil, fmt.Errorf("former employers: companies: %w", err)
	}
	companies := make([]Company, len(companyRows))
	for i, r := range companyRows {
		companies[i] = Company{Slug: r.Slug, Countries: r.Countries}
	}
	return Resolve(emps, canons, companies), nil
}
