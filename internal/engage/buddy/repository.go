package buddy

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgerr"
)

var _ Repository = (*QueriesRepository)(nil)

// QueriesRepository is the production Repository over sqlc's *db.Queries.
type QueriesRepository struct {
	q *db.Queries
}

// NewQueriesRepository builds a QueriesRepository.
func NewQueriesRepository(q *db.Queries) *QueriesRepository { return &QueriesRepository{q: q} }

func categoryText(k Keys) pgtype.Text {
	return pgtype.Text{String: k.Category, Valid: k.Category != ""}
}

func (r *QueriesRepository) Join(ctx context.Context, userID int64, handle string, keys Keys) (Member, error) {
	row, err := r.q.JoinBuddyMember(ctx, db.JoinBuddyMemberParams{
		UserID: userID, Handle: handle, Category: categoryText(keys),
		Countries: keys.Countries, Regions: keys.Regions,
	})
	if err != nil {
		if pgerr.IsUniqueViolation(err) {
			return Member{}, ErrHandleTaken
		}
		return Member{}, err
	}
	return memberFromRow(row), nil
}

func (r *QueriesRepository) Leave(ctx context.Context, userID int64) (bool, error) {
	_, err := r.q.LeaveBuddyMember(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

func (r *QueriesRepository) Member(ctx context.Context, userID int64) (Member, bool, error) {
	row, err := r.q.GetBuddyMemberByUser(ctx, userID)
	return memberResult(row, err)
}

func (r *QueriesRepository) MemberByHandle(ctx context.Context, handle string) (Member, bool, error) {
	row, err := r.q.GetBuddyMemberByHandle(ctx, handle)
	return memberResult(row, err)
}

func (r *QueriesRepository) SetKeys(ctx context.Context, userID int64, keys Keys) error {
	return r.q.UpdateBuddyMemberRankingKeys(ctx, db.UpdateBuddyMemberRankingKeysParams{
		UserID: userID, Category: categoryText(keys),
		Countries: keys.Countries, Regions: keys.Regions,
	})
}

// Inputs reads the CV, its derived geography and the profile. A missing or unreadable
// piece is simply absent: the ranking keys fall back and the card is empty.
func (r *QueriesRepository) Inputs(ctx context.Context, userID int64) (Inputs, error) {
	var in Inputs
	raw, err := r.q.GetUserResumeStructuredOnly(ctx, userID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Inputs{}, err
	}
	if len(raw) > 0 {
		var s resumeextract.Structured
		if json.Unmarshal(raw, &s) == nil {
			in.CV = s
		}
	}
	geo, err := r.q.GetUserResumeGeography(ctx, userID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Inputs{}, err
	}
	// Same freshness rule as resume.Store.Geography: a geography derived from a CV that
	// has since been replaced reads as unknown, so the base-country fallback engages.
	// (That Store needs a blob store this package has no use for.)
	if geo.ResumeStructuredUploadedAt.Valid && geo.ResumeUploadedAt.Valid &&
		geo.ResumeStructuredUploadedAt.Time.Equal(geo.ResumeUploadedAt.Time) {
		in.Countries, in.Regions = geo.ResumeCountries, geo.ResumeRegions
	}
	prof, err := r.q.GetUserProfile(ctx, userID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return Inputs{}, err
	}
	in.Specializations = prof.Specializations
	if len(prof.LocationPreferences) > 0 {
		var loc userprofile.LocationPreferences
		if json.Unmarshal(prof.LocationPreferences, &loc) == nil {
			in.BaseCountry = loc.Base.Country
		}
	}
	return in, nil
}

func (r *QueriesRepository) Similar(ctx context.Context, userID int64, limit, offset int) ([]SimilarRow, error) {
	rows, err := r.q.ListSimilarBuddies(ctx, db.ListSimilarBuddiesParams{
		ViewerID: userID, WCategory: weightCategory, WCountries: weightCountries,
		WRegions: weightRegions, WCompany: weightCompany, CompanyCap: SharedCompanyCap,
		PageLimit: int32(limit), PageOffset: int32(offset),
	})
	if err != nil {
		return nil, err
	}
	out := make([]SimilarRow, len(rows))
	for i, row := range rows {
		out[i] = SimilarRow{Handle: row.Handle, Countries: row.Countries, Score: int(row.Score)}
		if len(row.ResumeStructured) > 0 {
			// An unreadable CV leaves an empty card summary, as in Inputs.
			var s resumeextract.Structured
			if json.Unmarshal(row.ResumeStructured, &s) == nil {
				out[i].CV = s
			}
		}
		if len(row.SharedNames) != len(row.SharedSlugs) {
			return nil, errors.New("buddy: shared company slugs and names differ in length")
		}
		for j, slug := range row.SharedSlugs {
			out[i].Shared = append(out[i].Shared, Company{Slug: slug, Name: row.SharedNames[j]})
		}
	}
	return out, nil
}

func memberResult(row db.BuddyMember, err error) (Member, bool, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return Member{}, false, nil
	}
	if err != nil {
		return Member{}, false, err
	}
	return memberFromRow(row), true, nil
}

func memberFromRow(row db.BuddyMember) Member {
	return Member{
		UserID: row.UserID, Handle: row.Handle, Category: row.Category.String,
		Countries: row.Countries, Regions: row.Regions,
		JoinedAt: row.JoinedAt.Time, Active: !row.LeftAt.Valid,
	}
}
