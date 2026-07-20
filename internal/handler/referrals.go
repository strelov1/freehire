package handler

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/blobstore"
	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/referral"
)

// referralOfferResponse is the public shape of an offer. user_id is omitted (ownership,
// internal); proof_object_key is never exposed (it points at a private S3 object).
type referralOfferResponse struct {
	ID          int64      `json:"id"`
	CompanySlug string     `json:"company_slug"`
	CompanyName string     `json:"company_name"`
	LinkedInURL string     `json:"linkedin_url"`
	Status      string     `json:"status"`
	DecidedAt   *time.Time `json:"decided_at"`
	CreatedAt   *time.Time `json:"created_at"`
}

func toReferralOfferResponse(o referral.Offer) referralOfferResponse {
	return referralOfferResponse{
		ID: o.ID, CompanySlug: o.CompanySlug, CompanyName: o.CompanyName,
		LinkedInURL: o.LinkedInURL, Status: o.Status,
		DecidedAt: o.DecidedAt, CreatedAt: o.CreatedAt,
	}
}

// seekerRequestResponse is what a seeker sees of their own request: no referrer identity
// (there is none to show — the request targets a pool), just the target and status.
type seekerRequestResponse struct {
	ID          int64      `json:"id"`
	CompanySlug string     `json:"company_slug"`
	CompanyName string     `json:"company_name"`
	JobID       *int64     `json:"job_id"`
	CVKind      string     `json:"cv_kind"`
	CVID        *int64     `json:"cv_id"`
	Status      string     `json:"status"`
	CreatedAt   *time.Time `json:"created_at"`
}

func toSeekerRequestResponse(r referral.Request) seekerRequestResponse {
	return seekerRequestResponse{
		ID: r.ID, CompanySlug: r.CompanySlug, CompanyName: r.CompanyName, JobID: r.JobID,
		CVKind: r.CVKind, CVID: r.CVID, Status: r.Status, CreatedAt: r.CreatedAt,
	}
}

// incomingRequestResponse is what a referrer sees of an incoming request: the seeker's
// contact and CV choice to act on, plus the source vacancy. The seeker's user id stays
// hidden — the referrer reaches out over the contact the seeker chose to share.
type incomingRequestResponse struct {
	ID              int64      `json:"id"`
	CompanySlug     string     `json:"company_slug"`
	CompanyName     string     `json:"company_name"`
	JobID           *int64     `json:"job_id"`
	CVKind          string     `json:"cv_kind"`
	LinkedInURL     string     `json:"linkedin_url,omitempty"`
	ContactTelegram string     `json:"contact_telegram,omitempty"`
	ContactEmail    string     `json:"contact_email,omitempty"`
	Note            string     `json:"note,omitempty"`
	Status          string     `json:"status"`
	CreatedAt       *time.Time `json:"created_at"`
}

func toIncomingRequestResponse(r referral.Request) incomingRequestResponse {
	return incomingRequestResponse{
		ID: r.ID, CompanySlug: r.CompanySlug, CompanyName: r.CompanyName, JobID: r.JobID,
		CVKind: r.CVKind, LinkedInURL: r.LinkedInURL,
		ContactTelegram: r.ContactTelegram, ContactEmail: r.ContactEmail, Note: r.Note,
		Status: r.Status, CreatedAt: r.CreatedAt,
	}
}

// referralError maps the referral sentinels to HTTP statuses. Validation failures are 422,
// authorization is 403, missing targets 404, the cap is 429, and conflicts (duplicate,
// not-pending, not-open) are 409; anything else falls through to RenderError as 500.
func referralError(err error) error {
	switch {
	case errors.Is(err, referral.ErrProofRequired),
		errors.Is(err, referral.ErrInvalidLinkedIn),
		errors.Is(err, referral.ErrNoContact),
		errors.Is(err, referral.ErrInvalidCVChoice),
		errors.Is(err, referral.ErrNoResume):
		return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, referral.ErrNotAuthorized):
		return fiber.NewError(fiber.StatusForbidden, "not an approved referrer for this company")
	case errors.Is(err, referral.ErrRequestNotFound):
		return fiber.NewError(fiber.StatusNotFound, "referral request not found")
	case errors.Is(err, referral.ErrCompanyNotFound):
		return fiber.NewError(fiber.StatusNotFound, "we don't have that company")
	case errors.Is(err, referral.ErrDailyCapReached):
		return fiber.NewError(fiber.StatusTooManyRequests, "daily referral request limit reached")
	case errors.Is(err, referral.ErrCompanyNotEligible):
		return fiber.NewError(fiber.StatusConflict, "this company has no referral available")
	case errors.Is(err, referral.ErrAlreadyOffered):
		return fiber.NewError(fiber.StatusConflict, "you already offered to refer for this company")
	case errors.Is(err, referral.ErrOfferNotPending):
		return fiber.NewError(fiber.StatusConflict, "this offer is not pending")
	case errors.Is(err, referral.ErrOfferNotFound):
		return fiber.NewError(fiber.StatusNotFound, "offer not found")
	case errors.Is(err, referral.ErrAlreadyRequested):
		return fiber.NewError(fiber.StatusConflict, "you already have an active request for this company")
	case errors.Is(err, referral.ErrRequestNotOpen):
		return fiber.NewError(fiber.StatusConflict, "this request has already been handled")
	default:
		return err
	}
}

