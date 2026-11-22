package jobtracking

import (
	"context"
	"errors"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/strelov1/freehire/internal/application/autoapply"
	"github.com/strelov1/freehire/internal/application/userjob"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
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
func (r *QueriesRepository) JobIDBySlug(ctx context.Context, userID int64, slug string) (int64, error) {
	job, err := r.q.GetJobBySlug(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrJobNotFound
	}
	if err != nil {
		return 0, err
	}
	return trackedJobID(job, userID)
}

func trackedJobID(job db.Job, userID int64) (int64, error) {
	if job.IsPrivate && (!job.CreatedBy.Valid || job.CreatedBy.Int64 != userID) {
		return 0, ErrJobNotFound
	}
	return job.ID, nil
}

// RecordView records (or refreshes) a user's view of a job.
func (r *QueriesRepository) RecordView(ctx context.Context, userID, jobID int64) (Interaction, error) {
	row, err := r.q.RecordJobView(ctx, db.RecordJobViewParams{UserID: userID, JobID: jobID})
	if err != nil {
		return Interaction{}, err
	}
	// The view_count CTE makes sqlc emit its own row type; convert to share the mapping.
	return toInteraction(assembledRow(row)), nil
}

// MarkApplied marks a job as applied for a user. The transaction locks the job row
// first (LockJobForApply), so concurrent applies serialize and applied_count cannot
// double-bump.
func (r *QueriesRepository) MarkApplied(ctx context.Context, userID, jobID int64, source string) (Interaction, error) {
	return r.markApplied(ctx, userID, jobID, pgtype.Timestamptz{}, source)
}

// MarkAppliedAt records an application dated by `at` rather than now(), for one
// reconstructed from employer mail. It shares MarkApplied's locked transaction.
func (r *QueriesRepository) MarkAppliedAt(ctx context.Context, userID, jobID int64, at time.Time, source string) (Interaction, error) {
	return r.markApplied(ctx, userID, jobID, pgconv.Instant(at), source)
}

// MarkAppliedOn records an application on a day the candidate stated, overwriting a date
// already held.
//
// MarkJobApplied runs first and re-dates second, in one transaction: the first creates the
// application when there is none (applied_count bump and ledger insert under one predicate),
// the second imposes the stated date either way.
//
// RedateApplication returns no row for an application with no date. MarkJobApplied has just
// set one, so the miss should not happen here, and it is treated as an error, not silence.
func (r *QueriesRepository) MarkAppliedOn(ctx context.Context, userID, jobID int64, at time.Time, source string) (Interaction, error) {
	stated := pgconv.Instant(at)

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return Interaction{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	qtx := r.q.WithTx(tx)
	if err := qtx.LockJobForApply(ctx, jobID); err != nil {
		return Interaction{}, err
	}
	applied, err := qtx.MarkJobApplied(ctx, db.MarkJobAppliedParams{
		UserID: userID, JobID: jobID, At: stated, EventSource: source,
	})
	if err != nil {
		return Interaction{}, err
	}
	row, err := qtx.RedateApplication(ctx, db.RedateApplicationParams{
		UserID: userID, JobID: jobID, At: stated,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		// Should not happen (MarkJobApplied just set a date); mapped so it is not a 500.
		return Interaction{}, ErrApplicationNotFound
	}
	if err != nil {
		return Interaction{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Interaction{}, err
	}
	out := toInteraction(assembledRow(row))
	if applied.Created {
		out.ApplicationID = applied.ApplicationID
	}
	return out, nil
}

// markApplied is the shared apply transaction. An invalid `at` means "stamp now()".
//
// MarkJobApplied writes the `applied` ledger event under the same predicate as the
// applied_count bump, so the two cannot diverge.
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
	// The extra application_id column keeps this row from converting to assembledRow wholesale.
	out := toInteraction(assembledRow{
		UserID: row.UserID, JobID: row.JobID, ViewedAt: row.ViewedAt, AppliedAt: row.AppliedAt,
		SavedAt: row.SavedAt, Stage: row.Stage, Notes: row.Notes, DismissedAt: row.DismissedAt,
		Vote: row.Vote, FollowedUpAt: row.FollowedUpAt,
	})
	if row.Created {
		out.ApplicationID = row.ApplicationID
	}
	return out, nil
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
		Stage:       pgconv.TextFromPtr(stage),
		Notes:       pgconv.TextFromPtr(notes),
		EventSource: source,
	})
	if err != nil {
		return Interaction{}, err
	}
	// See RecordView.
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
		Stage:       pgconv.TextFromPtr(stage),
		Notes:       pgconv.TextFromPtr(notes),
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

// applicationRowErr maps "no row came back" to the not-found sentinel. The statements
// are scoped by `user_id`, so somebody else's row is indistinguishable from a missing one.
func applicationRowErr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrApplicationNotFound
	}
	return err
}

