package handler

import (
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

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

func (h *userModerationHandlers) register(api fiber.Router, mw middleware) {
	api.Get("/moderation/users", mw.cookie, mw.moderator, h.Find)
	api.Post("/moderation/users/:id/ban", mw.cookie, mw.moderator, h.Ban)
	api.Post("/moderation/users/:id/unban", mw.cookie, mw.moderator, h.Unban)
}

type moderatedUserResponse struct {
	ID        int64      `json:"id"`
	Email     string     `json:"email"`
	Username  *string    `json:"username"`
	Role      string     `json:"role"`
	Banned    bool       `json:"banned"`
	BannedAt  *time.Time `json:"banned_at"`
	BanReason *string    `json:"ban_reason"`
}

func toModeratedUser(r db.FindUserForModerationRow) moderatedUserResponse {
	out := moderatedUserResponse{ID: r.ID, Email: r.Email, Role: r.Role, Banned: r.BannedAt.Valid}
	if r.Username.Valid {
		out.Username = &r.Username.String
	}
	if r.BannedAt.Valid {
		out.BannedAt = &r.BannedAt.Time
		out.BanReason = &r.BanReason.String
	}
	return out
}

// lookup finds one account by numeric id, email or username.
func (h *userModerationHandlers) lookup(c *fiber.Ctx, query string) (db.FindUserForModerationRow, error) {
	id, _ := strconv.ParseInt(query, 10, 64) // not a number → 0, which matches no account
	row, err := h.queries.FindUserForModeration(c.Context(), db.FindUserForModerationParams{UserID: id, Query: query})
	if errors.Is(err, pgx.ErrNoRows) {
		return row, fiber.NewError(fiber.StatusNotFound, "user not found")
	}
	return row, err
}

// Find looks an account up by ?q= (id, exact email or exact username).
func (h *userModerationHandlers) Find(c *fiber.Ctx) error {
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		return fiber.NewError(fiber.StatusBadRequest, "q is required")
	}
	row, err := h.lookup(c, q)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": toModeratedUser(row)})
}

type banRequest struct {
	Reason string `json:"reason"`
}

// Ban suspends an account: every session, key and grant stops working at once and sign-in
// is refused until Unban. A moderator cannot ban an admin or themselves.
func (h *userModerationHandlers) Ban(c *fiber.Ctx) error {
	actor, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "invalid user id")
	}
	var in banRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	reason := strings.TrimSpace(in.Reason)
	if reason == "" || utf8.RuneCountInString(reason) > maxBanReason {
		return fiber.NewError(fiber.StatusBadRequest, "a reason of 1 to 500 characters is required")
	}
	if id == actor {
		return fiber.NewError(fiber.StatusForbidden, "you cannot ban yourself")
	}
	target, err := h.lookup(c, strconv.FormatInt(id, 10))
	if err != nil {
		return err
	}
	if target.Role == "admin" {
		return fiber.NewError(fiber.StatusForbidden, "an admin cannot be banned")
	}
	// BanUser repeats the admin and self rules in SQL, so a role change between the read
	// above and this write still cannot get around them.
	_, err = h.queries.BanUser(c.Context(), db.BanUserParams{
		ID: id, Reason: pgtype.Text{String: reason, Valid: true}, ActorID: pgtype.Int8{Int64: actor, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return fiber.NewError(fiber.StatusConflict, "user is already banned")
	}
	if err != nil {
		return err
	}
	return h.respond(c, id)
}

// Unban lifts a ban. Earlier sessions stay dead; the person signs in again.
func (h *userModerationHandlers) Unban(c *fiber.Ctx) error {
	id, err := strconv.ParseInt(c.Params("id"), 10, 64)
	if err != nil || id <= 0 {
		return fiber.NewError(fiber.StatusBadRequest, "invalid user id")
	}
	n, err := h.queries.UnbanUser(c.Context(), id)
	if err != nil {
		return err
	}
	if n == 0 {
		return fiber.NewError(fiber.StatusNotFound, "user not found or not banned")
	}
	return h.respond(c, id)
}

func (h *userModerationHandlers) respond(c *fiber.Ctx, id int64) error {
	row, err := h.lookup(c, strconv.FormatInt(id, 10))
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": toModeratedUser(row)})
}
