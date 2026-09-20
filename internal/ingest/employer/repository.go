package employer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/job/job"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
	"github.com/strelov1/freehire/internal/platform/pgerr"
)

// Compile-time proof that QueriesRepository satisfies both Repository and JobRepository —
// one adapter for the whole package, matching Service's own single-type shape.
var (
	_ Repository    = (*QueriesRepository)(nil)
	_ JobRepository = (*QueriesRepository)(nil)
)

// QueriesRepository adapts *db.Queries to Repository. Every write is a single
// INSERT/UPDATE/DELETE except Update, which needs the pool for its own transaction — see
// Update's own doc comment for why (the same reason moderation.QueriesRepository carries one).
type QueriesRepository struct {
	q    *db.Queries
	pool *pgxpool.Pool
}

// NewQueriesRepository constructs a QueriesRepository.
func NewQueriesRepository(q *db.Queries, pool *pgxpool.Pool) *QueriesRepository {
	return &QueriesRepository{q: q, pool: pool}
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

// UpdateCompanyProfile builds the company_info merge object from Description/Website (only
// the keys the patch actually supplies — an unset field is simply absent from the object,
// so the SQL's merge leaves that key alone) and applies the rest via COALESCE-guarded
// params (nil = unchanged). Industries is applied through the existing SetCompanyIndustries
// separately, only when the patch supplies it — it replaces the whole array, and
// immediately zeroes industries_derived (SetCompanyIndustries' own documented behavior),
// which is correct here too: a company just curated by its own employer must not keep
// matching through a stale derived value.
func (r *QueriesRepository) UpdateCompanyProfile(ctx context.Context, slug string, patch CompanyProfilePatch) error {
	infoPatch := map[string]string{}
	if patch.Description != nil {
		infoPatch["description"] = *patch.Description
	}
	if patch.Website != nil {
		infoPatch["website"] = *patch.Website
	}
	infoJSON, err := json.Marshal(infoPatch)
	if err != nil {
		return err
	}

	if err := r.q.SetCompanyAccountProfile(ctx, db.SetCompanyAccountProfileParams{
		Slug:             slug,
		Tagline:          nullableText(patch.Tagline),
		CompanyInfoPatch: infoJSON,
		YearFounded:      pgconv.Int4(patch.YearFounded),
		EmployeeCount:    pgconv.Int4(patch.EmployeeCount),
		HqCountry:        nullableText(patch.HqCountry),
		Subindustry:      nullableText(patch.Subindustry),
	}); err != nil {
		return err
	}

	if patch.Industries != nil {
		if _, err := r.q.SetCompanyIndustries(ctx, db.SetCompanyIndustriesParams{
			Slug:       slug,
			Industries: patch.Industries,
		}); err != nil {
			return err
		}
	}
	return nil
}

// Owner reports who created the job at (jobSource, externalID) — the URL-collision guard
// CreateVacancy runs before ever delegating to the Minter.
func (r *QueriesRepository) Owner(ctx context.Context, externalID string) (int64, bool, error) {
	row, err := r.q.GetJobBySourceExternalID(ctx, db.GetJobBySourceExternalIDParams{
		Source:     jobSource,
		ExternalID: externalID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	// created_by is NOT NULL for every jobSource row (Create always stamps it), so an
	// invalid value here would be a bug in the write path, not a case to handle quietly.
	return row.CreatedBy.Int64, true, nil
}

// BySlug loads an employer-owned job by its public slug. Scoped in Go, not SQL, matching
// moderation.QueriesRepository.BySlug's own shape: a missing row, a different owner, a
// different source, or (defensively — jobSource rows are never private) is_private all
// collapse to the one ErrJobNotFound the caller cannot distinguish further anyway.
func (r *QueriesRepository) BySlug(ctx context.Context, actorID int64, slug string) (job.Job, job.Extras, error) {
	row, err := r.q.GetJobBySlug(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return job.Job{}, job.Extras{}, ErrJobNotFound
	}
	if err != nil {
		return job.Job{}, job.Extras{}, err
	}
	if row.Source != jobSource || !row.CreatedBy.Valid || row.CreatedBy.Int64 != actorID || row.IsPrivate {
		return job.Job{}, job.Extras{}, ErrJobNotFound
	}
	return job.FromRow(row)
}

// ListMine returns every jobSource-authored job actorID created, newest first, mapping each
// row through the same job.FromRow every other read path uses.
func (r *QueriesRepository) ListMine(ctx context.Context, actorID int64) ([]job.Job, []job.Extras, error) {
	rows, err := r.q.ListEmployerJobs(ctx, actorID)
	if err != nil {
		return nil, nil, err
	}
	jobs := make([]job.Job, len(rows))
	extras := make([]job.Extras, len(rows))
	for i, row := range rows {
		j, x, err := job.FromRow(row)
		if err != nil {
			return nil, nil, err
		}
		jobs[i], extras[i] = j, x
	}
	return jobs, extras, nil
}

// Update writes the full resulting row for an employer-owned job and enqueues it to
// search_outbox in the SAME transaction — mirroring moderation.QueriesRepository.Update
// exactly (see that method's own comment): an edit changes the title/description/derived
// facets search shows, and writing the row without queueing it would leave Meilisearch
// serving the stale content until the next full `make reindex`, which this repo's own ops
// docs note can be hours away. The query's own created_by/source scope (see
// UpdateEmployerJob) means a slug that is missing, another owner's, or another source's
// affects no row (ErrNoRows -> ErrJobNotFound) — the same belt-and-suspenders BySlug already
// applies on the read side.
func (r *QueriesRepository) Update(ctx context.Context, actorID int64, slug string, f job.Fields) (job.Job, job.Extras, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return job.Job{}, job.Extras{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := r.q.WithTx(tx)
	row, err := qtx.UpdateEmployerJob(ctx, f.UpdateEmployerParams(slug, actorID))
	if errors.Is(err, pgx.ErrNoRows) {
		return job.Job{}, job.Extras{}, ErrJobNotFound
	}
	if err != nil {
		return job.Job{}, job.Extras{}, err
	}
	if err := qtx.EnqueueSearchOutbox(ctx, row.ID); err != nil {
		return job.Job{}, job.Extras{}, fmt.Errorf("enqueue search outbox: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return job.Job{}, job.Extras{}, err
	}
	return job.FromRow(row)
}

// Close soft-closes an employer-owned job. closedCount is 0 both when the row does not
// exist/is not owned by actorID and when it was already closed (CloseEmployerJob's own
// closed_at IS NULL guard) — Close does not distinguish the two, because BySlug already ran
// first in every real call path (see Service.CloseVacancy) and settled ownership/existence;
// by the time this runs, 0 can only mean "already closed", which is success, not an error.
func (r *QueriesRepository) Close(ctx context.Context, actorID int64, slug string) error {
	_, err := r.q.CloseEmployerJob(ctx, db.CloseEmployerJobParams{
		PublicSlug: slug,
		ActorID:    actorID,
	})
	return err
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

// nullableText maps a CompanyProfilePatch field's *string ("the request did not supply
// this" = nil) to the pgtype SetCompanyAccountProfile's COALESCE guard expects: invalid
// means "leave the stored value alone." The inverse of pgconv.TextPtr, which this package
// does not otherwise need — internal/platform/pgconv has no *string-typed write-side
// adapter today, since every existing nullable-text write path has a natural "" zero value
// (pgconv.Text) rather than a real nil/unset distinction like a PATCH's own fields have.
func nullableText(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
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
