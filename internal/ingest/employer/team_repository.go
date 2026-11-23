package employer

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
	"github.com/strelov1/freehire/internal/platform/pgerr"
)

var _ TeamRepository = (*QueriesRepository)(nil)

// Members reads everyone who acts for a company, the claimant first.
func (r *QueriesRepository) Members(ctx context.Context, companySlug string) ([]Member, error) {
	rows, err := r.q.ListCompanyTeam(ctx, companySlug)
	if err != nil {
		return nil, err
	}
	out := make([]Member, len(rows))
	for i, row := range rows {
		out[i] = Member{UserID: row.UserID, WorkEmail: row.WorkEmail, JoinedAt: row.JoinedAt.Time, Holder: row.Holder, Role: row.Role}
	}
	return out, nil
}

// Invitations reads a company's invitations that can still be accepted.
func (r *QueriesRepository) Invitations(ctx context.Context, companySlug string) ([]Invitation, error) {
	rows, err := r.q.ListCompanyInvitations(ctx, companySlug)
	if err != nil {
		return nil, err
	}
	out := make([]Invitation, len(rows))
	for i, row := range rows {
		out[i] = Invitation{ID: row.ID, Email: row.Email, CreatedAt: row.CreatedAt.Time, ExpiresAt: row.ExpiresAt.Time}
	}
	return out, nil
}

