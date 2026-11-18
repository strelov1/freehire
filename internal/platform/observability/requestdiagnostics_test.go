package observability

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func TestDiagnosticErrorCodesUseTypesInsteadOfMessages(t *testing.T) {
	for _, tc := range []struct {
		err  error
		code string
	}{
		{nil, ""},
		{context.Canceled, "canceled"},
		{context.DeadlineExceeded, "deadline_exceeded"},
		{fmt.Errorf("private user: %w", pgx.ErrNoRows), "not_found"},
		{fiber.NewError(400, "private argument"), "invalid_request"},
		{fiber.NewError(401, "private token"), "unauthenticated"},
		{fiber.NewError(403, "private permission"), "permission_denied"},
		{fiber.NewError(404, "private resource"), "not_found"},
		{fiber.NewError(409, "private conflict"), "conflict"},
		{fiber.NewError(422, "private validation"), "invalid_request"},
		{fiber.NewError(429, "private rate key"), "rate_limited"},
		{fiber.NewError(503, "private dependency"), "unavailable"},
		{&pgconn.PgError{Message: "private SQL"}, "database_error"},
		{errors.New("403 permission_denied private token"), "internal_error"},
	} {
		t.Run(tc.code, func(t *testing.T) {
			if got := ErrorCode(tc.err); got != tc.code {
				t.Errorf("code = %q, want %q", got, tc.code)
			}
		})
	}
}
