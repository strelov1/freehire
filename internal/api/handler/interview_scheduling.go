package handler

import (
	"context"
	"errors"
	"slices"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/engage/interviewscheduling"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/identity/auth/oauth"
	"github.com/strelov1/freehire/internal/platform/db"
)

const interviewSchedulingStateCookie = "interview_scheduling_oauth_state"

var errPartialSchedulingGrant = errors.New("google did not grant all scheduling permissions")

type schedulingAccess struct {
	enabled     bool
	pilotUserID int64
	plans       *plan.Store
}

func (h *inboxHandlers) schedulingAvailable(userID int64) bool {
	return h.gmailReady() && h.scheduling.enabled &&
		(h.scheduling.pilotUserID == 0 || h.scheduling.pilotUserID == userID)
}

func (h *inboxHandlers) interviewSchedulingStatus(c *fiber.Ctx) error {
	id, _ := auth.UserID(c)
	c.Set(fiber.HeaderCacheControl, "private, no-store")
	out := fiber.Map{"available": h.schedulingAvailable(id), "enabled": false, "connected": false}
	if out["available"] == true {
		state, err := h.queries.GetInterviewSchedulingSettings(c.UserContext(), db.GetInterviewSchedulingSettingsParams{
			UserID: id, RequiredScopes: interviewscheduling.RequiredScopes(),
		})
		if err != nil {
			return err
		}
		out["enabled"] = state.InterviewSchedulingEnabledAt.Valid
		out["connected"] = state.GrantReady
	}
	return c.JSON(fiber.Map{"data": out})
}

func (h *inboxHandlers) interviewSchedulingConnect(c *fiber.Ctx) error {
	id, _ := auth.UserID(c)
	if !h.schedulingAvailable(id) {
		return fiber.ErrNotFound
	}
	if _, err := h.scheduling.plans.RequirePaid(c.UserContext(), id); err != nil {
		if errors.Is(err, plan.ErrPaymentRequired) {
			return fiber.NewError(fiber.StatusPaymentRequired, "Interview scheduling needs Pro or Ultra")
		}
		return err
	}
	state, err := oauth.NewState()
	if err != nil {
		return err
	}
	oauth.SetStateCookieNamed(c, interviewSchedulingStateCookie, state, h.cookieSecure)
	return c.Redirect(h.gmailConnector.SchedulingAuthCodeURL(state), fiber.StatusFound)
}

func (h *inboxHandlers) interviewSchedulingCallback(c *fiber.Ctx) error {
	id, ok := auth.UserID(c)
	if ok && !h.schedulingAvailable(id) {
		return fiber.ErrNotFound
	}
	var email string
	return h.runOAuthCallback(c, oauthCallbackFlow{
		name: "interview scheduling connect", stateCookie: interviewSchedulingStateCookie,
		errorParam: "scheduling_error", successQuery: "scheduling=connected",
		exchange: func(ctx context.Context, code string) (string, []string, error) {
			token, address, scopes, err := h.gmailConnector.Exchange(ctx, code)
			email = address
			return token, scopes, err
		},
		persist: func(ctx context.Context, userID int64, encrypted string, scopes []string) error {
			for _, scope := range interviewscheduling.RequiredScopes() {
				if !slices.Contains(scopes, scope) {
					return errPartialSchedulingGrant
				}
			}
			if _, err := h.scheduling.plans.RequirePaid(ctx, userID); err != nil {
				return err
			}
			return pgx.BeginFunc(ctx, h.pool, func(tx pgx.Tx) error {
				q := h.queries.WithTx(tx)
				if err := q.UpsertGmailConnection(ctx, db.UpsertGmailConnectionParams{
					UserID: userID, Email: email, RefreshTokenEnc: encrypted, Scopes: scopes,
				}); err != nil {
					return err
				}
				return q.EnableInterviewScheduling(ctx, userID)
			})
		},
	})
}

func (h *inboxHandlers) interviewSchedulingDisable(c *fiber.Ctx) error {
	id, _ := auth.UserID(c)
	if err := h.queries.DisableInterviewScheduling(c.UserContext(), id); err != nil {
		return err
	}
	return c.SendStatus(fiber.StatusNoContent)
}
