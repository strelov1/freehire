package referral

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/pgconv"
	"github.com/strelov1/freehire/internal/pgerr"
)

// Compile-time proof that QueriesRepository satisfies Repository.
var _ Repository = (*QueriesRepository)(nil)

// QueriesRepository is the production Repository backed by sqlc-generated *db.Queries. Every
// write is a single statement, so no transaction wrapper is needed; the unique/no-row guards
// live in the SQL and are mapped to sentinel errors here.
type QueriesRepository struct {
	q *db.Queries
}

// NewQueriesRepository constructs a QueriesRepository.
func NewQueriesRepository(q *db.Queries) *QueriesRepository {
	return &QueriesRepository{q: q}
}

// CreateOffer inserts an offer, mapping the (user, company) unique violation to
// ErrAlreadyOffered.
func (r *QueriesRepository) CreateOffer(ctx context.Context, in OfferInput) (Offer, error) {
	row, err := r.q.CreateReferralOffer(ctx, db.CreateReferralOfferParams{
		UserID: in.UserID, CompanySlug: in.CompanySlug, ProofObjectKey: in.ProofKey,
	})
	if err != nil {
		if pgerr.IsUniqueViolation(err) {
			return Offer{}, ErrAlreadyOffered
		}
		// The only foreign key on insert is company_slug (user_id is the authed caller),
		// so a FK violation means the company slug is not in the catalogue.
		if pgerr.IsForeignKeyViolation(err) {
			return Offer{}, ErrCompanyNotFound
		}
		return Offer{}, err
	}
	return offerFromRow(row), nil
}

