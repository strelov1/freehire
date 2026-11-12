package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/headshot"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/engage/referral"
	"github.com/strelov1/freehire/internal/platform/blobstore"
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
	// jobs resolves the vacancy a request is made from, which the page names by its slug.
	jobs jobIDBySlug
	// resumes serves the seeker's uploaded CV when that is the one attached, read through its
	// pointer: each upload has its own object, so the key is never derived from the user id.
	resumes *resume.Store
}

// jobIDBySlug resolves a vacancy's public slug to its id. *db.Queries satisfies it.
type jobIDBySlug interface {
	GetJobIDBySlug(ctx context.Context, publicSlug string) (int64, error)
}

func newReferralHandlers(referral *referral.Service, blob blobstore.Store, cvRenderer cv.Renderer, cvStore *cv.Store, photos *headshot.Store, jobs jobIDBySlug, resumes *resume.Store) *referralHandlers {
	return &referralHandlers{referral: referral, blob: blob, cvRenderer: cvRenderer, cvStore: cvStore, photos: photos, jobs: jobs, resumes: resumes}
}

// requestJobID is the vacancy a request is about: job_slug when the page names one, else
// the legacy numeric job_id. An unknown slug is a 404 — the vacancy the seeker was looking
// at no longer exists — rather than a request silently detached from it.
func (h *referralHandlers) requestJobID(ctx context.Context, slug string, id *int64) (*int64, error) {
	if slug == "" {
		return id, nil
	}
	jobID, err := h.jobs.GetJobIDBySlug(ctx, slug)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, fiber.NewError(fiber.StatusNotFound, "vacancy not found")
	}
	if err != nil {
		return nil, err
	}
	return &jobID, nil
}

func (h *referralHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	// Employee referrals: any authenticated user (cookie or API key) offers to refer into a
	// company (proof CV, moderated) and requests a referral from a company's approved-referrer
	// pool; referrers manage their own incoming requests. The offer-moderation queue is
	// moderator-gated, mirroring the submissions queue above.
	api.Post("/me/referrals/offers", mw.key, h.SubmitReferralOffer)
	api.Get("/me/referrals/offers", mw.key, contract)
	api.Delete("/me/referrals/offers/:id", mw.key, contract)
	api.Post("/me/referrals/requests", mw.key, contract)
	api.Get("/me/referrals/cv-options", mw.key, contract)
	api.Get("/me/referrals/requests", mw.key, contract)
	api.Get("/me/referrals/incoming", mw.key, contract)
	api.Get("/me/referrals/incoming/:id/cv", mw.key, h.ViewReferralRequestCV)
	api.Post("/me/referrals/incoming/:id/resolve", mw.key, contract)
	api.Get("/referrals/offers", mw.key, mw.moderator, contract)
	api.Get("/referrals/offers/:id/proof", mw.key, mw.moderator, h.ViewReferralOfferProof)
	api.Post("/referrals/offers/:id/decide", mw.key, mw.moderator, contract)
}

// referralOfferOut is the public shape of an offer. user_id is omitted (ownership,
// internal); proof_object_key is never exposed (it points at a private S3 object).
func referralOfferOut(o referral.Offer) apigen.ReferralOffer {
	return apigen.ReferralOffer{
		ID: o.ID.String(), CompanySlug: o.CompanySlug, CompanyName: o.CompanyName,
		LinkedinURL: o.LinkedInURL, Status: apigen.ReferralOfferStatus(o.Status),
		DecidedAt: nilDateTime(o.DecidedAt), CreatedAt: nilDateTime(o.CreatedAt),
	}
}

func referralOffersOut(rows []referral.Offer) []apigen.ReferralOffer {
	out := make([]apigen.ReferralOffer, len(rows))
	for i, o := range rows {
		out[i] = referralOfferOut(o)
	}
	return out
}

func nilInt64(p *int64) apigen.NilInt64 {
	if p == nil {
		return apigen.NilInt64{Null: true}
	}
	return apigen.NewNilInt64(*p)
}

// seekerRequestOut is what a seeker sees of their own request: no referrer identity
// (there is none to show — the request targets a pool), just the target and status.
func seekerRequestOut(r referral.Request) apigen.ReferralSeekerRequest {
	return apigen.ReferralSeekerRequest{
		ID: r.ID.String(), CompanySlug: r.CompanySlug, CompanyName: r.CompanyName, JobID: nilInt64(r.JobID),
		CvKind: apigen.ReferralCvKind(r.CVKind), CvID: nilString(cvIDString(r.CVID)), Status: apigen.ReferralRequestStatus(r.Status), CreatedAt: nilDateTime(r.CreatedAt),
	}
}

