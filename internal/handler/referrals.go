package handler

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/blobstore"
	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/headshot"
	"github.com/strelov1/freehire/internal/referral"
)

// referralHandlers serves the employee-referral use cases (offer to refer, request a
// referral, moderate offers, notify referrers), delegated to referral.Service. blob is
// the S3 store proof CVs are written to (nil when S3 is unconfigured — offer submit then
// reports 503); cvStore + cvRenderer render a stored builder CV as an alternative proof.
type referralHandlers struct {
	referral   *referral.Service
	blob       blobstore.Store
	cvRenderer cv.Renderer
	cvStore    *cv.Store
	// photos serves the OWNER's headshot when their chosen template prints one, so the
	// proof a referrer opens is the CV the candidate sees, not a silhouette of them.
	photos *headshot.Store
}

func newReferralHandlers(referral *referral.Service, blob blobstore.Store, cvRenderer cv.Renderer, cvStore *cv.Store, photos *headshot.Store) *referralHandlers {
	return &referralHandlers{referral: referral, blob: blob, cvRenderer: cvRenderer, cvStore: cvStore, photos: photos}
}

func (h *referralHandlers) register(api fiber.Router, mw middleware) {
	// Employee referrals: any authenticated user (cookie or API key) offers to refer into a
	// company (proof CV, moderated) and requests a referral from a company's approved-referrer
	// pool; referrers manage their own incoming requests. The offer-moderation queue is
	// moderator-gated, mirroring the submissions queue above.
	api.Post("/me/referrals/offers", mw.key, h.SubmitReferralOffer)
	api.Get("/me/referrals/offers", mw.key, h.ListMyReferralOffers)
	api.Delete("/me/referrals/offers/:id", mw.key, h.WithdrawReferralOffer)
	api.Post("/me/referrals/requests", mw.key, h.CreateReferralRequest)
	api.Get("/me/referrals/requests", mw.key, h.ListMyReferralRequests)
	api.Get("/me/referrals/incoming", mw.key, h.ListIncomingReferralRequests)
	api.Get("/me/referrals/incoming/:id/cv", mw.key, h.ViewReferralRequestCV)
	api.Post("/me/referrals/incoming/:id/resolve", mw.key, h.ResolveReferralRequest)
	api.Get("/referrals/offers", mw.key, mw.moderator, h.ListPendingReferralOffers)
	api.Get("/referrals/offers/:id/proof", mw.key, mw.moderator, h.ViewReferralOfferProof)
	api.Post("/referrals/offers/:id/decide", mw.key, mw.moderator, h.DecideReferralOffer)
}

// referralOfferResponse is the public shape of an offer. user_id is omitted (ownership,
// internal); proof_object_key is never exposed (it points at a private S3 object).
type referralOfferResponse struct {
	ID          string     `json:"id"`
	CompanySlug string     `json:"company_slug"`
	CompanyName string     `json:"company_name"`
	LinkedInURL string     `json:"linkedin_url"`
	Status      string     `json:"status"`
	DecidedAt   *time.Time `json:"decided_at"`
	CreatedAt   *time.Time `json:"created_at"`
}

func toReferralOfferResponse(o referral.Offer) referralOfferResponse {
	return referralOfferResponse{
		ID: o.ID.String(), CompanySlug: o.CompanySlug, CompanyName: o.CompanyName,
		LinkedInURL: o.LinkedInURL, Status: o.Status,
		DecidedAt: o.DecidedAt, CreatedAt: o.CreatedAt,
	}
}

// seekerRequestResponse is what a seeker sees of their own request: no referrer identity
// (there is none to show — the request targets a pool), just the target and status.
type seekerRequestResponse struct {
	ID          string     `json:"id"`
	CompanySlug string     `json:"company_slug"`
	CompanyName string     `json:"company_name"`
	JobID       *int64     `json:"job_id"`
	CVKind      string     `json:"cv_kind"`
	CVID        *string    `json:"cv_id"`
	Status      string     `json:"status"`
	CreatedAt   *time.Time `json:"created_at"`
}

// optionalCVID parses an optional CV id from a request body. Absent stays absent;
// present-but-malformed is a client error, not a lookup that quietly finds nothing.
func optionalCVID(raw *string) (*uuid.UUID, error) {
	if raw == nil || *raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(*raw)
	if err != nil {
		return nil, fiber.NewError(fiber.StatusUnprocessableEntity, "cv_id is not a valid id")
	}
	return &id, nil
}