// createReferralRequestBody is the seeker's submit payload. Exactly one of the CV fields is
// meaningful per cv_kind (validated in the domain); the contact is Telegram and/or email.
type createReferralRequestBody struct {
	CompanySlug     string `json:"company_slug"`
	JobID           *int64 `json:"job_id"`
	CVKind          string `json:"cv_kind"`
	CVID            *int64 `json:"cv_id"`
	LinkedInURL     string `json:"linkedin_url"`
	ContactTelegram string `json:"contact_telegram"`
	ContactEmail    string `json:"contact_email"`
	Note            string `json:"note"`
}

// CreateReferralRequest records a seeker's request into a company's referrer pool and pings
// the approved referrers. RequireAuth. Validation failures 422, no referrer 409, cap 429.
func (a *API) CreateReferralRequest(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in createReferralRequestBody
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	req, err := a.referral.CreateRequest(c.Context(), referral.RequestInput{
		SeekerUserID:    userID,
		CompanySlug:     in.CompanySlug,
		JobID:           in.JobID,
		CVKind:          in.CVKind,
		CVID:            in.CVID,
		LinkedInURL:     in.LinkedInURL,
		ContactTelegram: in.ContactTelegram,
		ContactEmail:    in.ContactEmail,
		Note:            in.Note,
	})
	if err != nil {
		return referralError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": toSeekerRequestResponse(req)})
}

// ListMyReferralRequests returns the caller's own referral requests, newest first.
func (a *API) ListMyReferralRequests(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	rows, err := a.referral.ListMyRequests(c.Context(), userID)
	if err != nil {
		return err
	}
	out := make([]seekerRequestResponse, len(rows))
	for i, r := range rows {
		out[i] = toSeekerRequestResponse(r)
	}
	return c.JSON(fiber.Map{"data": out})
}

// SubmitReferralOffer records a member's offer to refer into a company. The proof CV is a
// multipart "file" stored to S3; company_slug is a form field. RequireAuth; the offer waits
// on moderation. 503 when the blob store is unconfigured, 409 on a duplicate offer.
func (a *API) SubmitReferralOffer(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if a.blob == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "proof upload is unavailable")
	}
	companySlug := c.FormValue("company_slug")
	if companySlug == "" {
		return fiber.NewError(fiber.StatusBadRequest, "company_slug is required")
	}
	up, err := readResumeUpload(c)
	if err != nil {
		return err
	}
	key := referralProofKey(userID, companySlug)
	if err := a.blob.Put(c.Context(), key, up.ContentType, bytes.NewReader(up.Data), int64(len(up.Data))); err != nil {
		return err
	}
	offer, err := a.referral.SubmitOffer(c.Context(), referral.OfferInput{
		UserID: userID, CompanySlug: companySlug, LinkedInURL: c.FormValue("linkedin_url"), ProofKey: key,
	})
	if err != nil {
		return referralError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": toReferralOfferResponse(offer)})
}

// ListMyReferralOffers returns the caller's own offers with moderation status, newest first.
func (a *API) ListMyReferralOffers(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	rows, err := a.referral.ListMyOffers(c.Context(), userID)
	if err != nil {
		return err
	}
	out := make([]referralOfferResponse, len(rows))
	for i, o := range rows {
		out[i] = toReferralOfferResponse(o)
	}
	return c.JSON(fiber.Map{"data": out})
}

// WithdrawReferralOffer lets a member stop being a referrer by deleting their own offer.
// RequireAuth; owner-scoped in the service. 404 when the offer is absent or not theirs.
func (a *API) WithdrawReferralOffer(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid offer id")
	}
	if err := a.referral.WithdrawOffer(c.Context(), int64(id), userID); err != nil {
		return referralError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ListIncomingReferralRequests returns the open requests for every company the caller is an
// approved referrer of — their inbox.
func (a *API) ListIncomingReferralRequests(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	rows, err := a.referral.ListIncoming(c.Context(), userID)
	if err != nil {
		return err
	}
	out := make([]incomingRequestResponse, len(rows))
	for i, r := range rows {
		out[i] = toIncomingRequestResponse(r)
	}
	return c.JSON(fiber.Map{"data": out})
}

// resolveReferralRequestBody carries the referrer's mark: "contacted" or "declined".
type resolveReferralRequestBody struct {
	Status string `json:"status"`
}

// ResolveReferralRequest marks an incoming request contacted or declined on the caller's
// behalf, after verifying they are an approved referrer of the request's company.
func (a *API) ResolveReferralRequest(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request id")
	}
	var in resolveReferralRequestBody
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	var contacted bool
	switch in.Status {
	case referral.RequestContacted:
		contacted = true
	case referral.RequestDeclined:
		contacted = false
	default:
		return fiber.NewError(fiber.StatusBadRequest, "status must be contacted or declined")
	}
	req, err := a.referral.ResolveRequest(c.Context(), int64(id), userID, contacted)
	if err != nil {
		return referralError(err)
	}
	return c.JSON(fiber.Map{"data": toIncomingRequestResponse(req)})
}