// DecideOffer applies a moderator decision, mapping the no-row update (offer absent or
// already decided) to ErrOfferNotPending.
func (r *QueriesRepository) DecideOffer(ctx context.Context, offerID, moderatorID int64, status string) (Offer, error) {
	row, err := r.q.DecideReferralOffer(ctx, db.DecideReferralOfferParams{
		ID: offerID, Status: status, DecidedBy: int8Val(moderatorID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Offer{}, ErrOfferNotPending
	}
	if err != nil {
		return Offer{}, err
	}
	return offerFromRow(row), nil
}

// GetOffer returns one offer by id; ok is false when it does not exist.
func (r *QueriesRepository) GetOffer(ctx context.Context, offerID int64) (Offer, bool, error) {
	row, err := r.q.GetReferralOffer(ctx, offerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Offer{}, false, nil
	}
	if err != nil {
		return Offer{}, false, err
	}
	return offerFromRow(row), true, nil
}

// ListOffersByUser returns a member's offers, newest first.
func (r *QueriesRepository) ListOffersByUser(ctx context.Context, userID int64) ([]Offer, error) {
	rows, err := r.q.ListReferralOffersByUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	return mapSlice(rows, offerFromRow), nil
}

// ListPendingOffers returns the moderator queue, oldest first.
func (r *QueriesRepository) ListPendingOffers(ctx context.Context) ([]Offer, error) {
	rows, err := r.q.ListPendingReferralOffers(ctx)
	if err != nil {
		return nil, err
	}
	return mapSlice(rows, offerFromRow), nil
}

// CompanyHasApprovedReferrer reports whether the company is referral-eligible.
func (r *QueriesRepository) CompanyHasApprovedReferrer(ctx context.Context, companySlug string) (bool, error) {
	return r.q.CompanyHasApprovedReferrer(ctx, companySlug)
}

// ReferrerApprovedForCompany reports whether the member is an approved referrer of the company.
func (r *QueriesRepository) ReferrerApprovedForCompany(ctx context.Context, userID int64, companySlug string) (bool, error) {
	return r.q.ReferrerApprovedForCompany(ctx, db.ReferrerApprovedForCompanyParams{
		UserID: userID, CompanySlug: companySlug,
	})
}

// ApprovedReferrerRecipients returns the company's approved referrers with their email and
// linked Telegram chat (0 when unlinked).
func (r *QueriesRepository) ApprovedReferrerRecipients(ctx context.Context, companySlug string) ([]Recipient, error) {
	rows, err := r.q.ListApprovedReferrerRecipients(ctx, companySlug)
	if err != nil {
		return nil, err
	}
	out := make([]Recipient, len(rows))
	for i, row := range rows {
		out[i] = Recipient{UserID: row.UserID, Email: row.Email, ChatID: row.ChatID.Int64}
	}
	return out, nil
}

// CVBelongsToUser reports whether the builder CV is owned by the user.
func (r *QueriesRepository) CVBelongsToUser(ctx context.Context, cvID, userID int64) (bool, error) {
	return r.q.CVBelongsToUser(ctx, db.CVBelongsToUserParams{CvID: cvID, UserID: userID})
}

// UserHasResume reports whether the user has a stored original résumé.
func (r *QueriesRepository) UserHasResume(ctx context.Context, userID int64) (bool, error) {
	return r.q.UserHasResume(ctx, userID)
}

// CreateRequest inserts a request, mapping the active partial-unique violation to
// ErrAlreadyRequested.
func (r *QueriesRepository) CreateRequest(ctx context.Context, in RequestInput) (Request, error) {
	row, err := r.q.CreateReferralRequest(ctx, db.CreateReferralRequestParams{
		SeekerUserID:    in.SeekerUserID,
		CompanySlug:     in.CompanySlug,
		JobID:           int8Ptr(in.JobID),
		CvKind:          in.CVKind,
		CvID:            int8Ptr(in.CVID),
		ContactTelegram: textOrNull(in.ContactTelegram),
		ContactEmail:    textOrNull(in.ContactEmail),
		Note:            in.Note,
	})
	if err != nil {
		if pgerr.IsUniqueViolation(err) {
			return Request{}, ErrAlreadyRequested
		}
		return Request{}, err
	}
	return requestFromRow(row), nil
}

// CountRequestsSince counts a seeker's requests created at or after the cutoff.
func (r *QueriesRepository) CountRequestsSince(ctx context.Context, seekerID int64, since time.Time) (int64, error) {
	return r.q.CountReferralRequestsSince(ctx, db.CountReferralRequestsSinceParams{
		SeekerUserID: seekerID, Since: pgconv.Timestamptz(&since),
	})
}

// GetRequest returns a request by id; ok is false when it does not exist.
func (r *QueriesRepository) GetRequest(ctx context.Context, id int64) (Request, bool, error) {
	row, err := r.q.GetReferralRequest(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, false, nil
	}
	if err != nil {
		return Request{}, false, err
	}
	return requestFromRow(row), true, nil
}

// ResolveRequest applies a referrer's mark, mapping the no-row update (already resolved) to
// ErrRequestNotOpen.
func (r *QueriesRepository) ResolveRequest(ctx context.Context, id, actorID int64, status string) (Request, error) {
	row, err := r.q.ResolveReferralRequest(ctx, db.ResolveReferralRequestParams{
		ID: id, Status: status, ActedBy: int8Val(actorID),
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Request{}, ErrRequestNotOpen
	}
	if err != nil {
		return Request{}, err
	}
	return requestFromRow(row), nil
}

// ListRequestsBySeeker returns a seeker's requests, newest first.
func (r *QueriesRepository) ListRequestsBySeeker(ctx context.Context, seekerID int64) ([]Request, error) {
	rows, err := r.q.ListReferralRequestsBySeeker(ctx, seekerID)
	if err != nil {
		return nil, err
	}
	return mapSlice(rows, requestFromRow), nil
}

// ListIncomingRequests returns the open requests for the referrer's approved companies.
func (r *QueriesRepository) ListIncomingRequests(ctx context.Context, referrerID int64) ([]Request, error) {
	rows, err := r.q.ListIncomingReferralRequests(ctx, referrerID)
	if err != nil {
		return nil, err
	}
	return mapSlice(rows, requestFromRow), nil
}

func offerFromRow(row db.ReferralOffer) Offer {
	return Offer{
		ID:          row.ID,
		UserID:      row.UserID,
		CompanySlug: row.CompanySlug,
		ProofKey:    row.ProofObjectKey,
		Status:      row.Status,
		DecidedBy:   int64PtrFrom(row.DecidedBy),
		DecidedAt:   pgconv.TimePtr(row.DecidedAt),
		CreatedAt:   pgconv.TimePtr(row.CreatedAt),
	}
}

func requestFromRow(row db.ReferralRequest) Request {
	return Request{
		ID:              row.ID,
		SeekerUserID:    row.SeekerUserID,
		CompanySlug:     row.CompanySlug,
		JobID:           int64PtrFrom(row.JobID),
		CVKind:          row.CvKind,
		CVID:            int64PtrFrom(row.CvID),
		ContactTelegram: row.ContactTelegram.String,
		ContactEmail:    row.ContactEmail.String,
		Note:            row.Note,
		Status:          row.Status,
		ActedBy:         int64PtrFrom(row.ActedBy),
		ActedAt:         pgconv.TimePtr(row.ActedAt),
		CreatedAt:       pgconv.TimePtr(row.CreatedAt),
	}
}

// mapSlice maps every element of in through f.
func mapSlice[T, U any](in []T, f func(T) U) []U {
	out := make([]U, len(in))
	for i, v := range in {
		out[i] = f(v)
	}
	return out
}

// int8Val wraps a required int64 as a non-null pgtype.Int8.
func int8Val(n int64) pgtype.Int8 { return pgtype.Int8{Int64: n, Valid: true} }

// int8Ptr wraps an optional *int64 as a pgtype.Int8, NULL when nil.
func int8Ptr(n *int64) pgtype.Int8 {
	if n == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *n, Valid: true}
}

// int64PtrFrom unwraps a pgtype.Int8 to *int64, nil when NULL.
func int64PtrFrom(n pgtype.Int8) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

// textOrNull wraps a string as a pgtype.Text, storing NULL for an empty string so the
// contact column carries a real value or nothing.
func textOrNull(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}
