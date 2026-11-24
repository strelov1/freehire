package handler

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/engage/sponsored"
	"github.com/strelov1/freehire/internal/identity/billing"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
)

// sponsoredStatsDays is how far back a campaign's dashboard reads.
const sponsoredStatsDays = 30

type topupLinker interface {
	SponsoredTopupURL(ctx context.Context, userID, campaignID, cents int64) (string, error)
}

// sponsoredHandlers serve the buyer's campaigns and the moderators' review queue. The click
// redirect is separate (sponsoredClicks). Owner access is by the session user only: another
// user's campaign answers 404, never 403.
type sponsoredHandlers struct {
	svc     *sponsored.Service
	queries *db.Queries
	billing topupLinker
	now     func() time.Time
}

func newSponsoredHandlers(svc *sponsored.Service, queries *db.Queries, b topupLinker) *sponsoredHandlers {
	return &sponsoredHandlers{svc: svc, queries: queries, billing: b, now: time.Now}
}

// wireSponsored builds the handlers. Top-ups are paid through billing but credited by sponsored
// (which billing cannot import), so billing is told who credits them and sponsored who refunds.
func wireSponsored(pool *pgxpool.Pool, queries *db.Queries, b *billing.Service) *sponsoredHandlers {
	svc := sponsored.New(pool).WithRefunder(b).WithMinCPC(sponsored.MinCPCFromEnv())
	b.SetTopupCrediter(svc)
	return newSponsoredHandlers(svc, queries, b)
}

func (h *sponsoredHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	// Cookie only: these routes spend money and read what a buyer paid.
	api.Post("/sponsored/campaigns", mw.cookie, contract)
	api.Get("/sponsored/campaigns", mw.cookie, contract)
	api.Get("/sponsored/campaigns/:id", mw.cookie, contract)
	api.Post("/sponsored/campaigns/:id/pause", mw.cookie, contract)
	api.Post("/sponsored/campaigns/:id/resume", mw.cookie, contract)
	api.Post("/sponsored/campaigns/:id/end", mw.cookie, contract)
	api.Post("/sponsored/campaigns/:id/topup", mw.cookie, contract)
	api.Get("/sponsored/review", mw.cookie, mw.moderator, contract)
	api.Post("/sponsored/review/:id/approve", mw.cookie, mw.moderator, contract)
	api.Post("/sponsored/review/:id/reject", mw.cookie, mw.moderator, contract)
}

const noStore = "no-store"

func sponsoredError(err error) error {
	switch {
	case errors.Is(err, sponsored.ErrInvalid), errors.Is(err, sponsored.ErrJobUnavailable):
		return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, sponsored.ErrBadTransition):
		return fiber.NewError(fiber.StatusConflict, "the campaign is not in a state that allows this")
	default:
		return err // pgx.ErrNoRows renders as 404
	}
}

func sponsoredView(c db.SponsoredCampaign, title, company, slug string) apigen.SponsoredCampaign {
	v := apigen.SponsoredCampaign{
		ID: c.ID, Status: c.Status, JobSlug: slug, JobTitle: title, JobCompany: company,
		CpcCents: c.CpcCents, BudgetCents: c.BudgetCents, SpentCents: c.SpentCents,
		RemainingCents: c.BudgetCents - c.SpentCents, RejectReason: optString(c.RejectReason.String),
		CreatedAt: c.CreatedAt.Time,
	}
	if n := pgconv.Int8Ptr(c.DailyCapCents); n != nil {
		v.DailyCapCents = apigen.NewOptInt64(*n)
	}
	return v
}

// owned loads the caller's campaign or fails with 404.
func (h *sponsoredHandlers) owned(ctx context.Context, idParam string) (userID int64, row db.GetSponsoredCampaignForOwnerRow, err error) {
	if userID, err = requireUserIDFromContext(ctx); err != nil {
		return
	}
	id, err := contractPathID(idParam)
	if err != nil {
		return
	}
	row, err = h.queries.GetSponsoredCampaignForOwner(ctx, db.GetSponsoredCampaignForOwnerParams{ID: id, OwnerUserID: userID})
	return
}

func (h *sponsoredHandlers) CreateSponsoredCampaign(ctx context.Context, req *apigen.SponsoredCampaignRequest) (*apigen.CreateSponsoredCampaignCreatedHeaders, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	var dailyCap *int64
	if req.DailyCapCents.Set {
		dailyCap = &req.DailyCapCents.Value
	}
	c, err := h.svc.Create(ctx, userID, strings.TrimSpace(req.JobSlug.Value), req.CpcCents.Value, dailyCap)
	if err != nil {
		return nil, sponsoredError(err)
	}
	row, err := h.queries.GetSponsoredCampaignForOwner(ctx, db.GetSponsoredCampaignForOwnerParams{ID: c.ID, OwnerUserID: userID})
	if err != nil {
		return nil, err
	}
	return &apigen.CreateSponsoredCampaignCreatedHeaders{CacheControl: noStore, Response: apigen.CreateSponsoredCampaignCreated{
		Data: sponsoredView(row.SponsoredCampaign, row.JobTitle, row.JobCompany, row.JobSlug)}}, nil
}

