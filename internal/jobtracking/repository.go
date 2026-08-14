package jobtracking

import (
	"context"
	"errors"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobview"
	"github.com/strelov1/freehire/internal/pgconv"
	"github.com/strelov1/freehire/internal/userjob"
)

// Compile-time proof that QueriesRepository satisfies Repository.
var _ Repository = (*QueriesRepository)(nil)

// QueriesRepository adapts *db.Queries to the Repository interface. The pool
// drives the per-apply transaction (see MarkApplied).
type QueriesRepository struct {
	q    *db.Queries
	pool *pgxpool.Pool
}

// NewQueriesRepository wraps q as a Repository.
func NewQueriesRepository(q *db.Queries, pool *pgxpool.Pool) *QueriesRepository {
	return &QueriesRepository{q: q, pool: pool}
}

// JobIDBySlug returns the internal job id for the given public slug, or
// ErrJobNotFound when no job matches.
func (r *QueriesRepository) JobIDBySlug(ctx context.Context, slug string) (int64, error) {
	id, err := r.q.GetJobIDBySlug(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrJobNotFound
	}
	if err != nil {
		return 0, err
	}
	return id, nil
}

// RecordView records (or refreshes) a user's view of a job.
func (r *QueriesRepository) RecordView(ctx context.Context, userID, jobID int64) (Interaction, error) {
	row, err := r.q.RecordJobView(ctx, db.RecordJobViewParams{UserID: userID, JobID: jobID})
	if err != nil {
		return Interaction{}, err
	}
	// The CTE that bumps view_count makes sqlc emit a bespoke row type with the
	// same shape as db.UserJob; convert so the interaction mapping stays shared.
	return toInteraction(assembledRow(row)), nil
}

// MarkApplied marks a job as applied for a user. The write runs in a
// transaction that locks the job row first (LockJobForApply), so concurrent
// applies serialize and applied_count cannot double-bump — the same pattern
// the vote path uses for its counters.
func (r *QueriesRepository) MarkApplied(ctx context.Context, userID, jobID int64, source string) (Interaction, error) {
	return r.markApplied(ctx, userID, jobID, pgtype.Timestamptz{}, source)
}

// MarkAppliedAt records an application dated by `at` rather than by now() — the
// path for an application reconstructed from employer mail. It runs the same
// locked transaction as MarkApplied, so the applied_count guarantee is the same
// one, not a second implementation of it.
func (r *QueriesRepository) MarkAppliedAt(ctx context.Context, userID, jobID int64, at time.Time, source string) (Interaction, error) {
	return r.markApplied(ctx, userID, jobID, pgtype.Timestamptz{Time: at, Valid: true}, source)
}

