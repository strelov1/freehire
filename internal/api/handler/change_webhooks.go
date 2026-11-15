package handler

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/engage/changehook"
	"github.com/strelov1/freehire/internal/identity/billing"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

const (
	maxChangeWebhooks         = 5
	maxChangeWebhookCompanies = 100
)

var allChangeKinds = []string{"opened", "closed", "reopened"}

// changeWebhookHandlers manages the account's push subscriptions to the change feed. The
// deliveries themselves are cmd/changes-webhook (internal/engage/changehook).
type changeWebhookHandlers struct {
	queries *db.Queries
	tester  changeWebhookTester
}

// changeWebhookTester sends a signed test event: changehook.Sender in production.
type changeWebhookTester interface {
	Test(ctx context.Context, url, secret string) error
}

func newChangeWebhookHandlers(queries *db.Queries, tester changeWebhookTester) *changeWebhookHandlers {
	return &changeWebhookHandlers{queries: queries, tester: tester}
}

// register: a session or a full-scope key, so a customer's own tooling can manage it; the
// API plan is checked on creation, in the handler.
func (h *changeWebhookHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	api.Post("/me/change-webhooks", mw.key, contract)
	api.Get("/me/change-webhooks", mw.key, contract)
	api.Delete("/me/change-webhooks/:id", mw.key, contract)
	api.Post("/me/change-webhooks/:id/test", mw.key, contract)
}

func (h *changeWebhookHandlers) CreateChangeWebhook(ctx context.Context, req *apigen.ChangeWebhookCreate) (*apigen.CreateChangeWebhookCreated, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	row, err := h.queries.GetUserAPIEntitlement(ctx, userID)
	if err != nil {
		return nil, err
	}
	if apiPlanFrom(row, time.Now()).Tier == billing.APITierNone {
		return nil, fiber.NewError(fiber.StatusPaymentRequired, "change webhooks need an API plan")
	}

	url := strings.TrimSpace(req.URL)
	if err := changehook.ValidateURL(url); err != nil {
		return nil, fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	slugs, err := normalizeCompanySlugs(req.CompanySlugs)
	if err != nil {
		return nil, err
	}
	kinds := make([]string, 0, len(req.Kinds))
	for _, k := range req.Kinds {
		if !slices.Contains(kinds, string(k)) {
			kinds = append(kinds, string(k))
		}
	}
	if len(kinds) == 0 {
		kinds = allChangeKinds
	}

	if n, err := h.queries.CountChangeWebhooks(ctx, userID); err != nil {
		return nil, err
	} else if n >= maxChangeWebhooks {
		return nil, fiber.NewError(fiber.StatusConflict, fmt.Sprintf("at most %d change webhooks per account", maxChangeWebhooks))
	}

	secret, err := newWebhookSecret()
	if err != nil {
		return nil, err
	}
	// No probe here: the receiver cannot verify a signature before it has the secret this call
	// returns. The /test endpoint checks it afterwards.
	created, err := h.queries.CreateChangeWebhook(ctx, db.CreateChangeWebhookParams{
		UserID: userID, URL: url, Secret: secret, CompanySlugs: slugs, Kinds: kinds,
	})
	if err != nil {
		return nil, err
	}
	out := changeWebhookOut(created.ID, created.URL, created.CompanySlugs, created.Kinds, created.ConsecutiveFailures,
		created.LastError, created.DisabledReason, created.CreatedAt, created.LastSuccessAt, created.DisabledAt)
	return &apigen.CreateChangeWebhookCreated{Data: apigen.ChangeWebhookCreated{
		ID: out.ID, URL: out.URL, CompanySlugs: out.CompanySlugs, Kinds: out.Kinds, CreatedAt: out.CreatedAt,
		LastSuccessAt: out.LastSuccessAt, ConsecutiveFailures: out.ConsecutiveFailures, LastError: out.LastError,
		DisabledAt: out.DisabledAt, DisabledReason: out.DisabledReason, Secret: secret,
	}}, nil
}

func (h *changeWebhookHandlers) ListChangeWebhooks(ctx context.Context) (*apigen.ListChangeWebhooksOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := h.queries.ListChangeWebhooks(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]apigen.ChangeWebhook, len(rows))
	for i, r := range rows {
		out[i] = changeWebhookOut(r.ID, r.URL, r.CompanySlugs, r.Kinds, r.ConsecutiveFailures,
			r.LastError, r.DisabledReason, r.CreatedAt, r.LastSuccessAt, r.DisabledAt)
	}
	return &apigen.ListChangeWebhooksOK{Data: out}, nil
}

func (h *changeWebhookHandlers) DeleteChangeWebhook(ctx context.Context, params apigen.DeleteChangeWebhookParams) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	id, err := contractPathID(params.ID)
	if err != nil {
		return err
	}
	n, err := h.queries.DeleteChangeWebhook(ctx, db.DeleteChangeWebhookParams{ID: id, UserID: userID})
	if err != nil {
		return err
	}
	if n == 0 {
		return fiber.NewError(fiber.StatusNotFound, "change webhook not found")
	}
	return nil
}

func (h *changeWebhookHandlers) TestChangeWebhook(ctx context.Context, params apigen.TestChangeWebhookParams) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	id, err := contractPathID(params.ID)
	if err != nil {
		return err
	}
	row, err := h.queries.GetChangeWebhook(ctx, db.GetChangeWebhookParams{ID: id, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return fiber.NewError(fiber.StatusNotFound, "change webhook not found")
	}
	if err != nil {
		return err
	}
	if err := h.tester.Test(ctx, row.URL, row.Secret); err != nil {
		return probeFailure(err)
	}
	return nil
}

// probeFailure is a 422 naming what the endpoint answered, without the package prefix.
func probeFailure(err error) error {
	return fiber.NewError(fiber.StatusUnprocessableEntity, "webhook test failed: "+strings.TrimPrefix(err.Error(), "changehook: "))
}

func newWebhookSecret() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return "whsec_" + hex.EncodeToString(b), nil
}

// normalizeCompanySlugs lowercases, trims and dedupes, and bounds the list.
func normalizeCompanySlugs(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.ToLower(strings.TrimSpace(s))
		if s == "" {
			return nil, fiber.NewError(fiber.StatusBadRequest, "company_slugs must not contain an empty slug")
		}
		if !slices.Contains(out, s) {
			out = append(out, s)
		}
	}
	if len(out) > maxChangeWebhookCompanies {
		return nil, fiber.NewError(fiber.StatusBadRequest, fmt.Sprintf("at most %d company_slugs", maxChangeWebhookCompanies))
	}
	return out, nil
}

func changeWebhookOut(id int64, url string, slugs, kinds []string, failures int64, lastErr, disabledReason string,
	created, lastOK, disabled pgtype.Timestamptz) apigen.ChangeWebhook {
	ks := make([]apigen.ChangeWebhookKind, len(kinds))
	for i, k := range kinds {
		ks[i] = apigen.ChangeWebhookKind(k)
	}
	out := apigen.ChangeWebhook{
		ID: id, URL: url, CompanySlugs: slugs, Kinds: ks, CreatedAt: created.Time,
		ConsecutiveFailures: failures, LastError: lastErr, DisabledReason: disabledReason,
	}
	if t := pgconv.TimePtr(lastOK); t != nil {
		out.LastSuccessAt.SetTo(*t)
	} else {
		out.LastSuccessAt.SetToNull()
	}
	if t := pgconv.TimePtr(disabled); t != nil {
		out.DisabledAt.SetTo(*t)
	} else {
		out.DisabledAt.SetToNull()
	}
	return out
}
