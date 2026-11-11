package handler

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/platform/db"
)

// maxBanReason bounds the note a moderator leaves; it is shown to no one but moderators.
const maxBanReason = 500

// userModerationHandlers is the manual account ban: find an account, ban it with a
// reason, lift the ban. Cookie-only (a leaked API key must not suspend people) and
// moderator-only. Nothing bans automatically; this is the only writer of users.banned_at.
type userModerationHandlers struct{ queries *db.Queries }

func newUserModerationHandlers(queries *db.Queries) *userModerationHandlers {
	return &userModerationHandlers{queries: queries}
}

func (h *userModerationHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	api.Get("/moderation/users", mw.cookie, mw.moderator, contract)
	api.Post("/moderation/users/:id/ban", mw.cookie, mw.moderator, contract)
	api.Post("/moderation/users/:id/unban", mw.cookie, mw.moderator, contract)
}

func toModeratedUser(r db.FindUserForModerationRow) apigen.ModeratedUser {
	out := apigen.ModeratedUser{ID: r.ID, Email: r.Email, Role: r.Role, Banned: r.BannedAt.Valid}
	if r.Username.Valid {
		out.Username = apigen.NewNilString(r.Username.String)
	} else {
		out.Username.SetToNull()
	}
	if r.BannedAt.Valid {
		out.BannedAt = apigen.NewNilDateTime(r.BannedAt.Time)
		out.BanReason = apigen.NewNilString(r.BanReason.String)
	} else {
		out.BannedAt.SetToNull()
		out.BanReason.SetToNull()
	}
	return out
}

// lookup finds one account by numeric id, email or username.
func (h *userModerationHandlers) lookup(ctx context.Context, query string) (db.FindUserForModerationRow, error) {
	id, _ := strconv.ParseInt(query, 10, 64) // not a number → 0, which matches no account
	row, err := h.queries.FindUserForModeration(ctx, db.FindUserForModerationParams{UserID: id, Query: query})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, fiber.NewError(fiber.StatusNotFound, "user not found")
	}
	return row, err
}

// FindModeratedUser looks an account up by ?q= (id, exact email or exact username).
func (h *userModerationHandlers) FindModeratedUser(ctx context.Context, params apigen.FindModeratedUserParams) (*apigen.FindModeratedUserOK, error) {
	q := strings.TrimSpace(params.Q.Value)
	if q == "" {
		return nil, fiber.NewError(fiber.StatusBadRequest, "q is required")
	}
	row, err := h.lookup(ctx, q)
	if err != nil {
		return nil, err
	}
	return &apigen.FindModeratedUserOK{Data: toModeratedUser(row)}, nil
}

// BanUser suspends an account: every session, key and grant stops working at once and sign-in
// is refused until Unban. A moderator cannot ban an admin or themselves.
func (h *userModerationHandlers) BanUser(ctx context.Context, req *apigen.BanRequest, params apigen.BanUserParams) (*apigen.BanUserOK, error) {
	actor, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := strconv.ParseInt(params.ID, 10, 64)
	if err != nil || id <= 0 {
		return nil, fiber.NewError(fiber.StatusBadRequest, "invalid user id")
	}
	reason := strings.TrimSpace(req.Reason.Value)
	if reason == "" || utf8.RuneCountInString(reason) > maxBanReason {
		return nil, fiber.NewError(fiber.StatusBadRequest, "a reason of 1 to 500 characters is required")
	}
	if id == actor {
		return nil, fiber.NewError(fiber.StatusForbidden, "you cannot ban yourself")
	}
	target, err := h.lookup(ctx, strconv.FormatInt(id, 10))
	if err != nil {
		return nil, err
	}
	if target.Role == "admin" {
		return nil, fiber.NewError(fiber.StatusForbidden, "an admin cannot be banned")
	}
	// BanUser repeats the admin and self rules in SQL, so a role change between the read
	// above and this write still cannot get around them.
	_, err = h.queries.BanUser(ctx, db.BanUserParams{
		ID: id, Reason: pgtype.Text{String: reason, Valid: true}, ActorID: pgtype.Int8{Int64: actor, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fiber.NewError(fiber.StatusConflict, "user is already banned")
	}
	if err != nil {
		return nil, err
	}
	user, err := h.reread(ctx, id)
	if err != nil {
		return nil, err
	}
	return &apigen.BanUserOK{Data: user}, nil
}

// UnbanUser lifts a ban. Earlier sessions stay dead; the person signs in again.
func (h *userModerationHandlers) UnbanUser(ctx context.Context, params apigen.UnbanUserParams) (*apigen.UnbanUserOK, error) {
	id, err := strconv.ParseInt(params.ID, 10, 64)
	if err != nil || id <= 0 {
		return nil, fiber.NewError(fiber.StatusBadRequest, "invalid user id")
	}
	n, err := h.queries.UnbanUser(ctx, id)
	if err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, fiber.NewError(fiber.StatusNotFound, "user not found or not banned")
	}
	user, err := h.reread(ctx, id)
	if err != nil {
		return nil, err
	}
	return &apigen.UnbanUserOK{Data: user}, nil
}

// reread is the account as it stands after a ban or an unban, in its wire shape.
func (h *userModerationHandlers) reread(ctx context.Context, id int64) (apigen.ModeratedUser, error) {
	row, err := h.lookup(ctx, strconv.FormatInt(id, 10))
	if err != nil {
		return apigen.ModeratedUser{}, err
	}
	return toModeratedUser(row), nil
}