func toSeekerRequestResponse(r referral.Request) seekerRequestResponse {
	return seekerRequestResponse{
		ID: r.ID.String(), CompanySlug: r.CompanySlug, CompanyName: r.CompanyName, JobID: r.JobID,
		CVKind: r.CVKind, CVID: cvIDString(r.CVID), Status: r.Status, CreatedAt: r.CreatedAt,
	}
}

// incomingRequestResponse is what a referrer sees of an incoming request: the seeker's
// contact and CV choice to act on, plus the source vacancy. The seeker's user id stays
// hidden — the referrer reaches out over the contact the seeker chose to share.
type incomingRequestResponse struct {
	ID              string     `json:"id"`
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
		ID: r.ID.String(), CompanySlug: r.CompanySlug, CompanyName: r.CompanyName, JobID: r.JobID,
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
		errors.Is(err, referral.ErrContactTooLong),
		errors.Is(err, referral.ErrNoteTooLong),
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
	case errors.Is(err, referral.ErrProofStorageUnavailable):
		// Nothing was deleted — the offer stands and the same request can be retried.
		return fiber.NewError(fiber.StatusServiceUnavailable, "could not erase the proof CV; the offer was kept, please try again")
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
	CompanySlug     string  `json:"company_slug"`
	JobID           *int64  `json:"job_id"`
	CVKind          string  `json:"cv_kind"`
	CVID            *string `json:"cv_id"`
	LinkedInURL     string  `json:"linkedin_url"`
	ContactTelegram string  `json:"contact_telegram"`
	ContactEmail    string  `json:"contact_email"`
	Note            string  `json:"note"`
}

