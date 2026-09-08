package mentorship

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
	"github.com/strelov1/freehire/internal/platform/pgerr"
)

// Compile-time proof that QueriesRepository satisfies Repository.
var _ Repository = (*QueriesRepository)(nil)

// The constraint names the adapter translates. They are matched by NAME rather than by
// "it was a unique violation", because mentors carries two — one account may hold one
// profile, and one profile owns one public address — and telling a mentor "that address
// is taken" when they in fact already have a profile sends them to fix the wrong thing.
const (
	constraintOneProfilePerUser = "mentors_user_id_key"
	constraintProfileSlug       = "mentors_slug_key"
)

// QueriesRepository is the production Repository over sqlc-generated *db.Queries.
//
// Almost every write here is a single statement, so the guards live in the SQL and are
// translated into this package's sentinels rather than wrapped in a transaction. The one
// exception is replacing the recurring week — a delete plus a set of inserts that is one
// edit from the mentor's point of view — which is why the pool is held alongside the
// queries.
type QueriesRepository struct {
	q    *db.Queries
	pool *pgxpool.Pool
}

// NewQueriesRepository constructs a QueriesRepository.
func NewQueriesRepository(q *db.Queries, pool *pgxpool.Pool) *QueriesRepository {
	return &QueriesRepository{q: q, pool: pool}
}

// CreateProfile inserts a profile, translating each constraint the database can refuse it
// with into the sentinel that tells the mentor what to change.
func (r *QueriesRepository) CreateProfile(ctx context.Context, in ProfileInput) (Profile, error) {
	row, err := r.q.CreateMentorProfile(ctx, db.CreateMentorProfileParams{
		UserID:             in.UserID,
		CompanySlug:        in.CompanySlug,
		Slug:               in.Slug,
		DisplayName:        in.DisplayName,
		Headline:           in.Headline,
		Bio:                in.Bio,
		Topics:             in.Topics,
		Languages:          in.Languages,
		Timezone:           in.Timezone,
		SessionDurationMin: minutesOf(in.Session.Duration),
		BufferBeforeMin:    minutesOf(in.Session.BufferBefore),
		BufferAfterMin:     minutesOf(in.Session.BufferAfter),
		MinNoticeMin:       minutesOf(in.Session.MinimumNotice),
		HorizonDays:        daysOf(in.Session.Horizon),
		MeetingUrl:         in.MeetingURL,
	})
	if err != nil {
		if name, ok := pgerr.UniqueViolationConstraint(err); ok {
			switch name {
			case constraintOneProfilePerUser:
				return Profile{}, ErrAlreadyAMentor
			case constraintProfileSlug:
				return Profile{}, ErrSlugTaken
			}
		}
		// The only foreign key exercised on insert is company_slug — user_id is the
		// authenticated caller and decided_by is not set here.
		if pgerr.IsForeignKeyViolation(err) {
			return Profile{}, ErrCompanyNotFound
		}
		return Profile{}, err
	}
	return profileFromRow(row), nil
}

// ProfileByUser is the owner's own profile, whatever its status.
func (r *QueriesRepository) ProfileByUser(ctx context.Context, userID int64) (Profile, bool, error) {
	row, err := r.q.GetMentorByUserID(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, false, nil
	}
	if err != nil {
		return Profile{}, false, err
	}
	return profileFromRow(row), true, nil
}

// ProfileByID is the profile behind an id the caller already holds.
func (r *QueriesRepository) ProfileByID(ctx context.Context, id int64) (Profile, bool, error) {
	row, err := r.q.GetMentorByID(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, false, nil
	}
	if err != nil {
		return Profile{}, false, err
	}
	return profileFromRow(row), true, nil
}

// PublishedProfileBySlug is the public read. The publication predicate is in the query,
// so an unpublished profile is simply absent here rather than filtered out afterwards.
func (r *QueriesRepository) PublishedProfileBySlug(ctx context.Context, slug string) (Profile, bool, error) {
	row, err := r.q.GetPublishedMentorBySlug(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, false, nil
	}
	if err != nil {
		return Profile{}, false, err
	}
	profile := profileFromRow(row.Mentor)
	profile.CompanyName = pgconv.TextString(row.CompanyName)
	profile.RatingCount = row.RatingCount
	profile.RatingAvg = numericFloat(row.RatingAvg)
	return profile, true, nil
}