// incomingRequestOut is what a referrer sees of an incoming request: the seeker's contact
// and CV choice to act on, plus the source vacancy. The seeker's user id stays hidden — the
// referrer reaches out over the contact the seeker chose to share.
func incomingRequestOut(r referral.Request) apigen.ReferralIncomingRequest {
	return apigen.ReferralIncomingRequest{
		ID: r.ID.String(), CompanySlug: r.CompanySlug, CompanyName: r.CompanyName, JobID: nilInt64(r.JobID),
		CvKind: apigen.ReferralCvKind(r.CVKind), LinkedinURL: optString(r.LinkedInURL),
		ContactTelegram: optString(r.ContactTelegram), ContactEmail: optString(r.ContactEmail), Note: optString(r.Note),
		Status: apigen.ReferralRequestStatus(r.Status), CreatedAt: nilDateTime(r.CreatedAt),
	}
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

// referralError maps the referral sentinels to HTTP statuses. Validation failures are 422,
// authorization is 403, missing targets 404, and conflicts (duplicate,
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

// CreateReferralRequest records a seeker's request into a company's referrer pool and pings
// the approved referrers. RequireAuth. Validation failures 422, no referrer 409, cap 429.
// Exactly one of the CV fields is meaningful per cv_kind (validated in the domain); the
// contact is Telegram and/or email.
func (h *referralHandlers) CreateReferralRequest(ctx context.Context, req *apigen.ReferralRequestCreate) (*apigen.CreateReferralRequestCreated, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	// The attached CV is addressed by its opaque id; a malformed one names no CV,
	// which the service reports as an unusable attachment rather than a 500.
	cvID, err := optionalCVID(ptrOf(req.CvID.Get))
	if err != nil {
		return nil, err
	}
	jobID, err := h.requestJobID(ctx, req.JobSlug.Or(""), ptrOf(req.JobID.Get))
	if err != nil {
		return nil, err
	}
	created, err := h.referral.CreateRequest(ctx, referral.RequestInput{
		SeekerUserID:    userID,
		CompanySlug:     req.CompanySlug.Or(""),
		JobID:           jobID,
		CVKind:          req.CvKind.Or(""),
		CVID:            cvID,
		LinkedInURL:     req.LinkedinURL.Or(""),
		ContactTelegram: req.ContactTelegram.Or(""),
		ContactEmail:    req.ContactEmail.Or(""),
		Note:            req.Note.Or(""),
	})
	if err != nil {
		return nil, referralError(err)
	}
	return &apigen.CreateReferralRequestCreated{Data: seekerRequestOut(created)}, nil
}

// ListReferralCVOptions lists what the caller can attach to a request into ?company_slug=,
// about ?job_slug= when asked from a vacancy: the CVs tailored for it (or, with no vacancy,
// for any vacancy at the company), newest first, and whether they uploaded a CV file.
func (h *referralHandlers) ListReferralCVOptions(ctx context.Context, params apigen.ListReferralCVOptionsParams) (*apigen.ListReferralCVOptionsOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	company := params.CompanySlug.Or("")
	if company == "" {
		return nil, fiber.NewError(fiber.StatusBadRequest, "company_slug is required")
	}
	jobID, err := h.requestJobID(ctx, params.JobSlug.Or(""), nil)
	if err != nil {
		return nil, err
	}
	opts, err := h.referral.CVOptions(ctx, userID, company, jobID)
	if err != nil {
		return nil, err
	}
	tailored := make([]apigen.ReferralCVOption, len(opts.Tailored))
	for i, o := range opts.Tailored {
		tailored[i] = apigen.ReferralCVOption{ID: o.ID.String(), Title: o.Title}
	}
	return &apigen.ListReferralCVOptionsOK{Data: apigen.ReferralCVOptions{Tailored: tailored, HasResume: opts.HasResume}}, nil
}

// ListMyReferralRequests returns the caller's own referral requests, newest first.
func (h *referralHandlers) ListMyReferralRequests(ctx context.Context) (*apigen.ListMyReferralRequestsOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := h.referral.ListMyRequests(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]apigen.ReferralSeekerRequest, len(rows))
	for i, r := range rows {
		out[i] = seekerRequestOut(r)
	}
	return &apigen.ListMyReferralRequestsOK{Data: out}, nil
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
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": referralOfferOut(offer)})
}

