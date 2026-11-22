package jobtracking

import (
	"context"
	"time"

	"github.com/strelov1/freehire/internal/application/userjob"
)

// Repository is the narrow persistence contract required by the Service.
type Repository interface {
	// JobIDBySlug returns the internal job id for the given public slug, or
	// ErrJobNotFound when no job matches.
	JobIDBySlug(ctx context.Context, userID int64, slug string) (int64, error)

	RecordView(ctx context.Context, userID, jobID int64) (Interaction, error)

	// MarkApplied records an application. `source` is the appevent source of the
	// recording (who observed it, not who applied), supplied by the caller: the SPA,
	// an API key and the assistant all use this method.
	MarkApplied(ctx context.Context, userID, jobID int64, source string) (Interaction, error)

	// MarkAppliedAt records an application dated by `at` instead of now(), for one
	// reconstructed from employer mail. It keeps an existing applied_at.
	MarkAppliedAt(ctx context.Context, userID, jobID int64, at time.Time, source string) (Interaction, error)

	// MarkAppliedOn records an application on a day the candidate states, and
	// overwrites a date already recorded: mail supplies an upper bound, a person
	// the answer. The `applied` ledger event moves with the column.
	MarkAppliedOn(ctx context.Context, userID, jobID int64, at time.Time, source string) (Interaction, error)

	SaveJob(ctx context.Context, userID, jobID int64) (Interaction, error)

	// UnsaveJob clears the saved mark. It returns ErrNoInteraction when no row
	// exists at all (the Service turns that into a zero-interaction success).
	UnsaveJob(ctx context.Context, userID, jobID int64) (Interaction, error)

	// DismissJob sets the dismissed mark, idempotently, keeping the job out of the
	// swipe deck without affecting the public list/search.
	DismissJob(ctx context.Context, userID, jobID int64) (Interaction, error)

	// UndismissJob clears the dismissed mark. It returns ErrNoInteraction when no
	// row exists at all (the Service turns that into a zero-interaction success).
	UndismissJob(ctx context.Context, userID, jobID int64) (Interaction, error)

	// TrackJob upserts the stage and/or notes for the interaction. A nil pointer
	// means "leave unchanged". `source` is the appevent source of the recording,
	// carried for the same reason MarkApplied carries it.
	TrackJob(ctx context.Context, userID, jobID int64, stage, notes *string, source string) (Interaction, error)

	// ClearJobProgress drops stage and applied_at, keeping saved_at/viewed_at/notes.
	ClearJobProgress(ctx context.Context, userID, jobID int64) (Interaction, error)

	// UntrackJob removes a job from the board entirely: clears saved_at, applied_at,
	// stage, and notes, keeping viewed_at so the job stays in view history.
	UntrackJob(ctx context.Context, userID, jobID int64) (Interaction, error)

	// The three below address an application by its own id, because the posting can
	// disappear and leave the slug-addressed writes above nothing to resolve. Each
	// returns ErrApplicationNotFound when the id names nothing the caller holds.

	// TrackApplication sets stage and/or notes on the application. A nil pointer
	// means "leave unchanged", and `source` is carried as TrackJob carries it.
	TrackApplication(ctx context.Context, userID, appID int64, stage, notes *string, source string) (Interaction, error)

	// ClearApplicationProgress drops stage and applied_at, keeping the record and its
	// notes — the pair is what puts an application on the board.
	ClearApplicationProgress(ctx context.Context, userID, appID int64) (Interaction, error)

	// UntrackApplication removes the application outright, as UntrackJob does for a
	// posting-backed one.
	UntrackApplication(ctx context.Context, userID, appID int64) (Interaction, error)

	// ListInteractions returns the caller's interactions joined with the jobs,
	// narrowed by an already-validated filter, most recently touched first.
	ListInteractions(ctx context.Context, userID int64, filter Filter, detail CardDetail, limit, offset int32) ([]TrackedJob, error)

	// CountInteractions returns the per-filter counts for the caller in one pass.
	CountInteractions(ctx context.Context, userID int64) (Counts, error)

	// PipelineCounts returns the caller's per-stage application counts (saved-only
	// rows excluded; an applied row with no stage carries an empty Stage).
	PipelineCounts(ctx context.Context, userID int64) ([]userjob.StageCount, error)

	// ReplyRateCounts returns the caller's own observable/answered counts and the
	// global figure summed across every company, both under the same definitions —
	// observable requires a connected mailbox, answered requires a non-retracted
	// employer_reply. The sample gate is applied by the caller (userjob.GateReplyRateBenchmark),
	// not here.
	ReplyRateCounts(ctx context.Context, userID int64) (you, global userjob.ReplyRateSide, err error)

	// ViewedSlugs returns every public job slug the caller has interacted with.
	ViewedSlugs(ctx context.Context, userID int64) ([]string, error)

	// SavedSlugs returns every public job slug the caller has saved (bookmarked).
	SavedSlugs(ctx context.Context, userID int64) ([]string, error)

	// DismissedSlugs returns every public job slug the caller has hidden (dismissed).
	DismissedSlugs(ctx context.Context, userID int64) ([]string, error)

	// ExcludedJobIDs returns up to limit job ids the caller has already interacted
	// with (viewed, saved, applied, or dismissed), most-recently-touched first.
	ExcludedJobIDs(ctx context.Context, userID int64, limit int32) ([]int64, error)

	// FeedExcludedJobIDs returns up to limit job ids the caller saved, dismissed or
	// applied to before the instant, newest action first.
	FeedExcludedJobIDs(ctx context.Context, userID int64, before time.Time, limit int32) ([]int64, error)

	// AppliedCompanySlugs returns the company slugs of the employers the caller has
	// applied to, drawn from at most maxApplications of their newest applications.
	AppliedCompanySlugs(ctx context.Context, userID int64, maxApplications int32) ([]string, error)
}