// applicationInteraction builds the interaction an application-addressed write
// returns. The user_jobs marks (viewed, saved, dismissed) are left absent: an
// application whose posting is gone has no user_jobs row to report on.
func applicationInteraction(jobID pgtype.Int8, appliedAt pgtype.Timestamptz, stage, notes pgtype.Text) Interaction {
	return Interaction{
		JobID:     jobID.Int64, // 0 once the posting is gone
		AppliedAt: pgconv.TimePtr(appliedAt),
		Stage:     pgconv.TextPtr(stage),
		Notes:     pgconv.TextPtr(notes),
	}
}

// ListInteractions returns the caller's interactions joined with the jobs in the
// canonical jobview shape, narrowed by the already-validated filter.
//
// (limit, offset) addresses ONE window over the merged list, posting-backed rows
// first and orphaned applications after, and the two queries divide that window.
// Passing the same (limit, offset) to both would return 2*limit rows and skip rows
// for a client paging by response length, and loop on an empty page past the total.
func (r *QueriesRepository) ListInteractions(
	ctx context.Context,
	userID int64,
	filter Filter,
	detail CardDetail,
	limit, offset int32,
) ([]TrackedJob, error) {
	compact := detail == CardCompact
	rows, err := r.q.ListUserJobs(ctx, db.ListUserJobsParams{
		UserID:  userID,
		Filter:  string(filter),
		Compact: compact,
		Limit:   limit,
		Offset:  offset,
	})
	if err != nil {
		return nil, err
	}
	items := make([]TrackedJob, 0, len(rows))
	for _, row := range rows {
		skills, collections := row.Skills, row.Collections
		if compact {
			skills, collections = nil, nil
		}
		card := jobview.NewCard(jobview.CardInput{
			PublicSlug:          row.PublicSlug,
			Title:               row.Title,
			Company:             row.Company,
			ClosedAt:            pgconv.TimePtr(row.ClosedAt),
			ApplicationDeadline: pgconv.DatePtr(row.ApplicationDeadline),
			Location:            row.Location,
			WorkMode:            row.WorkMode,
			Seniority:           row.Seniority,
			EmploymentType:      row.EmploymentType,
			DictCountries:       row.Countries,
			DictRegions:         row.Regions,
			LLMCountries:        row.LlmCountries,
			LLMRegions:          row.LlmRegions,
			Skills:              skills,
			Collections:         collections,
			PostedAt:            pgconv.TimePtr(row.PostedAt),
			CreatedAt:           pgconv.TimePtr(row.CreatedAt),
			Blurb:               row.Blurb,
		})
		items = append(items, TrackedJob{
			ID:             row.PublicSlug,
			AddedToBoardAt: pgconv.TimePtr(row.AddedToBoardAt),
			CompanySlug:    row.CompanySlug,
			RoleTitle:      row.Title,
			Job:            &card,
			Interaction: Interaction{
				JobID:     row.ID,
				ViewedAt:  pgconv.TimePtr(row.ViewedAt),
				SavedAt:   pgconv.TimePtr(row.SavedAt),
				AppliedAt: pgconv.TimePtr(row.AppliedAt),
				Stage:     pgconv.TextPtr(row.Stage),
				Notes:     pgconv.TextPtr(row.Notes),
			},
			EmailCount:           int(row.EmailCount),
			LastActivityAt:       pgconv.TimePtr(row.LastActivityAt),
			HasPendingSuggestion: row.HasPendingSuggestion,
			FollowedUpAt:         pgconv.TimePtr(row.FollowedUpAt),
			CVOpenedAt:           pgconv.TimePtr(row.CvOpenedAt),
			AutoApplyStatus: autoApplyStatusFor(row.AutoApplyID.Valid, row.AutoApplyTailoredCvID,
				row.AutoApplyHasPreview, row.AutoApplyReviewDecision, row.AutoApplyBlockedAt,
				row.AutoApplyFailedAt, row.AutoApplyPreviewFailedAt, row.AutoApplyTailorFailedAt),
		})
	}

	// Applications whose posting is gone: the query above is driven by user_jobs rows, which
	// cascade away with the posting. Only the board, applied and all views include them.
	if filter == FilterBoard || filter == FilterApplied || filter == FilterAll {
		orphanLimit := limit - int32(len(items))
		if orphanLimit > 0 {
			orphanOffset, err := r.orphanOffset(ctx, userID, filter, offset, len(items))
			if err != nil {
				return nil, err
			}
			orphans, err := r.q.ListOrphanedApplications(ctx, db.ListOrphanedApplicationsParams{
				UserID: userID, Limit: orphanLimit, Offset: orphanOffset,
			})
			if err != nil {
				return nil, err
			}
			items = append(items, orphanedTracked(orphans)...)
		}
	}
	return items, nil
}