// Invite stores an invitation, or replaces the one already sent to the same address, under
// the company's lock: the inviter must still act for the company (a removal that committed
// first is seen), and the open-invitation count cannot be passed by two invitations at once.
// ErrNotFound when the inviter no longer acts for it.
func (r *QueriesRepository) Invite(ctx context.Context, inv NewInvitation, maxLive int64) (Invitation, error) {
	var out Invitation
	err := r.asCompanyActor(ctx, inv.CompanySlug, inv.InvitedBy, true, func(q *db.Queries) error {
		row, err := q.UpsertCompanyInvitation(ctx, db.UpsertCompanyInvitationParams{
			CompanySlug: inv.CompanySlug,
			Email:       inv.Email,
			TokenHash:   inv.TokenHash,
			InvitedBy:   inv.InvitedBy,
			ExpiresAt:   pgconv.Instant(inv.ExpiresAt),
			MaxLive:     maxLive,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrTooManyInvitations
		}
		if err != nil {
			return err
		}
		out = Invitation{ID: row.ID, Email: row.Email, CreatedAt: row.CreatedAt.Time, ExpiresAt: row.ExpiresAt.Time}
		return nil
	})
	return out, err
}

// WithdrawToken deletes the invitation only while it still carries tokenHash; a resend that
// replaced the token meanwhile is left alone.
func (r *QueriesRepository) WithdrawToken(ctx context.Context, companySlug string, tokenHash []byte) error {
	return r.q.DeleteCompanyInvitationByToken(ctx, db.DeleteCompanyInvitationByTokenParams{CompanySlug: companySlug, TokenHash: tokenHash})
}

// asCompany runs fn in a transaction holding the company's team lock (LockEmployerCompany).
func (r *QueriesRepository) asCompany(ctx context.Context, companySlug string, fn func(q *db.Queries) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := r.q.WithTx(tx)
	if err := q.LockEmployerCompany(ctx, companySlug); err != nil {
		return err
	}
	if err := fn(q); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// asCompanyActor is asCompany for a change made by actorID: under the lock it first checks
// that actorID still acts for the company (ErrNotFound otherwise), so a removal that
// committed first is seen.
func (r *QueriesRepository) asCompanyActor(ctx context.Context, companySlug string, actorID int64, requireManager bool, fn func(q *db.Queries) error) error {
	return r.asCompany(ctx, companySlug, func(q *db.Queries) error {
		acts, err := q.CanActForCompany(ctx, db.CanActForCompanyParams{CompanySlug: companySlug, UserID: actorID, RequireManager: requireManager})
		if err != nil {
			return err
		}
		if !acts {
			return ErrNotFound
		}
		return fn(q)
	})
}

// Withdraw deletes one of a company's invitations, under the company's lock and only while
// actorID still acts for it.
func (r *QueriesRepository) Withdraw(ctx context.Context, companySlug string, actorID, id int64) error {
	return r.asCompanyActor(ctx, companySlug, actorID, true, func(q *db.Queries) error {
		n, err := q.DeleteCompanyInvitation(ctx, db.DeleteCompanyInvitationParams{ID: id, CompanySlug: companySlug, ActorID: actorID})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrInvitationNotFound
		}
		return nil
	})
}

// Preview reads the live invitation a token names.
func (r *QueriesRepository) Preview(ctx context.Context, tokenHash []byte) (InvitationPreview, string, error) {
	row, err := r.q.GetCompanyInvitationByToken(ctx, tokenHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return InvitationPreview{}, "", ErrInvitationInvalid
	}
	if err != nil {
		return InvitationPreview{}, "", err
	}
	return InvitationPreview{CompanyName: row.CompanyName, Email: row.Email, Role: row.Role}, row.Status, nil
}

// Accept consumes the invitation and adds the member, in one statement, under the same
// per-user actor lock a claim takes and then the company's team lock (always in that order),
// so it cannot interleave with a removal that withdraws the invitation.
func (r *QueriesRepository) Accept(ctx context.Context, userID int64, tokenHash []byte) error {
	err := r.asActor(ctx, userID, func(q *db.Queries) error {
		inv, err := q.GetCompanyInvitationByToken(ctx, tokenHash)
		if err != nil {
			return err
		}
		if inv.Status != StatusActive {
			return ErrInvitationInvalid
		}
		if err := q.LockEmployerCompany(ctx, inv.CompanySlug); err != nil {
			return err
		}
		if inv.Role == RoleRecruiter {
			user, err := q.GetUserByID(ctx, userID)
			if err != nil {
				return err
			}
			if !user.EmailVerified || !strings.EqualFold(user.Email, inv.Email) {
				return ErrInvitationEmail
			}
		}
		_, err = q.AcceptCompanyInvitation(ctx, db.AcceptCompanyInvitationParams{UserID: userID, TokenHash: tokenHash})
		return err
	})
	if pgerr.IsUniqueViolation(err) {
		return ErrAlreadyHasAccount
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvitationInvalid
	}
	return err
}

// Remove takes a member out of a company's team, handing their vacancies to the claimant and
// withdrawing their invitations, under the company's team lock and only while actorID still
// acts for the company — a removal of actorID that committed first leaves their queued
// requests changing nothing.
func (r *QueriesRepository) Remove(ctx context.Context, companySlug string, actorID, memberID int64) error {
	return r.asCompanyActor(ctx, companySlug, actorID, actorID != memberID, func(q *db.Queries) error {
		// The row lock first, in its own statement (see LockCompanyMemberForRemoval), so the
		// hand-over sees a vacancy the member published while the removal waited.
		_, err := q.LockCompanyMemberForRemoval(ctx, db.LockCompanyMemberForRemovalParams{MemberID: memberID, CompanySlug: companySlug})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrMemberNotFound
		}
		if err != nil {
			return err
		}
		n, err := q.RemoveCompanyMember(ctx, db.RemoveCompanyMemberParams{MemberID: memberID, CompanySlug: companySlug, ActorID: actorID})
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrMemberNotFound
		}
		return nil
	})
}

// Activity reads company audit events visible to the current actor.
func (r *QueriesRepository) Activity(ctx context.Context, companySlug string, actorID, beforeID int64) ([]Activity, error) {
	rows, err := r.q.ListEmployerActivity(ctx, db.ListEmployerActivityParams{
		CompanySlug: companySlug,
		ActorID:     actorID,
		BeforeID:    pgtype.Int8{Int64: beforeID, Valid: beforeID > 0},
	})
	if err != nil {
		return nil, err
	}
	out := make([]Activity, 0, len(rows))
	for _, row := range rows {
		a := Activity{ID: row.ID, Action: row.Action, ActorEmail: row.ActorEmail, PreviousStage: row.PreviousStage.String,
			Stage: row.Stage.String, SubjectEmail: row.SubjectEmail.String, OccurredAt: row.OccurredAt.Time}
		if row.ApplicationID.Valid {
			a.ApplicationID = &row.ApplicationID.Int64
		}
		out = append(out, a)
	}
	return out, nil
}
