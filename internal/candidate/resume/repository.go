package resume

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/strelov1/freehire/internal/platform/db"
)

// QueriesRepository adapts *db.Queries to Repository, mapping the pointer to the nullable
// users columns. pool is what WithUploadLock opens its transaction on.
type QueriesRepository struct {
	q    *db.Queries
	pool *pgxpool.Pool
}

// NewQueriesRepository wraps *db.Queries as a Repository.
func NewQueriesRepository(q *db.Queries) *QueriesRepository { return &QueriesRepository{q: q} }

// WithUploadLocks gives the repository the pool WithUploadLock needs. Every wiring that
// stores CVs must set it; one that only reads them (a worker) has nothing to serialise.
func (r *QueriesRepository) WithUploadLocks(pool *pgxpool.Pool) *QueriesRepository {
	r.pool = pool
	return r
}

// WithUploadLock runs fn in a transaction holding the user's upload lock, with a repository
// bound to that transaction. Without a pool it runs fn unlocked.
func (r *QueriesRepository) WithUploadLock(ctx context.Context, userID int64, fn func(Repository) error) error {
	if r.pool == nil {
		return fn(r)
	}
	return pgx.BeginFunc(ctx, r.pool, func(tx pgx.Tx) error {
		q := r.q.WithTx(tx)
		if err := q.LockUserResumeUpload(ctx, userID); err != nil {
			return err
		}
		return fn(&QueriesRepository{q: q})
	})
}

func (r *QueriesRepository) Get(ctx context.Context, userID int64) (db.GetUserResumeRow, error) {
	return r.q.GetUserResume(ctx, userID)
}

func (r *QueriesRepository) Set(ctx context.Context, userID int64, key string) error {
	return r.q.SetUserResume(ctx, db.SetUserResumeParams{
		ID:              userID,
		ResumeObjectKey: pgtype.Text{String: key, Valid: true},
	})
}

func (r *QueriesRepository) Clear(ctx context.Context, userID int64) error {
	return r.q.ClearUserResume(ctx, userID)
}

func (r *QueriesRepository) SetStructured(ctx context.Context, w StructuredWrite) (bool, error) {
	rows, err := r.q.SetUserResumeStructured(ctx, db.SetUserResumeStructuredParams{
		ID:                         w.UserID,
		ResumeStructured:           w.Blob,
		ResumeStructuredModel:      pgtype.Text{String: w.Model, Valid: w.Model != ""},
		ResumeStructuredUploadedAt: pgtype.Timestamptz{Time: w.UploadedAt, Valid: true},
		ResumeCountries:            w.Countries,
		ResumeRegions:              w.Regions,
		ResumeCities:               w.Cities,
	})
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (r *QueriesRepository) GetStructured(ctx context.Context, userID int64) (db.GetUserResumeStructuredRow, error) {
	return r.q.GetUserResumeStructured(ctx, userID)
}

func (r *QueriesRepository) GetGeography(ctx context.Context, userID int64) (db.GetUserResumeGeographyRow, error) {
	return r.q.GetUserResumeGeography(ctx, userID)
}

func (r *QueriesRepository) GetCandidateContacts(ctx context.Context, userID int64) ([]byte, error) {
	return r.q.GetUserCandidateContacts(ctx, userID)
}

func (r *QueriesRepository) SetCandidateContacts(ctx context.Context, userID int64, blob []byte) error {
	return r.q.SetUserCandidateContacts(ctx, db.SetUserCandidateContactsParams{
		ID:                userID,
		CandidateContacts: blob,
	})
}

func (r *QueriesRepository) SetExtractFailed(ctx context.Context, userID int64, detail string, uploadedAt time.Time) error {
	return r.q.SetUserResumeExtractFailed(ctx, db.SetUserResumeExtractFailedParams{
		ID:                  userID,
		ResumeExtractDetail: pgtype.Text{String: detail, Valid: detail != ""},
		ResumeExtractFor:    pgtype.Timestamptz{Time: uploadedAt, Valid: true},
	})
}

func (r *QueriesRepository) SetExtractPending(ctx context.Context, userID int64, uploadedAt time.Time) error {
	return r.q.SetUserResumeExtractPending(ctx, db.SetUserResumeExtractPendingParams{
		ID:               userID,
		ResumeExtractFor: pgtype.Timestamptz{Time: uploadedAt, Valid: true},
	})
}