// orphanOffset says where this page's remainder starts in the orphaned applications:
// the requested offset less the filter's total posting-backed rows, floored at zero.
//
// A page that returned any posting-backed row already reached their end, so orphans start
// at zero. Only an EMPTY page at a non-zero offset has to ask, using the same aggregate as
// CountInteractions so the list and count boundaries agree.
func (r *QueriesRepository) orphanOffset(
	ctx context.Context,
	userID int64,
	filter Filter,
	offset int32,
	fetched int,
) (int32, error) {
	if offset == 0 || fetched > 0 {
		return 0, nil
	}
	row, err := r.q.CountUserJobs(ctx, userID)
	if err != nil {
		return 0, err
	}
	return max(0, offset-int32(postingBackedCount(row, filter))), nil
}

// postingBackedCount picks the CountUserJobs column matching the ListUserJobs filter.
// "viewed" and "dismissed" never reach here (their views carry no orphans), and an
// unrecognised filter reads as zero rather than as a guess.
func postingBackedCount(row db.CountUserJobsRow, filter Filter) int64 {
	switch filter {
	case FilterAll:
		return row.All
	case FilterApplied:
		return row.Applied
	case FilterBoard:
		return row.Board
	default:
		return 0
	}
}

// orphanedTracked projects orphaned application rows onto the shared card shape, with
// no Job: the employer and role title live on the application record.
func orphanedTracked(orphans []db.ListOrphanedApplicationsRow) []TrackedJob {
	items := make([]TrackedJob, 0, len(orphans))
	for _, o := range orphans {
		items = append(items, TrackedJob{
			ID:             "a" + strconv.FormatInt(o.ID, 10),
			AddedToBoardAt: pgconv.TimePtr(o.AddedToBoardAt),
			CompanySlug:    o.CompanySlug,
			RoleTitle:      o.RoleTitle,
			Interaction: Interaction{
				AppliedAt: pgconv.TimePtr(o.AppliedAt),
				Stage:     pgconv.TextPtr(o.Stage),
				Notes:     pgconv.TextPtr(o.Notes),
			},
			EmailCount:     int(o.EmailCount),
			LastActivityAt: pgconv.TimePtr(o.LastActivityAt),
			FollowedUpAt:   pgconv.TimePtr(o.FollowedUpAt),
		})
	}
	return items
}

// CountInteractions returns the per-filter counts for the caller in one pass.
//
// CountUserJobs reads user_jobs, which cascades away with a posting, so it misses orphaned
// applications. CountOrphanedApplications is added into "all", "applied" and "board" (the
// filters ListInteractions merges orphans into) and left out of the rest.
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