// ListPendingReferralOffers returns the moderator queue of offers awaiting a decision.
func (a *API) ListPendingReferralOffers(c *fiber.Ctx) error {
	rows, err := a.referral.ListPendingOffers(c.Context())
	if err != nil {
		return err
	}
	out := make([]referralOfferResponse, len(rows))
	for i, o := range rows {
		out[i] = toReferralOfferResponse(o)
	}
	return c.JSON(fiber.Map{"data": out})
}

// decideReferralOfferBody carries the moderator's verdict.
type decideReferralOfferBody struct {
	Approve bool `json:"approve"`
}

// DecideReferralOffer approves or rejects a pending offer. Moderator-gated.
func (a *API) DecideReferralOffer(c *fiber.Ctx) error {
	moderatorID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid offer id")
	}
	var in decideReferralOfferBody
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	offer, err := a.referral.DecideOffer(c.Context(), int64(id), moderatorID, in.Approve)
	if err != nil {
		return referralError(err)
	}
	return c.JSON(fiber.Map{"data": toReferralOfferResponse(offer)})
}

// ViewReferralRequestCV streams the CV a seeker attached to a request, to an authorized
// referrer of the request's company: the stored original résumé from S3, or the tailored
// builder CV rendered to PDF on the fly. AuthorizeCVAccess keeps this cabinet-only; the
// seeker's identity is never revealed by it.
func (a *API) ViewReferralRequestCV(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request id")
	}
	req, err := a.referral.AuthorizeCVAccess(c.Context(), int64(id), userID)
	if err != nil {
		return referralError(err)
	}
	switch req.CVKind {
	case referral.CVOriginal:
		return a.streamBlobPDF(c, blobstore.ResumeKey(req.SeekerUserID))
	case referral.CVBuilt:
		if req.CVID == nil {
			return fiber.NewError(fiber.StatusNotFound, "the attached CV is no longer available")
		}
		return a.renderOwnerCV(c, *req.CVID, req.SeekerUserID)
	default:
		return fiber.NewError(fiber.StatusInternalServerError, "unknown CV kind")
	}
}

// ViewReferralOfferProof streams a member's proof CV to a moderator reviewing the offer.
// Moderator-gated at the route; the proof key never leaves the server.
func (a *API) ViewReferralOfferProof(c *fiber.Ctx) error {
	id, err := c.ParamsInt("id")
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid offer id")
	}
	offer, ok, err := a.referral.GetOffer(c.Context(), int64(id))
	if err != nil {
		return err
	}
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "offer not found")
	}
	return a.streamBlobPDF(c, offer.ProofKey)
}

// streamBlobPDF streams a stored PDF object inline. 503 when the blob store is
// unconfigured, 404 when the object is missing (e.g. the seeker deleted their résumé).
func (a *API) streamBlobPDF(c *fiber.Ctx, key string) error {
	if a.blob == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "file storage is unavailable")
	}
	rc, err := a.blob.Get(c.Context(), key)
	if err != nil {
		return fiber.NewError(fiber.StatusNotFound, "CV not available")
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return err
	}
	c.Set(fiber.HeaderContentType, "application/pdf")
	c.Set(fiber.HeaderContentDisposition, `inline; filename="cv.pdf"`)
	return c.Send(data)
}

// renderOwnerCV renders a builder CV owned by ownerID to PDF. cvStore.Get is owner-scoped,
// so it is loaded as the seeker (the owner), not the viewing referrer. 501 when no renderer.
func (a *API) renderOwnerCV(c *fiber.Ctx, cvID, ownerID int64) error {
	if a.cvRenderer == nil {
		return fiber.NewError(fiber.StatusNotImplemented, "PDF rendering is not available")
	}
	rec, err := a.cvStore.Get(c.Context(), cvID, ownerID)
	if err != nil {
		return mapCVError(err)
	}
	tmpl, err := cv.ResolveTemplate(rec.TemplateID)
	if err != nil {
		return mapCVError(err)
	}
	pdf, err := a.cvRenderer.Render(c.Context(), rec.Document, tmpl)
	if err != nil {
		return err
	}
	c.Set(fiber.HeaderContentType, "application/pdf")
	c.Set(fiber.HeaderContentDisposition, `inline; filename="cv.pdf"`)
	return c.Send(pdf)
}

// referralProofKey is the S3 key of a member's proof CV for a company. One offer per
// (user, company) makes it stable, so a re-upload overwrites the same object.
func referralProofKey(userID int64, companySlug string) string {
	return fmt.Sprintf("referral-proof/%d/%s.pdf", userID, companySlug)
}
