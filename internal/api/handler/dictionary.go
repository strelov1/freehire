package handler

import (
	"context"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	dictclassify "github.com/strelov1/freehire/internal/dict/classify"
	"github.com/strelov1/freehire/internal/dict/skilltag"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

var (
	errGapCap                = errors.New("dictionary gap daily cap")
	errUnknownDictionaryKind = errors.New("unknown dictionary kind")
)

type gapWrite func(ctx context.Context, userID int64, kind, raw, normalized string) (int32, string, error)

type dictionaryHandlers struct {
	pool    *pgxpool.Pool
	queries *db.Queries
	write   gapWrite
}

func (h *dictionaryHandlers) register(api fiber.Router, mw middleware) {
	api.Post("/dictionary/gaps", mw.cookie, h.Record)
}

// dictionaryKnown reports whether the dictionary behind `kind` already resolves
// term, and refuses a kind it does not hold. A specialization is resolved the way a
// job title is (classify.Parse), a skill the way a stated skill is (skilltag).
func dictionaryKnown(kind, term string) (bool, error) {
	switch kind {
	case "specialization":
		return dictclassify.Parse(term).Category != "", nil
	case "skill":
		return len(skilltag.Canonicalize([]string{term})) > 0, nil
	}
	return false, errUnknownDictionaryKind
}

type dictionaryRecordRequest struct {
	Kind string `json:"kind"`
	Term string `json:"term"`
}

type dictionaryRecordBody struct {
	RequestCount int32  `json:"request_count"`
	Status       string `json:"status"`
}

func (h *dictionaryHandlers) Record(c *fiber.Ctx) error {
	userID, ok := auth.UserID(c)
	if !ok {
		return fiber.ErrUnauthorized
	}
	var req dictionaryRecordRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	raw, normalized, ok := dictclassify.GapTerm(req.Term)
	if !ok {
		return fiber.NewError(fiber.StatusBadRequest, "invalid term")
	}
	known, err := dictionaryKnown(req.Kind, raw)
	if errors.Is(err, errUnknownDictionaryKind) {
		return fiber.NewError(fiber.StatusBadRequest, "unknown kind")
	}
	if known {
		return fiber.NewError(fiber.StatusConflict, "term is already in the dictionary")
	}
	count, status, err := h.writeGap(c.Context(), userID, req.Kind, raw, normalized)
	if errors.Is(err, errGapCap) {
		return fiber.NewError(fiber.StatusTooManyRequests, "daily dictionary gap cap")
	}
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": dictionaryRecordBody{RequestCount: count, Status: status}})
}

// writeGap spends one of the user's daily writes and records the term in the same
// transaction, so a refused cap (errGapCap) leaves the queue untouched.
func (h *dictionaryHandlers) writeGap(ctx context.Context, userID int64, kind, raw, normalized string) (int32, string, error) {
	if h.write != nil {
		return h.write(ctx, userID, kind, raw, normalized)
	}
	tx, err := h.pool.Begin(ctx)
	if err != nil {
		return 0, "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := h.queries.WithTx(tx)
	now := time.Now().UTC()
	day := pgtype.Date{Time: time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC), Valid: true}
	if _, err := q.ClaimDictionaryGapWrite(ctx, db.ClaimDictionaryGapWriteParams{UserID: userID, Day: day}); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return 0, "", errGapCap
		}
		return 0, "", err
	}
	row, err := q.RecordDictionaryGap(ctx, db.RecordDictionaryGapParams{
		Kind: kind, RawTerm: raw, NormalizedTerm: normalized, SubmittedBy: userID,
	})
	if err != nil {
		return 0, "", err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, "", err
	}
	return row.RequestCount, row.Status, nil
}