func (h *sponsoredHandlers) ListSponsoredCampaigns(ctx context.Context) (*apigen.ListSponsoredCampaignsOKHeaders, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := h.queries.ListSponsoredCampaignsByOwner(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]apigen.SponsoredCampaign, len(rows))
	for i, r := range rows {
		out[i] = sponsoredView(r.SponsoredCampaign, r.JobTitle, r.JobCompany, r.JobSlug)
	}
	return &apigen.ListSponsoredCampaignsOKHeaders{CacheControl: noStore, Response: apigen.ListSponsoredCampaignsOK{Data: out}}, nil
}

func (h *sponsoredHandlers) GetSponsoredCampaign(ctx context.Context, params apigen.GetSponsoredCampaignParams) (*apigen.GetSponsoredCampaignOKHeaders, error) {
	_, row, err := h.owned(ctx, params.ID)
	if err != nil {
		return nil, err
	}
	now := h.now().UTC()
	since := now.AddDate(0, 0, -sponsoredStatsDays)
	days, err := h.queries.ListSponsoredDailySince(ctx, db.ListSponsoredDailySinceParams{CampaignID: row.SponsoredCampaign.ID, Since: pgconv.Date(&since)})
	if err != nil {
		return nil, err
	}
	reasons, err := h.queries.ListSponsoredInvalidReasonsSince(ctx, db.ListSponsoredInvalidReasonsSinceParams{CampaignID: row.SponsoredCampaign.ID, Since: pgconv.Instant(since)})
	if err != nil {
		return nil, err
	}
	d := apigen.SponsoredCampaignDetail{
		Campaign:              sponsoredView(row.SponsoredCampaign, row.JobTitle, row.JobCompany, row.JobSlug),
		Days:                  make([]apigen.SponsoredDayStat, len(days)),
		InvalidClicksByReason: make([]apigen.SponsoredInvalidReason, len(reasons)),
	}
	for i, s := range days {
		d.Days[i] = apigen.SponsoredDayStat{Day: s.Day.Time, Impressions: s.Impressions, ValidClicks: s.ValidClicks, InvalidClicks: s.InvalidClicks, SpentCents: s.SpentCents}
	}
	for i, r := range reasons {
		d.InvalidClicksByReason[i] = apigen.SponsoredInvalidReason{Reason: r.Reason, Clicks: r.Clicks}
	}
	return &apigen.GetSponsoredCampaignOKHeaders{CacheControl: noStore, Response: apigen.GetSponsoredCampaignOK{Data: d}}, nil
}

// move transitions the caller's campaign and returns it as it is afterwards.
func (h *sponsoredHandlers) move(ctx context.Context, idParam string, to sponsored.Status) (apigen.SponsoredCampaign, error) {
	userID, row, err := h.owned(ctx, idParam)
	if err != nil {
		return apigen.SponsoredCampaign{}, err
	}
	if err := h.svc.OwnerTransition(ctx, row.SponsoredCampaign.ID, to); err != nil {
		return apigen.SponsoredCampaign{}, sponsoredError(err)
	}
	row, err = h.queries.GetSponsoredCampaignForOwner(ctx, db.GetSponsoredCampaignForOwnerParams{ID: row.SponsoredCampaign.ID, OwnerUserID: userID})
	if err != nil {
		return apigen.SponsoredCampaign{}, err
	}
	return sponsoredView(row.SponsoredCampaign, row.JobTitle, row.JobCompany, row.JobSlug), nil
}

func (h *sponsoredHandlers) PauseSponsoredCampaign(ctx context.Context, params apigen.PauseSponsoredCampaignParams) (*apigen.PauseSponsoredCampaignOKHeaders, error) {
	v, err := h.move(ctx, params.ID, sponsored.StatusPaused)
	if err != nil {
		return nil, err
	}
	return &apigen.PauseSponsoredCampaignOKHeaders{CacheControl: noStore, Response: apigen.PauseSponsoredCampaignOK{Data: v}}, nil
}

func (h *sponsoredHandlers) ResumeSponsoredCampaign(ctx context.Context, params apigen.ResumeSponsoredCampaignParams) (*apigen.ResumeSponsoredCampaignOKHeaders, error) {
	v, err := h.move(ctx, params.ID, sponsored.StatusActive)
	if err != nil {
		return nil, err
	}
	return &apigen.ResumeSponsoredCampaignOKHeaders{CacheControl: noStore, Response: apigen.ResumeSponsoredCampaignOK{Data: v}}, nil
}