// FeedExcludedJobIDs returns up to limit job ids the caller saved, dismissed or applied
// to before the instant.
func (r *QueriesRepository) FeedExcludedJobIDs(ctx context.Context, userID int64, before time.Time, limit int32) ([]int64, error) {
	return r.q.FeedExcludedJobIDs(ctx, db.FeedExcludedJobIDsParams{UserID: userID, Before: pgconv.Instant(before), RowLimit: limit})
}

// AppliedCompanySlugs returns the company slugs of the employers the caller has
// applied to, from at most maxApplications of their newest applications.
func (r *QueriesRepository) AppliedCompanySlugs(ctx context.Context, userID int64, maxApplications int32) ([]string, error) {
	return r.q.ListAppliedCompanySlugs(ctx, db.ListAppliedCompanySlugsParams{UserID: userID, MaxApplications: maxApplications})
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
		counts = append(counts, userjob.StageCount{
			Stage: row.Stage.String, Count: row.Count,
			ReachedInterview: row.ReachedInterview, ReachedOffer: row.ReachedOffer,
		})
	}
	return counts, nil
}

// ReplyRateCounts returns the caller's own observable/answered counts alongside the
// global figure minus the caller's contribution (userjob.ExcludeCallerFromGlobal). The
// personal side is live (application_events); the global side reads the per-company
// rollup. The queries are independent, so they run concurrently.
func (r *QueriesRepository) ReplyRateCounts(ctx context.Context, userID int64) (you, global userjob.ReplyRateSide, err error) {
	var (
		youRow            db.GetUserResponseRateRow
		globalRow         db.GetGlobalCompanyResponseRow
		youErr, globalErr error
	)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		youRow, youErr = r.q.GetUserResponseRate(ctx, userID)
	}()
	go func() {
		defer wg.Done()
		globalRow, globalErr = r.q.GetGlobalCompanyResponse(ctx)
	}()
	wg.Wait()
	if youErr != nil {
		return userjob.ReplyRateSide{}, userjob.ReplyRateSide{}, youErr
	}
	if globalErr != nil {
		return userjob.ReplyRateSide{}, userjob.ReplyRateSide{}, globalErr
	}

	you = userjob.ReplyRateSide{Applications: int64(youRow.Applications), Answered: int64(youRow.Answered)}
	globalTotal := userjob.ReplyRateSide{Applications: int64(globalRow.Applications), Answered: int64(globalRow.Answered)}
	global = userjob.ExcludeCallerFromGlobal(you, globalTotal)
	return you, global, nil
}

// assembledRow is the shape every write query in user_jobs.sql and
// user_job_applications.sql returns: the marks from user_jobs and the process from
// applications. sqlc emits a distinct Row type per query with identical layout, so one
// conversion target keeps the mapping shared.
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
		Stage:       pgconv.TextPtr(r.Stage),
		Notes:       pgconv.TextPtr(r.Notes),
	}
}

// autoApplyStatusFor derives a board row's own auto-apply status from ListUserJobs's raw
// columns, or nil when hasAttempt is false (no auto_apply_queue row for this (user, job)).
// Mirrors autoApplyReviewInfoForJob's derivation (internal/api/handler), status only.
func autoApplyStatusFor(hasAttempt bool, tailoredCVID *uuid.UUID, hasPreview bool, reviewDecision pgtype.Text, blockedAt, failedAt, previewFailedAt, tailorFailedAt pgtype.Timestamptz) *autoapply.Status {
	if !hasAttempt {
		return nil
	}
	status := autoapply.DeriveStatus(autoapply.DerivedFrom{
		HasTailoredCV:      tailoredCVID != nil,
		HasResolvedPreview: hasPreview,
		ReviewDecision:     reviewDecision.String,
		Blocked:            blockedAt.Valid,
		// Either budget exhausting counts as failed, as in autoApplyReviewInfoForJob.
		Failed:       failedAt.Valid || previewFailedAt.Valid,
		TailorFailed: tailorFailedAt.Valid,
	})
	return &status
}
