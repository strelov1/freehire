package employer

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
	"github.com/strelov1/freehire/internal/platform/pgerr"
)

// Compile-time proof that QueriesRepository satisfies Repository.
var _ Repository = (*QueriesRepository)(nil)

// QueriesRepository adapts *db.Queries to Repository. No method here spans more than one
// statement, so unlike moderation/submission's repositories it needs no pool of its own —
// every write is a single INSERT/UPDATE/DELETE already atomic on its own.
type QueriesRepository struct {
	q *db.Queries
}

// NewQueriesRepository constructs a QueriesRepository.
func NewQueriesRepository(q *db.Queries) *QueriesRepository {
	return &QueriesRepository{q: q}
}

func (r *QueriesRepository) InsertPending(ctx context.Context, userID int64, companySlug, companyName, workEmail string) (Account, error) {
	row, err := r.q.InsertPendingCompanyAccount(ctx, db.InsertPendingCompanyAccountParams{
		UserID:      userID,
		CompanySlug: companySlug,
		CompanyName: companyName,
		WorkEmail:   workEmail,
	})
	if name, ok := pgerr.UniqueViolationConstraint(err); ok {
		if name == "company_accounts_pkey" {
			return Account{}, ErrAlreadyHasAccount
		}
		return Account{}, ErrCompanyAlreadyClaimed
	}
	if err != nil {
		return Account{}, err
	}
	return fromRow(row), nil
}

func (r *QueriesRepository) GetByUserID(ctx context.Context, userID int64) (Account, error) {
	row, err := r.q.GetCompanyAccountByUserID(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, err
	}
	return fromRow(row), nil
}

func (r *QueriesRepository) ListPending(ctx context.Context) ([]Account, error) {
	rows, err := r.q.ListPendingCompanyAccounts(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]Account, len(rows))
	for i, row := range rows {
		out[i] = fromRow(row)
	}
	return out, nil
}

func (r *QueriesRepository) Activate(ctx context.Context, userID int64) (Account, error) {
	row, err := r.q.ActivateCompanyAccount(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrClaimNotPending
	}
	if err != nil {
		return Account{}, err
	}
	return fromRow(row), nil
}

func (r *QueriesRepository) Revoke(ctx context.Context, userID int64) (Account, error) {
	row, err := r.q.RevokeCompanyAccount(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Account{}, ErrNotFound
	}
	if err != nil {
		return Account{}, err
	}
	return fromRow(row), nil
}

func (r *QueriesRepository) DeletePending(ctx context.Context, userID int64) error {
	n, err := r.q.DeleteCompanyAccount(ctx, userID)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrClaimNotPending
	}
	return nil
}

// ResolveCanonicalSlug reads company_slug_aliases by candidate: a no-match (candidate names
// no retired alias) is the ordinary case, not an error, so it returns candidate unchanged.
func (r *QueriesRepository) ResolveCanonicalSlug(ctx context.Context, candidate string) (string, error) {
	canonical, err := r.q.GetCompanySlugAlias(ctx, candidate)
	if errors.Is(err, pgx.ErrNoRows) {
		return candidate, nil
	}
	if err != nil {
		return "", err
	}
	return canonical, nil
}

func (r *QueriesRepository) ExistingCompany(ctx context.Context, slug string) (string, string, bool, error) {
	row, err := r.q.GetCompany(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	if err != nil {
		return "", "", false, err
	}
	return row.Name, websiteOf(row.CompanyInfo), true, nil
}

func (r *QueriesRepository) SeedCompanyWebsite(ctx context.Context, slug, name, website string) error {
	return r.q.SeedCompanyAccountWebsite(ctx, db.SeedCompanyAccountWebsiteParams{
		Slug:    slug,
		Name:    name,
		Website: website,
	})
}

// fromRow maps the generated db row to the package domain type.
func fromRow(row db.CompanyAccount) Account {
	return Account{
		UserID:      row.UserID,
		CompanySlug: row.CompanySlug,
		CompanyName: row.CompanyName,
		WorkEmail:   row.WorkEmail,
		Status:      row.Status,
		VerifiedAt:  pgconv.TimePtr(row.VerifiedAt),
		CreatedAt:   row.CreatedAt.Time,
	}
}

// websiteOf reads company_info's "website" key, "" when absent, blank, or the JSON is
// unreadable — an unusable value is never an error here, matching logodomain.Domain's own
// "unusable input is not an error" stance one layer up.
func websiteOf(companyInfo json.RawMessage) string {
	if len(companyInfo) == 0 {
		return ""
	}
	var fields struct {
		Website string `json:"website"`
	}
	if err := json.Unmarshal(companyInfo, &fields); err != nil {
		return ""
	}
	return fields.Website
}