func (h *sponsoredHandlers) EndSponsoredCampaign(ctx context.Context, params apigen.EndSponsoredCampaignParams) (*apigen.EndSponsoredCampaignOKHeaders, error) {
	v, err := h.move(ctx, params.ID, sponsored.StatusEnded)
	if err != nil {
		return nil, err
	}
	return &apigen.EndSponsoredCampaignOKHeaders{CacheControl: noStore, Response: apigen.EndSponsoredCampaignOK{Data: v}}, nil
}

func (h *sponsoredHandlers) TopupSponsoredCampaign(ctx context.Context, req *apigen.SponsoredTopupRequest, params apigen.TopupSponsoredCampaignParams) (*apigen.TopupSponsoredCampaignOKHeaders, error) {
	userID, row, err := h.owned(ctx, params.ID)
	if err != nil {
		return nil, err
	}
	if s := sponsored.Status(row.SponsoredCampaign.Status); s == sponsored.StatusRejected || s == sponsored.StatusEnded {
		return nil, fiber.NewError(fiber.StatusConflict, "this campaign is over and cannot be topped up")
	}
	url, err := h.billing.SponsoredTopupURL(ctx, userID, row.SponsoredCampaign.ID, req.AmountCents.Value)
	var below *billing.BelowMinimumError
	switch {
	case errors.As(err, &below):
		return nil, fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, billing.ErrDisabled), errors.Is(err, billing.ErrNoCheckout):
		return nil, fiber.NewError(fiber.StatusNotFound, "payments are not available")
	case err != nil:
		return nil, err
	}
	return &apigen.TopupSponsoredCampaignOKHeaders{CacheControl: noStore, Response: apigen.TopupSponsoredCampaignOK{Data: apigen.BillingUrl{URL: url}}}, nil
}

func (h *sponsoredHandlers) ListSponsoredReview(ctx context.Context) (*apigen.ListSponsoredReviewOKHeaders, error) {
	rows, err := h.queries.ListSponsoredReviewQueue(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]apigen.SponsoredReviewItem, len(rows))
	for i, r := range rows {
		c := r.SponsoredCampaign
		item := apigen.SponsoredReviewItem{
			ID: c.ID, JobSlug: r.JobSlug, JobTitle: r.JobTitle, JobCompany: r.JobCompany, JobURL: r.JobUrl,
			CpcCents: c.CpcCents, BudgetCents: c.BudgetCents, OwnerUserID: c.OwnerUserID.Int64, CreatedAt: c.CreatedAt.Time,
		}
		if n := pgconv.Int8Ptr(c.DailyCapCents); n != nil {
			item.DailyCapCents = apigen.NewOptInt64(*n)
		}
		out[i] = item
	}
	return &apigen.ListSponsoredReviewOKHeaders{CacheControl: noStore, Response: apigen.ListSponsoredReviewOK{Data: out}}, nil
}

func (h *sponsoredHandlers) ApproveSponsoredCampaign(ctx context.Context, params apigen.ApproveSponsoredCampaignParams) (*apigen.ApproveSponsoredCampaignOKHeaders, error) {
	moderatorID, id, err := h.reviewIDs(ctx, params.ID)
	if err != nil {
		return nil, err
	}
	if err := h.svc.Approve(ctx, id, moderatorID); err != nil {
		return nil, sponsoredError(err)
	}
	return &apigen.ApproveSponsoredCampaignOKHeaders{CacheControl: noStore, Response: apigen.ApproveSponsoredCampaignOK{Data: apigen.SponsoredApproval{Approved: true}}}, nil
}

func (h *sponsoredHandlers) RejectSponsoredCampaign(ctx context.Context, req *apigen.SponsoredRejectRequest, params apigen.RejectSponsoredCampaignParams) (*apigen.RejectSponsoredCampaignOKHeaders, error) {
	moderatorID, id, err := h.reviewIDs(ctx, params.ID)
	if err != nil {
		return nil, err
	}
	reason := strings.TrimSpace(req.Reason.Value)
	if reason == "" {
		return nil, fiber.NewError(fiber.StatusUnprocessableEntity, "a rejection needs a reason")
	}
	err = h.svc.Reject(ctx, id, moderatorID, reason)
	// The rejection committed; only the refund is left, and the reconciler retries it.
	pending := errors.Is(err, sponsored.ErrRefundPending)
	if err != nil && !pending {
		return nil, sponsoredError(err)
	}
	return &apigen.RejectSponsoredCampaignOKHeaders{CacheControl: noStore, Response: apigen.RejectSponsoredCampaignOK{Data: apigen.SponsoredRejection{Rejected: true, RefundPending: pending}}}, nil
}

func (h *sponsoredHandlers) reviewIDs(ctx context.Context, idParam string) (moderatorID, id int64, err error) {
	if moderatorID, err = requireUserIDFromContext(ctx); err != nil {
		return
	}
	id, err = contractPathID(idParam)
	return
}