// ListMyReferralOffers returns the caller's own offers with moderation status, newest first.
func (h *referralHandlers) ListMyReferralOffers(ctx context.Context) (*apigen.ListMyReferralOffersOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := h.referral.ListMyOffers(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &apigen.ListMyReferralOffersOK{Data: referralOffersOut(rows)}, nil
}

// WithdrawReferralOffer lets a member stop being a referrer by deleting their own offer.
// RequireAuth; owner-scoped in the service. 404 when the offer is absent or not theirs.
func (h *referralHandlers) WithdrawReferralOffer(ctx context.Context, params apigen.WithdrawReferralOfferParams) error {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return err
	}
	id, err := referralUUID(params.ID, "offer not found")
	if err != nil {
		return err
	}
	return referralError(h.referral.WithdrawOffer(ctx, id, userID))
}

// ListIncomingReferralRequests returns the open requests for every company the caller is an
// approved referrer of — their inbox.
func (h *referralHandlers) ListIncomingReferralRequests(ctx context.Context) (*apigen.ListIncomingReferralRequestsOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := h.referral.ListIncoming(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]apigen.ReferralIncomingRequest, len(rows))
	for i, r := range rows {
		out[i] = incomingRequestOut(r)
	}
	return &apigen.ListIncomingReferralRequestsOK{Data: out}, nil
}

// ResolveReferralRequest marks an incoming request contacted or declined on the caller's
// behalf, after verifying they are an approved referrer of the request's company. The body's
// status is "contacted" or "declined".
func (h *referralHandlers) ResolveReferralRequest(ctx context.Context, req *apigen.ReferralResolve, params apigen.ResolveReferralRequestParams) (*apigen.ResolveReferralRequestOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := referralUUID(params.ID, "referral request not found")
	if err != nil {
		return nil, err
	}
	var contacted bool
	switch req.Status.Or("") {
	case referral.RequestContacted:
		contacted = true
	case referral.RequestDeclined:
		contacted = false
	default:
		return nil, fiber.NewError(fiber.StatusBadRequest, "status must be contacted or declined")
	}
	resolved, err := h.referral.ResolveRequest(ctx, id, userID, contacted)
	if err != nil {
		return nil, referralError(err)
	}
	return &apigen.ResolveReferralRequestOK{Data: incomingRequestOut(resolved)}, nil
}

// ListPendingReferralOffers returns the moderator queue of offers awaiting a decision.
func (h *referralHandlers) ListPendingReferralOffers(ctx context.Context) (*apigen.ListPendingReferralOffersOK, error) {
	rows, err := h.referral.ListPendingOffers(ctx)
	if err != nil {
		return nil, err
	}
	return &apigen.ListPendingReferralOffersOK{Data: referralOffersOut(rows)}, nil
}

// DecideReferralOffer approves or rejects a pending offer. Moderator-gated.
func (h *referralHandlers) DecideReferralOffer(ctx context.Context, req *apigen.ReferralDecision, params apigen.DecideReferralOfferParams) (*apigen.DecideReferralOfferOK, error) {
	moderatorID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := referralUUID(params.ID, "offer not found")
	if err != nil {
		return nil, err
	}
	offer, err := h.referral.DecideOffer(ctx, id, moderatorID, req.Approve.Or(false))
	if err != nil {
		return nil, referralError(err)
	}
	return &apigen.DecideReferralOfferOK{Data: referralOfferOut(offer)}, nil
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
		return h.streamUploadedCV(c, req.SeekerUserID)
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

// streamUploadedCV serves the CV a seeker uploaded, as streamBlobPDF serves a proof: 503 without
// storage, 404 when they have none any more.
func (h *referralHandlers) streamUploadedCV(c *fiber.Ctx, seekerID int64) error {
	if !h.resumes.Enabled() {
		return fiber.NewError(fiber.StatusServiceUnavailable, "file storage is unavailable")
	}
	data, err := h.resumes.File(c.Context(), seekerID)
	if errors.Is(err, resume.ErrNotStored) {
		return fiber.NewError(fiber.StatusNotFound, "CV not available")
	}
	if err != nil {
		return err
	}
	contentType, filename := uploadedCVType(data)
	c.Set(fiber.HeaderContentType, contentType)
	c.Set(fiber.HeaderContentDisposition, `inline; filename="`+filename+`"`)
	return c.Send(data)
}

// uploadedCVType names a stored CV's type the way the store reads it (resume.extractText): a
// PDF by its magic number, otherwise the pasted text it was stored as.
func uploadedCVType(data []byte) (contentType, filename string) {
	if bytes.HasPrefix(data, []byte("%PDF")) {
		return "application/pdf", "cv.pdf"
	}
	return "text/plain; charset=utf-8", "cv.txt"
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
	return referralUUID(c.Params("id"), notFound)
}

// referralUUID is referralPathID for an id the contract already read from the path.
func referralUUID(raw, notFound string) (uuid.UUID, error) {
	id, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, fiber.NewError(fiber.StatusNotFound, notFound)
	}
	return id, nil
}