// UpdateProfile applies an edit. The no-row case is the owner guard failing, which is
// reported as "not found" rather than "not yours" — an endpoint must not confirm that
// somebody else's profile exists.
func (r *QueriesRepository) UpdateProfile(ctx context.Context, in ProfileInput) (Profile, error) {
	row, err := r.q.UpdateMentorProfile(ctx, db.UpdateMentorProfileParams{
		UserID:             in.UserID,
		DisplayName:        in.DisplayName,
		Headline:           in.Headline,
		Bio:                in.Bio,
		Topics:             in.Topics,
		Languages:          in.Languages,
		Timezone:           in.Timezone,
		SessionDurationMin: minutesOf(in.Session.Duration),
		BufferBeforeMin:    minutesOf(in.Session.BufferBefore),
		BufferAfterMin:     minutesOf(in.Session.BufferAfter),
		MinNoticeMin:       minutesOf(in.Session.MinimumNotice),
		HorizonDays:        daysOf(in.Session.Horizon),
		MeetingUrl:         in.MeetingURL,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, ErrProfileNotFound
	}
	if err != nil {
		return Profile{}, err
	}
	return profileFromRow(row), nil
}

// SetPaused flips the mentor's own switch.
func (r *QueriesRepository) SetPaused(ctx context.Context, userID int64, paused bool) (Profile, error) {
	row, err := r.q.SetMentorPaused(ctx, db.SetMentorPausedParams{
		UserID: userID, Paused: paused,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, ErrProfileNotFound
	}
	if err != nil {
		return Profile{}, err
	}
	return profileFromRow(row), nil
}

// DecideProfile applies a moderator decision. A no-row update means the profile is absent
// or already decided — the status guard — and both are ErrProfileNotPending.
func (r *QueriesRepository) DecideProfile(ctx context.Context, id, moderatorID int64, status string) (Profile, error) {
	row, err := r.q.DecideMentorProfile(ctx, db.DecideMentorProfileParams{
		ID: id, Status: status, DecidedBy: pgtype.Int8{Int64: moderatorID, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Profile{}, ErrProfileNotPending
	}
	if err != nil {
		return Profile{}, err
	}
	return profileFromRow(row), nil
}

// ListPendingProfiles is the moderation queue with its corroborating evidence.
func (r *QueriesRepository) ListPendingProfiles(ctx context.Context) ([]PendingProfile, error) {
	rows, err := r.q.ListPendingMentorProfiles(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]PendingProfile, 0, len(rows))
	for _, row := range rows {
		profile := profileFromRow(row.Mentor)
		profile.CompanyName = pgconv.TextString(row.CompanyName)
		out = append(out, PendingProfile{
			Profile:                  profile,
			HasApprovedReferralOffer: row.HasApprovedReferralOffer,
		})
	}
	return out, nil
}

// ListPublishedProfiles is the public directory. An empty filter field becomes a NULL
// parameter, which the query reads as "unfiltered".
func (r *QueriesRepository) ListPublishedProfiles(ctx context.Context, f DirectoryFilter) ([]Profile, error) {
	rows, err := r.q.ListPublishedMentors(ctx, db.ListPublishedMentorsParams{
		CompanySlug: optionalText(f.CompanySlug),
		Topic:       optionalText(f.Topic),
		Language:    optionalText(f.Language),
		RowLimit:    f.Limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Profile, 0, len(rows))
	for _, row := range rows {
		profile := profileFromRow(row.Mentor)
		profile.CompanyName = pgconv.TextString(row.CompanyName)
		profile.RatingCount = row.RatingCount
		profile.RatingAvg = numericFloat(row.RatingAvg)
		out = append(out, profile)
	}
	return out, nil
}

// CancelFutureBookings ends every confirmed session still ahead, returning them so their
// seekers can be told. It is called BEFORE the profile is deleted: the ON DELETE CASCADE
// would otherwise take these rows and nobody could be notified.
func (r *QueriesRepository) CancelFutureBookings(ctx context.Context, mentorID, cancelledBy int64, reason string) ([]Booking, error) {
	rows, err := r.q.CancelFutureBookingsForMentor(ctx, db.CancelFutureBookingsForMentorParams{
		MentorID: mentorID, CancelledBy: cancelledBy, CancelReason: reason,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Booking, 0, len(rows))
	for _, row := range rows {
		booking := bookingFromRow(row.MentorBooking)
		// The seeker's address comes back with the row: every one of these bookings needs
		// a notification, and fetching addresses one at a time afterwards would be a
		// query per cancelled session.
		booking.SeekerEmail = row.SeekerEmail
		out = append(out, booking)
	}
	return out, nil
}

// WithdrawProfile marks the profile withdrawn. It does NOT delete the row: bookings and
// reviews reference it ON DELETE CASCADE, so deleting would erase the history this
// feature promises to keep. Zero rows means no profile of that caller's, or one already
// withdrawn.
func (r *QueriesRepository) WithdrawProfile(ctx context.Context, userID int64) error {
	rows, err := r.q.WithdrawMentorProfile(ctx, userID)
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrProfileNotFound
	}
	return nil
}

func profileFromRow(row db.Mentor) Profile {
	return Profile{
		ID:          row.ID,
		UserID:      row.UserID,
		CompanySlug: row.CompanySlug,
		Slug:        row.Slug,
		DisplayName: row.DisplayName,
		Headline:    row.Headline,
		Bio:         row.Bio,
		Topics:      row.Topics,
		Languages:   row.Languages,
		Timezone:    row.Timezone,
		Session: SessionParams{
			Duration:      time.Duration(row.SessionDurationMin) * time.Minute,
			BufferBefore:  time.Duration(row.BufferBeforeMin) * time.Minute,
			BufferAfter:   time.Duration(row.BufferAfterMin) * time.Minute,
			MinimumNotice: time.Duration(row.MinNoticeMin) * time.Minute,
			Horizon:       time.Duration(row.HorizonDays) * 24 * time.Hour,
		},
		MeetingURL: row.MeetingUrl,
		Status:     row.Status,
		Paused:     row.Paused,
		DecidedBy:  row.DecidedBy.Int64,
		CreatedAt:  row.CreatedAt.Time,
	}
}

func bookingFromRow(row db.MentorBooking) Booking {
	return Booking{
		ID:             uuid.UUID(row.ID.Bytes),
		MentorID:       row.MentorID,
		SeekerUserID:   row.SeekerUserID,
		StartsAt:       row.StartsAt.Time,
		EndsAt:         row.EndsAt.Time,
		Status:         row.Status,
		JobID:          row.JobID.Int64,
		Note:           row.Note,
		SeekerTimezone: row.SeekerTimezone,
		MeetingURL:     row.MeetingUrl,
	}
}

// minutesOf converts a duration to the whole minutes the schema stores. SessionParams
// .Validate has already refused anything finer, so this cannot silently round.
func minutesOf(d time.Duration) int32 { return int32(d / time.Minute) }

// daysOf converts the booking horizon to the whole days the schema stores, rounding UP so
// a horizon shorter than a day is still a day rather than none.
func daysOf(d time.Duration) int32 {
	days := int32(d / (24 * time.Hour))
	if d%(24*time.Hour) != 0 {
		days++
	}
	return days
}

// optionalText turns an empty filter field into the NULL the query reads as "unfiltered".
func optionalText(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

// numericFloat reads an aggregate average, treating an absent one as zero — the query
// already COALESCEs, so this is the belt to that pair of braces.
func numericFloat(n pgtype.Numeric) float64 {
	if !n.Valid {
		return 0
	}
	f, err := n.Float64Value()
	if err != nil || !f.Valid {
		return 0
	}
	return f.Float64
}
