package handler

import (
	"context"
	"errors"

	"github.com/gofiber/fiber/v2"

	dictclassify "github.com/strelov1/freehire/internal/dict/classify"
	"github.com/strelov1/freehire/internal/dict/industrytag"
	"github.com/strelov1/freehire/internal/dict/skilltag"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/platform/db"
)

var errUnknownDictionaryKind = errors.New("unknown dictionary kind")

type gapWrite func(ctx context.Context, userID int64, kind, raw, normalized string) (int32, string, error)

type dictionaryHandlers struct {
	queries *db.Queries
	write   gapWrite
}

func (h *dictionaryHandlers) register(api fiber.Router, mw middleware) {
	api.Post("/dictionary/gaps", mw.cookie, h.Record)
}

// dictionaryKnown reports whether the dictionary behind `kind` already resolves
// term, and refuses a kind it does not hold. Specializations, skills, and
// industries use the same dictionaries as their job/company pipelines.
func dictionaryKnown(kind, term string) (bool, error) {
	switch kind {
	case "specialization":
		return dictclassify.Parse(term).Category != "", nil
	case "skill":
		return len(skilltag.Canonicalize([]string{term})) > 0, nil
	case "industry":
		return len(industrytag.Canonicalize([]string{term})) > 0, nil
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
	result, err := h.recordTerm(c.Context(), userID, req)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": result})
}

func (h *dictionaryHandlers) recordTerm(
	ctx context.Context,
	userID int64,
	req dictionaryRecordRequest,
) (dictionaryRecordBody, error) {
	raw, normalized, ok := dictclassify.GapTerm(req.Term)
	if !ok {
		return dictionaryRecordBody{}, fiber.NewError(fiber.StatusBadRequest, "invalid term")
	}
	known, err := dictionaryKnown(req.Kind, raw)
	if errors.Is(err, errUnknownDictionaryKind) {
		return dictionaryRecordBody{}, fiber.NewError(fiber.StatusBadRequest, "unknown kind")
	}
	if known {
		return dictionaryRecordBody{}, fiber.NewError(fiber.StatusConflict, "term is already in the dictionary")
	}
	count, status, err := h.writeGap(
		ctx,
		userID,
		req.Kind,
		raw,
		normalized,
	)
	if err != nil {
		return dictionaryRecordBody{}, err
	}
	return dictionaryRecordBody{RequestCount: count, Status: status}, nil
}

// writeGap records the term through one atomic upsert.
func (h *dictionaryHandlers) writeGap(ctx context.Context, userID int64, kind, raw, normalized string) (int32, string, error) {
	if h.write != nil {
		return h.write(ctx, userID, kind, raw, normalized)
	}
	row, err := h.queries.RecordDictionaryGap(ctx, db.RecordDictionaryGapParams{
		Kind: kind, RawTerm: raw, NormalizedTerm: normalized, SubmittedBy: userID,
	})
	if err != nil {
		return 0, "", err
	}
	return row.RequestCount, row.Status, nil
}
