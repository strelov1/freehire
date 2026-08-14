package cvedit

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/db"
)

// queriesRepository is the Repository over the generated queries. It owns the transaction
// boundary: everything a commit does happens inside one, against a CV row locked for its
// duration, so the document and the revision describing it are written together or not at
// all.
type queriesRepository struct {
	pool *pgxpool.Pool
	q    *db.Queries
}

// NewRepository adapts the connection pool and the generated queries to the Repository the
// editor writes through.
func NewRepository(pool *pgxpool.Pool, q *db.Queries) Repository {
	return queriesRepository{pool: pool, q: q}
}

func (r queriesRepository) Edit(ctx context.Context, cvID uuid.UUID, userID int64, fn func(context.Context, Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	q := r.q.WithTx(tx)
	// The lock is taken here rather than by the callback, so every path through Edit holds
	// it: two agent turns arriving together are serialised instead of interleaving.
	row, err := q.GetCVForEdit(ctx, db.GetCVForEditParams{ID: cvID, UserID: userID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return cv.ErrNotFound
		}
		return err
	}

	state, err := stateFromRow(row)
	if err != nil {
		return err
	}
	work := &queriesTx{q: q, cvID: cvID, userID: userID, state: state, updatedAt: row.UpdatedAt.Time}
	if err := fn(ctx, work); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r queriesRepository) List(ctx context.Context, cvID uuid.UUID, userID int64, limit int32) ([]Revision, error) {
	rows, err := r.q.ListCVRevisions(ctx, db.ListCVRevisionsParams{CvID: cvID, UserID: userID, Limit: limit})
	if err != nil {
		return nil, err
	}
	return revisionsFromRows(rows)
}

// queriesTx is one CV's locked row for the length of a transaction. It reads the state once
// and keeps it, because the callback needs the same view it will write against.
type queriesTx struct {
	q         *db.Queries
	cvID      uuid.UUID
	userID    int64
	state     State
	updatedAt time.Time
}

func (t *queriesTx) State(context.Context) (State, time.Time, error) {
	return t.state, t.updatedAt, nil
}

func (t *queriesTx) Save(ctx context.Context, state State) (cv.Meta, error) {
	data, err := json.Marshal(state.Document)
	if err != nil {
		return cv.Meta{}, err
	}
	row, err := t.q.UpdateCV(ctx, db.UpdateCVParams{
		ID: t.cvID, UserID: t.userID, Title: state.Title, TemplateID: state.TemplateID, Data: data,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return cv.Meta{}, cv.ErrNotFound
		}
		return cv.Meta{}, err
	}
	// Keep the in-transaction view current, so a second operation in the same commit sees
	// what the first one wrote.
	t.state = state
	return cv.Meta{
		ID: row.ID, Title: row.Title, TemplateID: row.TemplateID,
		CreatedAt: row.CreatedAt.Time, UpdatedAt: row.UpdatedAt.Time,
	}, nil
}

func (t *queriesTx) Newest(ctx context.Context) (Revision, bool, error) {
	row, err := t.q.NewestCVRevision(ctx, db.NewestCVRevisionParams{CvID: t.cvID, UserID: t.userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Revision{}, false, nil
	}
	if err != nil {
		return Revision{}, false, err
	}
	rev, err := revisionFromRow(row)
	return rev, err == nil, err
}

func (t *queriesTx) Revision(ctx context.Context, id uuid.UUID) (Revision, bool, error) {
	row, err := t.q.GetCVRevision(ctx, db.GetCVRevisionParams{ID: id, UserID: t.userID, CvID: t.cvID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Revision{}, false, nil
	}
	if err != nil {
		return Revision{}, false, err
	}
	rev, err := revisionFromRow(row)
	return rev, err == nil, err
}

func (t *queriesTx) Insert(ctx context.Context, rev Revision) (Revision, error) {
	ops, err := json.Marshal(rev.Ops)
	if err != nil {
		return Revision{}, err
	}
	inverse, err := json.Marshal(rev.Inverse)
	if err != nil {
		return Revision{}, err
	}
	row, err := t.q.InsertCVRevision(ctx, db.InsertCVRevisionParams{
		CvID:        t.cvID,
		UserID:      t.userID,
		Actor:       string(rev.Actor),
		Origin:      string(rev.Origin),
		BatchID:     optionalUUID(rev.BatchID),
		Title:       rev.Title,
		Note:        pgtype.Text{String: rev.Note, Valid: rev.Note != ""},
		Ops:         ops,
		Inverse:     inverse,
		BaseVersion: pgtype.Timestamptz{Time: rev.BaseVersion, Valid: true},
		RevertsID:   optionalUUID(rev.RevertsID),
	})
	if err != nil {
		return Revision{}, err
	}
	return revisionFromRow(row)
}

func (t *queriesTx) Amend(ctx context.Context, id uuid.UUID, ops []Op, title, note string) (Revision, error) {
	blob, err := json.Marshal(ops)
	if err != nil {
		return Revision{}, err
	}
	row, err := t.q.AmendCVRevision(ctx, db.AmendCVRevisionParams{
		ID: id, UserID: t.userID, Ops: blob, Title: title,
		Note: pgtype.Text{String: note, Valid: note != ""},
	})
	if err != nil {
		return Revision{}, err
	}
	return revisionFromRow(row)
}

func (t *queriesTx) MarkReverted(ctx context.Context, id uuid.UUID) (bool, error) {
	n, err := t.q.MarkCVRevisionReverted(ctx, db.MarkCVRevisionRevertedParams{ID: id, UserID: t.userID})
	return n > 0, err
}

func (t *queriesTx) InBatch(ctx context.Context, batchID uuid.UUID) ([]Revision, error) {
	rows, err := t.q.ListCVRevisionsInBatch(ctx, db.ListCVRevisionsInBatchParams{
		CvID: t.cvID, UserID: t.userID, BatchID: &batchID,
	})
	if err != nil {
		return nil, err
	}
	return revisionsFromRows(rows)
}

func (t *queriesTx) Feed(ctx context.Context, limit int32) ([]Revision, error) {
	rows, err := t.q.ListCVRevisions(ctx, db.ListCVRevisionsParams{CvID: t.cvID, UserID: t.userID, Limit: limit})
	if err != nil {
		return nil, err
	}
	return revisionsFromRows(rows)
}

func (t *queriesTx) Trim(ctx context.Context, keep int32) error {
	_, err := t.q.TrimCVRevisions(ctx, db.TrimCVRevisionsParams{CvID: t.cvID, Limit: keep})
	return err
}

func stateFromRow(row db.GetCVForEditRow) (State, error) {
	var doc cv.Document
	if len(row.Data) > 0 {
		if err := json.Unmarshal(row.Data, &doc); err != nil {
			return State{}, err
		}
	}
	return State{Title: row.Title, TemplateID: row.TemplateID, Document: doc}, nil
}

func revisionsFromRows(rows []db.CvRevision) ([]Revision, error) {
	out := make([]Revision, 0, len(rows))
	for _, row := range rows {
		rev, err := revisionFromRow(row)
		if err != nil {
			return nil, err
		}
		out = append(out, rev)
	}
	return out, nil
}

func revisionFromRow(row db.CvRevision) (Revision, error) {
	ops, err := decodeOps(row.Ops)
	if err != nil {
		return Revision{}, err
	}
	inverse, err := decodeOps(row.Inverse)
	if err != nil {
		return Revision{}, err
	}
	rev := Revision{
		ID:          row.ID,
		CVID:        row.CvID,
		Actor:       Actor(row.Actor),
		Origin:      Origin(row.Origin),
		Title:       row.Title,
		Note:        row.Note.String,
		Ops:         ops,
		Inverse:     inverse,
		BaseVersion: row.BaseVersion.Time,
		CreatedAt:   row.CreatedAt.Time,
		UpdatedAt:   row.UpdatedAt.Time,
	}
	if row.BatchID != nil {
		rev.BatchID = *row.BatchID
	}
	if row.RevertsID != nil {
		rev.RevertsID = *row.RevertsID
	}
	if row.RevertedAt.Valid {
		at := row.RevertedAt.Time
		rev.RevertedAt = &at
	}
	return rev, nil
}

func decodeOps(blob json.RawMessage) ([]Op, error) {
	if len(blob) == 0 {
		return nil, nil
	}
	var ops []Op
	if err := json.Unmarshal(blob, &ops); err != nil {
		return nil, err
	}
	return ops, nil
}

// optionalUUID renders a zero id as absent, which is what the nullable columns mean: most
// revisions belong to no batch, and only an undo reverts something.
func optionalUUID(id uuid.UUID) *uuid.UUID {
	if id == uuid.Nil {
		return nil
	}
	return &id
}
