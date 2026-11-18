package observability

import (
	"context"
	"errors"
	"log/slog"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/strelov1/freehire/internal/platform/pgerr"
)

const RequestIDKey = "diagnostics.request_id"
const requestIdentityKey = "diagnostics.identity"

// RequestIDs generates an identifier rather than trusting a caller-supplied header.
// RequestCtx values also survive Fiber's net/http adaptor into MCP tool contexts.
func RequestIDs() fiber.Handler {
	return func(c *fiber.Ctx) error {
		id := uuid.NewString()
		c.Context().SetUserValue(RequestIDKey, id)
		c.Set("X-Request-ID", id)
		return c.Next()
	}
}

func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(RequestIDKey).(string)
	return id
}

// RequestIdentity contains only resolved database identifiers, never credentials.
// KeyID is zero for sessions and OAuth grants.
type RequestIdentity struct {
	Kind   string
	UserID int64
	KeyID  int64
	Scope  string
}

func IdentifyRequest(c *fiber.Ctx, identity RequestIdentity) {
	c.Context().SetUserValue(requestIdentityKey, identity)
}

func RequestLogger(ctx context.Context) *slog.Logger {
	logger := slog.Default().With("request_id", RequestID(ctx))
	if identity, ok := ctx.Value(requestIdentityKey).(RequestIdentity); ok {
		logger = logger.With("credential_type", identity.Kind, "user_id", identity.UserID)
		if identity.KeyID != 0 {
			logger = logger.With("key_id", identity.KeyID)
		}
		if identity.Scope != "" {
			logger = logger.With("scope", identity.Scope)
		}
	}
	return logger
}

func LogAuthFailure(c *fiber.Ctx, status int, reason string) {
	RequestLogger(c.Context()).InfoContext(c.Context(), "authentication refused",
		"status", status, "reason", reason, "route", c.Route().Path)
}

// ErrorCode classifies types and sentinels, never error messages or request data.
// It describes diagnostics only; it does not choose an HTTP response status.
func ErrorCode(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	case errors.Is(err, pgx.ErrNoRows):
		return "not_found"
	}
	var fe *fiber.Error
	if errors.As(err, &fe) {
		switch fe.Code {
		case 400, 422:
			return "invalid_request"
		case 401:
			return "unauthenticated"
		case 402:
			return "payment_required"
		case 403:
			return "permission_denied"
		case 404:
			return "not_found"
		case 409:
			return "conflict"
		case 429:
			return "rate_limited"
		case 503:
			return "unavailable"
		}
	}
	if pgerr.IsPostgresError(err) {
		return "database_error"
	}
	return "internal_error"
}