// CreateReferralRequest records a seeker's request into a company's referrer pool and pings
// the approved referrers. RequireAuth. Validation failures 422, no referrer 409, cap 429.
func (h *referralHandlers) CreateReferralRequest(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in createReferralRequestBody
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	// The attached CV is addressed by its opaque id; a malformed one names no CV,
	// which the service reports as an unusable attachment rather than a 500.
	cvID, err := optionalCVID(in.CVID)
	if err != nil {
		return err
	}
	req, err := h.referral.CreateRequest(c.Context(), referral.RequestInput{
		SeekerUserID:    userID,
		CompanySlug:     in.CompanySlug,
		JobID:           in.JobID,
		CVKind:          in.CVKind,
		CVID:            cvID,
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
func (h *referralHandlers) ListMyReferralRequests(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	rows, err := h.referral.ListMyRequests(c.Context(), userID)
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
func (h *referralHandlers) SubmitReferralOffer(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if h.blob == nil {
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
	if err := h.blob.Put(c.Context(), key, up.ContentType, bytes.NewReader(up.Data), int64(len(up.Data))); err != nil {
		return err
	}
	offer, err := h.referral.SubmitOffer(c.Context(), referral.OfferInput{
		UserID: userID, CompanySlug: companySlug, LinkedInURL: c.FormValue("linkedin_url"), ProofKey: key,
	})
	if err != nil {
		return referralError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": toReferralOfferResponse(offer)})
}

// ListMyReferralOffers returns the caller's own offers with moderation status, newest first.
func (h *referralHandlers) ListMyReferralOffers(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	rows, err := h.referral.ListMyOffers(c.Context(), userID)
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
func (h *referralHandlers) WithdrawReferralOffer(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := referralPathID(c, "offer not found")
	if err != nil {
		return err
	}
	if err := h.referral.WithdrawOffer(c.Context(), id, userID); err != nil {
		return referralError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// ListIncomingReferralRequests returns the open requests for every company the caller is an
// approved referrer of — their inbox.
func (h *referralHandlers) ListIncomingReferralRequests(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	rows, err := h.referral.ListIncoming(c.Context(), userID)
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
func (h *referralHandlers) ResolveReferralRequest(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := referralPathID(c, "referral request not found")
	if err != nil {
		return err
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
	req, err := h.referral.ResolveRequest(c.Context(), id, userID, contacted)
	if err != nil {
		return referralError(err)
	}
	return c.JSON(fiber.Map{"data": toIncomingRequestResponse(req)})
}

// ListPendingReferralOffers returns the moderator queue of offers awaiting a decision.
func (h *referralHandlers) ListPendingReferralOffers(c *fiber.Ctx) error {
	rows, err := h.referral.ListPendingOffers(c.Context())
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
func (h *referralHandlers) DecideReferralOffer(c *fiber.Ctx) error {
	moderatorID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := referralPathID(c, "offer not found")
	if err != nil {
		return err
	}
	var in decideReferralOfferBody
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	offer, err := h.referral.DecideOffer(c.Context(), id, moderatorID, in.Approve)
	if err != nil {
		return referralError(err)
	}
	return c.JSON(fiber.Map{"data": toReferralOfferResponse(offer)})
}

// ViewReferralRequestCV streams the CV a seeker attached to a request, to an authorized
// referrer of the request's company: the stored original résumé from S3, or the tailored
// builder CV rendered to PDF on the fly. AuthorizeCVAccess keeps this cabinet-only; the
// seeker's identity is never revealed by it.
func (h *referralHandlers) ViewReferralRequestCV(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := referralPathID(c, "referral request not found")
	if err != nil {
		return err
	}
	req, err := h.referral.AuthorizeCVAccess(c.Context(), id, userID)
	if err != nil {
		return referralError(err)
	}
	switch req.CVKind {
	case referral.CVOriginal:
		return h.streamBlobPDF(c, blobstore.ResumeKey(req.SeekerUserID))
	case referral.CVBuilt:
		if req.CVID == nil {
			return fiber.NewError(fiber.StatusNotFound, "the attached CV is no longer available")
		}
		return h.renderOwnerCV(c, *req.CVID, req.SeekerUserID)
	default:
		return fiber.NewError(fiber.StatusInternalServerError, "unknown CV kind")
	}
}

// ViewReferralOfferProof streams a member's proof CV to a moderator reviewing the offer.
// Moderator-gated at the route; the proof key never leaves the server.
func (h *referralHandlers) ViewReferralOfferProof(c *fiber.Ctx) error {
	id, err := referralPathID(c, "offer not found")
	if err != nil {
		return err
	}
	offer, ok, err := h.referral.GetOffer(c.Context(), id)
	if err != nil {
		return err
	}
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "offer not found")
	}
	return h.streamBlobPDF(c, offer.ProofKey)
}

// streamBlobPDF streams a stored PDF object inline. 503 when the blob store is
// unconfigured, 404 when the object is missing (e.g. the seeker deleted their résumé).
func (h *referralHandlers) streamBlobPDF(c *fiber.Ctx, key string) error {
	if h.blob == nil {
		return fiber.NewError(fiber.StatusServiceUnavailable, "file storage is unavailable")
	}
	rc, err := h.blob.Get(c.Context(), key)
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
func (h *referralHandlers) renderOwnerCV(c *fiber.Ctx, cvID uuid.UUID, ownerID int64) error {
	if h.cvRenderer == nil {
		return fiber.NewError(fiber.StatusNotImplemented, "PDF rendering is not available")
	}
	rec, err := h.cvStore.Get(c.Context(), cvID, ownerID)
	if err != nil {
		return mapCVError(err)
	}
	tmpl, err := cv.ResolveTemplate(rec.TemplateID)
	if err != nil {
		return mapCVError(err)
	}
	// Untraced on purpose, even for a CV whose owner has tracing on: this PDF is read inside the
	// product by someone the candidate already shared it with, so a click here says nothing about
	// an employer opening an application and would only pollute the count.
	pdf, err := h.cvRenderer.Render(c.Context(), rec.Document, tmpl,
		headshotForTemplate(c.Context(), h.photos, ownerID, tmpl), cv.LinkHrefs{})
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

// referralPathID parses the :id route param as a referral's UUID. A malformed id
// cannot name any referral, so it answers exactly as a missing one does, down to
// the message referralError would render. That matters more here than elsewhere:
// an incoming request's CV is read by an approved referrer rather than by the
// owner, so "not visible to you" is a membership answer, and none of the ways to
// miss should be told apart.
func referralPathID(c *fiber.Ctx, notFound string) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return uuid.Nil, fiber.NewError(fiber.StatusNotFound, notFound)
	}
	return id, nil
}