// MarkAppliedOn records an application on a day the candidate stated, overwriting a date
// already held.
//
// It runs MarkJobApplied first and re-dates second, in one transaction, because the two
// statements answer different halves of the request: the first creates the application when
// there is none — carrying the applied_count bump and the ledger insert under the single
// predicate that decides them together — and the second imposes the stated date on the
// application either way. Neither alone is the whole operation, and a caller left to run them
// in sequence would eventually run only the first.
//
// RedateApplication returns no row when the application carries no date, which the tracker
// allows: a stage may be set on a job never applied to. MarkJobApplied has just set one here,
// so the miss cannot happen on this path, and treating it as an error rather than silence keeps
// it that way.
func (r *QueriesRepository) MarkAppliedOn(ctx context.Context, userID, jobID int64, at time.Time, source string) (Interaction, error) {
	stated := pgtype.Timestamptz{Time: at, Valid: true}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Interaction{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := r.q.WithTx(tx)
	if err := qtx.LockJobForApply(ctx, jobID); err != nil {
		return Interaction{}, err
	}
	if _, err := qtx.MarkJobApplied(ctx, db.MarkJobAppliedParams{
		UserID: userID, JobID: jobID, At: stated, EventSource: source,
	}); err != nil {
		return Interaction{}, err
	}
	row, err := qtx.RedateApplication(ctx, db.RedateApplicationParams{
		UserID: userID, JobID: jobID, At: stated,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// The statement re-dates only an application that carries a date, and MarkJobApplied has
		// just set one — so this cannot happen on this path today. It is mapped rather than left
		// to surface as a 500 because "cannot happen" is an argument about the code above, not a
		// guarantee, and the honest answer to a missing application is that it is missing.
		return Interaction{}, ErrApplicationNotFound
	}
	if err != nil {
		return Interaction{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Interaction{}, err
	}
	return toInteraction(assembledRow(row)), nil
}

// markApplied is the shared apply transaction. An invalid `at` means "stamp
// now()", the ordinary apply.
//
// The `applied` ledger event is written by MarkJobApplied itself, under the same
// predicate as the applied_count bump, so it lands inside this transaction and
// cannot diverge from the counter.
func (r *QueriesRepository) markApplied(ctx context.Context, userID, jobID int64, at pgtype.Timestamptz, source string) (Interaction, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Interaction{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := r.q.WithTx(tx)
	if err := qtx.LockJobForApply(ctx, jobID); err != nil {
		return Interaction{}, err
	}
	row, err := qtx.MarkJobApplied(ctx, db.MarkJobAppliedParams{
		UserID: userID, JobID: jobID, At: at, EventSource: source,
	})
	if err != nil {
		return Interaction{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Interaction{}, err
	}
	// See RecordView: the applied_count CTE yields a bespoke row of db.UserJob shape.
	return toInteraction(assembledRow(row)), nil
}

// SaveJob saves (bookmarks) a job for a user.
func (r *QueriesRepository) SaveJob(ctx context.Context, userID, jobID int64) (Interaction, error) {
	row, err := r.q.SaveJob(ctx, db.SaveJobParams{UserID: userID, JobID: jobID})
	if err != nil {
		return Interaction{}, err
	}
	return toInteraction(assembledRow(row)), nil
}

// UnsaveJob clears the saved mark. Returns ErrNoInteraction when no row exists.
func (r *QueriesRepository) UnsaveJob(ctx context.Context, userID, jobID int64) (Interaction, error) {
	row, err := r.q.UnsaveJob(ctx, db.UnsaveJobParams{UserID: userID, JobID: jobID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Interaction{}, ErrNoInteraction
	}
	if err != nil {
		return Interaction{}, err
	}
	return toInteraction(assembledRow(row)), nil
}

// DismissJob marks a job dismissed (swiped away) for a user.
func (r *QueriesRepository) DismissJob(ctx context.Context, userID, jobID int64) (Interaction, error) {
	row, err := r.q.DismissJob(ctx, db.DismissJobParams{UserID: userID, JobID: jobID})
	if err != nil {
		return Interaction{}, err
	}
	return toInteraction(assembledRow(row)), nil
}

// UndismissJob clears the dismissed mark. Returns ErrNoInteraction when no row exists.
func (r *QueriesRepository) UndismissJob(ctx context.Context, userID, jobID int64) (Interaction, error) {
	row, err := r.q.UndismissJob(ctx, db.UndismissJobParams{UserID: userID, JobID: jobID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Interaction{}, ErrNoInteraction
	}
	if err != nil {
		return Interaction{}, err
	}
	return toInteraction(assembledRow(row)), nil
}

// TrackJob upserts stage and/or notes for the interaction. A nil pointer means
// "leave unchanged".
func (r *QueriesRepository) TrackJob(
	ctx context.Context,
	userID, jobID int64,
	stage, notes *string,
	source string,
) (Interaction, error) {
	row, err := r.q.TrackJob(ctx, db.TrackJobParams{
		UserID:      userID,
		JobID:       jobID,
		Stage:       textFromPtr(stage),
		Notes:       textFromPtr(notes),
		EventSource: source,
	})
	if err != nil {
		return Interaction{}, err
	}
	// See RecordView: the stage-event CTE yields a bespoke row of db.UserJob shape.
	return toInteraction(assembledRow(row)), nil
}

// ClearJobProgress drops stage and applied_at for a user's interaction with a job.
func (r *QueriesRepository) ClearJobProgress(ctx context.Context, userID, jobID int64) (Interaction, error) {
	row, err := r.q.ClearJobProgress(ctx, db.ClearJobProgressParams{UserID: userID, JobID: jobID})
	if err != nil {
		return Interaction{}, err
	}
	return toInteraction(assembledRow(row)), nil
}

// UntrackJob removes a job from the board by clearing all pipeline marks except viewed_at.
func (r *QueriesRepository) UntrackJob(ctx context.Context, userID, jobID int64) (Interaction, error) {
	row, err := r.q.UntrackJob(ctx, db.UntrackJobParams{UserID: userID, JobID: jobID})
	if err != nil {
		return Interaction{}, err
	}
	return toInteraction(assembledRow(row)), nil
}

// TrackApplication sets stage and/or notes on an application named by its own id.
func (r *QueriesRepository) TrackApplication(
	ctx context.Context,
	userID, appID int64,
	stage, notes *string,
	source string,
) (Interaction, error) {
	row, err := r.q.TrackApplicationByID(ctx, db.TrackApplicationByIDParams{
		ID:          appID,
		UserID:      userID,
		Stage:       textFromPtr(stage),
		Notes:       textFromPtr(notes),
		EventSource: source,
	})
	if err != nil {
		return Interaction{}, applicationRowErr(err)
	}
	return applicationInteraction(row.JobID, row.AppliedAt, row.Stage, row.Notes), nil
}

// ClearApplicationProgress drops the application's stage and applied_at, which is
// what takes it off the board, and keeps the record.
func (r *QueriesRepository) ClearApplicationProgress(ctx context.Context, userID, appID int64) (Interaction, error) {
	row, err := r.q.ClearApplicationProgressByID(ctx, db.ClearApplicationProgressByIDParams{ID: appID, UserID: userID})
	if err != nil {
		return Interaction{}, applicationRowErr(err)
	}
	return applicationInteraction(row.JobID, row.AppliedAt, row.Stage, row.Notes), nil
}

// UntrackApplication removes the application outright.
func (r *QueriesRepository) UntrackApplication(ctx context.Context, userID, appID int64) (Interaction, error) {
	row, err := r.q.UntrackApplicationByID(ctx, db.UntrackApplicationByIDParams{ID: appID, UserID: userID})
	if err != nil {
		return Interaction{}, applicationRowErr(err)
	}
	return applicationInteraction(row.JobID, row.AppliedAt, row.Stage, row.Notes), nil
}

// applicationRowErr maps "no row came back" to the not-found sentinel. Every one of
// the three statements is scoped by `user_id`, so a row belonging to somebody else
// matches nothing and arrives here indistinguishable from one that never existed —
// which is the answer the caller is owed in both cases.
func applicationRowErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrApplicationNotFound
	}
	return err
}

// applicationInteraction builds the interaction an application-addressed write
// returns. The marks that live on user_jobs — viewed, saved, dismissed — are absent
// rather than zeroed-and-implied: an application whose posting is gone has no
// user_jobs row at all, and reporting `saved_at: null` for it would be a claim we
// cannot make. The board writes optimistically and does not read this back.
func applicationInteraction(jobID pgtype.Int8, appliedAt pgtype.Timestamptz, stage, notes pgtype.Text) Interaction {
	return Interaction{
		JobID:     jobID.Int64, // 0 once the posting is gone
		AppliedAt: pgconv.TimePtr(appliedAt),
		Stage:     textPtr(stage),
		Notes:     textPtr(notes),
	}
}

// ListInteractions returns the caller's interactions joined with the jobs in the
// canonical jobview shape, narrowed by the already-validated filter.
func (r *QueriesRepository) ListInteractions(
	ctx context.Context,
	userID int64,
	filter Filter,
	limit, offset int32,
) ([]TrackedJob, error) {
	rows, err := r.q.ListUserJobs(ctx, db.ListUserJobsParams{
		UserID: userID,
		Filter: string(filter),
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		return nil, err
	}
	items := make([]TrackedJob, 0, len(rows))
	for _, row := range rows {
		card := jobview.NewCard(jobview.CardInput{
			PublicSlug:     row.PublicSlug,
			Title:          row.Title,
			Company:        row.Company,
			ClosedAt:       pgconv.TimePtr(row.ClosedAt),
			WorkMode:       row.WorkMode,
			Seniority:      row.Seniority,
			EmploymentType: row.EmploymentType,
			DictCountries:  row.Countries,
			DictRegions:    row.Regions,
			LLMCountries:   row.LlmCountries,
			LLMRegions:     row.LlmRegions,
			Skills:         row.Skills,
			Collections:    row.Collections,
			PostedAt:       pgconv.TimePtr(row.PostedAt),
			CreatedAt:      pgconv.TimePtr(row.CreatedAt),
			Blurb:          row.Blurb,
		})
		items = append(items, TrackedJob{
			ID:          row.PublicSlug,
			CompanySlug: row.CompanySlug,
			RoleTitle:   row.Title,
			Job:         &card,
			Interaction: Interaction{
				JobID:     row.ID,
				ViewedAt:  pgconv.TimePtr(row.ViewedAt),
				SavedAt:   pgconv.TimePtr(row.SavedAt),
				AppliedAt: pgconv.TimePtr(row.AppliedAt),
				Stage:     textPtr(row.Stage),
				Notes:     textPtr(row.Notes),
			},
			EmailCount:           int(row.EmailCount),
			LastActivityAt:       pgconv.TimePtr(row.LastActivityAt),
			HasPendingSuggestion: row.HasPendingSuggestion,
			FollowedUpAt:         pgconv.TimePtr(row.FollowedUpAt),
			CVOpenedAt:           pgconv.TimePtr(row.CvOpenedAt),
		})
	}

	// Applications the catalogue no longer holds a posting for. They cannot come from
	// the query above — it is driven by user_jobs rows, which cmd/prune cascades away —
	// so they are read separately and merged here. Only the board and applied views owe
	// them: a pruned application was never a view or a bookmark.
	if filter == FilterBoard || filter == FilterApplied || filter == FilterAll {
		orphans, err := r.q.ListOrphanedApplications(ctx, db.ListOrphanedApplicationsParams{
			UserID: userID, Limit: limit, Offset: offset,
		})
		if err != nil {
			return nil, err
		}
		for _, o := range orphans {
			items = append(items, TrackedJob{
				ID:          "a" + strconv.FormatInt(o.ID, 10),
				CompanySlug: o.CompanySlug,
				RoleTitle:   o.RoleTitle,
				Interaction: Interaction{
					AppliedAt: pgconv.TimePtr(o.AppliedAt),
					Stage:     textPtr(o.Stage),
					Notes:     textPtr(o.Notes),
				},
				EmailCount:     int(o.EmailCount),
				LastActivityAt: pgconv.TimePtr(o.LastActivityAt),
				FollowedUpAt:   pgconv.TimePtr(o.FollowedUpAt),
			})
		}
	}
	return items, nil
}

// CountInteractions returns the per-filter counts for the caller in one pass.
//
// CountUserJobs is driven entirely FROM user_jobs, which cmd/prune cascades away when it
// prunes a posting — so it never counts an orphaned application (see
// ListOrphanedApplications), even though ListInteractions merges those into the board,
// applied, and all views. CountOrphanedApplications closes that gap: added into "all",
// "applied", and "board" (the same three filters ListInteractions merges orphans into),
// left out of "viewed"/"saved"/"dismissed" (an orphaned application was never a view or a
// bookmark, and dismissal lives only on user_jobs).
func (r *QueriesRepository) CountInteractions(ctx context.Context, userID int64) (Counts, error) {
	row, err := r.q.CountUserJobs(ctx, userID)
	if err != nil {
		return Counts{}, err
	}
	orphaned, err := r.q.CountOrphanedApplications(ctx, userID)
	if err != nil {
		return Counts{}, err
	}
	return Counts{
		All:       row.All + orphaned,
		Viewed:    row.Viewed,
		Saved:     row.Saved,
		Applied:   row.Applied + orphaned,
		Board:     row.Board + orphaned,
		Dismissed: row.Dismissed,
	}, nil
}

// ViewedSlugs returns every public job slug the caller has interacted with.
func (r *QueriesRepository) ViewedSlugs(ctx context.Context, userID int64) ([]string, error) {
	return r.q.ListViewedJobSlugs(ctx, userID)
}

// SavedSlugs returns every public job slug the caller has saved (bookmarked).
func (r *QueriesRepository) SavedSlugs(ctx context.Context, userID int64) ([]string, error) {
	return r.q.ListSavedJobSlugs(ctx, userID)
}

// DismissedSlugs returns every public job slug the caller has hidden (dismissed).
func (r *QueriesRepository) DismissedSlugs(ctx context.Context, userID int64) ([]string, error) {
	return r.q.ListDismissedJobSlugs(ctx, userID)
}

// ExcludedJobIDs returns up to limit job ids the caller has already interacted
// with (viewed, saved, applied, or dismissed).
func (r *QueriesRepository) ExcludedJobIDs(ctx context.Context, userID int64, limit int32) ([]int64, error) {
	return r.q.ExcludedJobIDs(ctx, db.ExcludedJobIDsParams{UserID: userID, Limit: limit})
}

// PipelineCounts returns the caller's per-stage application counts. A NULL stage
// (an applied row with no explicit stage) becomes an empty Stage, which
// userjob.Aggregate folds into the no_answer bucket.
func (r *QueriesRepository) PipelineCounts(ctx context.Context, userID int64) ([]userjob.StageCount, error) {
	rows, err := r.q.CountMyJobsByStage(ctx, userID)
	if err != nil {
		return nil, err
	}
	counts := make([]userjob.StageCount, 0, len(rows))
	for _, row := range rows {
		counts = append(counts, userjob.StageCount{Stage: row.Stage.String, Count: row.Count})
	}
	return counts, nil
}

// assembledRow is the shape every write query in user_jobs.sql returns: the marks from
// user_jobs and the process from applications, in user_jobs' historical column order.
// sqlc emits a distinct Row type per query with identical layout, so one conversion
// target keeps the mapping shared — db.UserJob stopped serving that once the application
// columns were dropped from the table.
type assembledRow struct {
	UserID       int64
	JobID        int64
	ViewedAt     pgtype.Timestamptz
	AppliedAt    pgtype.Timestamptz
	SavedAt      pgtype.Timestamptz
	Stage        pgtype.Text
	Notes        pgtype.Text
	DismissedAt  pgtype.Timestamptz
	Vote         pgtype.Int2
	FollowedUpAt pgtype.Timestamptz
}

// toInteraction converts an assembled row to the domain Interaction type.
func toInteraction(r assembledRow) Interaction {
	return Interaction{
		JobID:       r.JobID,
		ViewedAt:    pgconv.TimePtr(r.ViewedAt),
		AppliedAt:   pgconv.TimePtr(r.AppliedAt),
		SavedAt:     pgconv.TimePtr(r.SavedAt),
		DismissedAt: pgconv.TimePtr(r.DismissedAt),
		Stage:       textPtr(r.Stage),
		Notes:       textPtr(r.Notes),
	}
}

// textPtr converts a pgtype.Text to *string (nil when NULL).
func textPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	v := t.String
	return &v
}

// textFromPtr converts a *string to pgtype.Text. A nil pointer produces an
// invalid (NULL) Text so the SQL COALESCE leaves the column unchanged.
func textFromPtr(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *s, Valid: true}
}
