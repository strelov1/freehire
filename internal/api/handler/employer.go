package handler

import (
	"encoding/json"
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/identity/accounts"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/ingest/employer"
	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
)

// employerHandlers serves the verified-employer surface: claiming a company,
// editing its curated profile, and publishing/editing/closing its own vacancies
// (internal/ingest/employer.Service) — plus the moderator review queue for a claim that
// cannot auto-verify, and the admin revoke action.
type employerHandlers struct {
	queries  *db.Queries
	employer *employer.Service
	// adminOnly is built locally rather than added to the shared middleware struct: this
	// is the only route in the whole API gated on the admin role rather than moderator.
	adminOnly fiber.Handler
}

func newEmployerHandlers(queries *db.Queries, svc *employer.Service) *employerHandlers {
	return &employerHandlers{
		queries:   queries,
		employer:  svc,
		adminOnly: auth.RequireRole(queries, "admin"),
	}
}

func (h *employerHandlers) register(api fiber.Router, mw middleware) {
	// Cookie-only (RequireAuth), not mw.key: this MVP does not open employer actions to
	// API keys — see design.md's Non-Goals. Every route below requires ActiveAccount
	// except the claim/confirm pair, which is how an account BECOMES active.
	api.Post("/employer/claim", mw.cookie, h.Claim)
	api.Post("/employer/claim/confirm", mw.cookie, h.ConfirmClaim)
	api.Get("/employer/company", mw.cookie, h.GetCompany)
	api.Patch("/employer/company", mw.cookie, h.UpdateCompany)
	api.Get("/employer/jobs", mw.cookie, h.ListVacancies)
	api.Post("/employer/jobs", mw.cookie, h.CreateVacancy)
	api.Patch("/employer/jobs/:slug", mw.cookie, h.UpdateVacancy)
	api.Post("/employer/jobs/:slug/close", mw.cookie, h.CloseVacancy)

	// Moderator review queue for a claim the domain check could not itself verify.
	api.Get("/employer/claims", mw.cookie, mw.moderator, h.ListPendingClaims)
	api.Post("/employer/claims/:user_id/approve", mw.cookie, mw.moderator, h.ApproveClaim)
	api.Post("/employer/claims/:user_id/reject", mw.cookie, mw.moderator, h.RejectClaim)
	// Revocation is an admin action, not a moderator one — see employer-account's spec.
	api.Post("/employer/claims/:user_id/revoke", mw.cookie, h.adminOnly, h.RevokeAccount)
}

// employerError maps the employer package's sentinels onto HTTP statuses, and the
// accounts code-flow sentinels ConfirmClaim/Claim can surface too (ConfirmClaim delegates
// straight to accounts.Service.ConfirmCode; Claim's own ErrMailUnavailable check runs
// first, but accounts.ErrMailUnavailable is mapped too for completeness — same shape
// recoveryError uses for /auth/verify). employer.ErrInvalid carries a user-facing message
// surfaced in the 400 body; anything else falls through to RenderError as a 500.
func employerError(err error) error {
	switch {
	case errors.Is(err, employer.ErrInvalid):
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	case errors.Is(err, employer.ErrPublicWebmailDomain):
		return fiber.NewError(fiber.StatusForbidden, "work email must be at a company domain, not a public email provider")
	case errors.Is(err, employer.ErrNotActive):
		return fiber.NewError(fiber.StatusForbidden, "employer account is not active")
	case errors.Is(err, employer.ErrNotFound):
		return fiber.NewError(fiber.StatusNotFound, "no employer account for this user")
	case errors.Is(err, employer.ErrAlreadyHasAccount):
		return fiber.NewError(fiber.StatusConflict, "this user already has an employer account")
	case errors.Is(err, employer.ErrCompanyAlreadyClaimed):
		return fiber.NewError(fiber.StatusConflict, "this company is already claimed")
	case errors.Is(err, employer.ErrClaimNotPending):
		return fiber.NewError(fiber.StatusConflict, "no pending claim for this user")
	case errors.Is(err, employer.ErrJobNotFound):
		return fiber.NewError(fiber.StatusNotFound, "vacancy not found")
	case errors.Is(err, employer.ErrURLTaken):
		return fiber.NewError(fiber.StatusConflict, "this URL is already used by another employer's vacancy")
	case errors.Is(err, employer.ErrMailUnavailable), errors.Is(err, accounts.ErrMailUnavailable):
		return fiber.NewError(fiber.StatusServiceUnavailable, "email delivery is not configured")
	case errors.Is(err, accounts.ErrInvalidCode):
		return fiber.NewError(fiber.StatusBadRequest, "invalid or already used code")
	case errors.Is(err, accounts.ErrCodeExpired):
		return fiber.NewError(fiber.StatusBadRequest, "this code has expired — request a new one")
	case errors.Is(err, accounts.ErrResendTooSoon):
		return fiber.NewError(fiber.StatusTooManyRequests, "a code was just sent — check your inbox")
	default:
		return err
	}
}

// employerAccountResponse is the public shape of a company_accounts row. UserID IS
// included — unlike most ownership ids, this one is the address the moderator/admin action
// endpoints (approve/reject/revoke) take (:user_id), so the moderator queue needs it back to
// act on a row at all. It costs nothing on the self-service reads: the caller already knows
// it is their own, the same way a job's own internal id costs nothing being in jobview.Job.
type employerAccountResponse struct {
	UserID      int64      `json:"user_id"`
	CompanySlug string     `json:"company_slug"`
	CompanyName string     `json:"company_name"`
	WorkEmail   string     `json:"work_email"`
	Status      string     `json:"status"`
	VerifiedAt  *time.Time `json:"verified_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

func toEmployerAccountResponse(a employer.Account) employerAccountResponse {
	return employerAccountResponse{
		UserID:      a.UserID,
		CompanySlug: a.CompanySlug,
		CompanyName: a.CompanyName,
		WorkEmail:   a.WorkEmail,
		Status:      a.Status,
		VerifiedAt:  a.VerifiedAt,
		CreatedAt:   a.CreatedAt,
	}
}

// employerClaimRequest is the claim body: a company name (resolved against the existing
// catalogue or normalized fresh — see employer.Service.Claim) and the work email to verify.
type employerClaimRequest struct {
	CompanyName string `json:"company_name"`
	WorkEmail   string `json:"work_email"`
}

// Claim starts a company-account claim: resolves the company slug, refuses a public-webmail
// work email, reserves the slug, and mails a verification code. Returns the pending account
// with 201.
func (h *employerHandlers) Claim(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in employerClaimRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	acc, err := h.employer.Claim(c.Context(), userID, in.CompanyName, in.WorkEmail)
	if err != nil {
		return employerError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": toEmployerAccountResponse(acc)})
}

type employerConfirmRequest struct {
	Code string `json:"code"`
}

// ConfirmClaim consumes the mailed code. The account activates immediately when the work
// email's domain matches the company's already-known website; otherwise it stays pending,
// visible to moderators. Either way the response is 200 with the (possibly still pending)
// account.
func (h *employerHandlers) ConfirmClaim(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in employerConfirmRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	acc, err := h.employer.ConfirmClaim(c.Context(), userID, in.Code)
	if err != nil {
		return employerError(err)
	}
	return c.JSON(fiber.Map{"data": toEmployerAccountResponse(acc)})
}

// employerCompanyResponse combines the caller's account state with their company's current
// curated profile — never the job-derived facets (company_types/company_sizes etc.), which
// this surface does not expose for editing.
type employerCompanyResponse struct {
	employerAccountResponse
	Tagline       string   `json:"tagline,omitempty"`
	Description   string   `json:"description,omitempty"`
	Website       string   `json:"website,omitempty"`
	Industries    []string `json:"industries,omitempty"`
	YearFounded   *int     `json:"year_founded,omitempty"`
	EmployeeCount *int     `json:"employee_count,omitempty"`
	HqCountry     string   `json:"hq_country,omitempty"`
	Subindustry   string   `json:"subindustry,omitempty"`
}

// companyInfoField reads a string key out of a company's company_info JSONB, "" for a
// missing key, a non-string value, an absent company_info, or unreadable JSON — matching
// the same never-an-error-just-absent stance internal/ingest/employer's own websiteOf
// takes. map[string]any (not map[string]string): company_info also carries non-string
// values under other keys (e.g. a subsidiaries array), and unmarshalling into a
// string-valued map would fail the WHOLE object — and so read every key as absent,
// including description/website — the moment any other key in it is not a string.
func companyInfoField(raw []byte, key string) string {
	if len(raw) == 0 {
		return ""
	}
	var fields map[string]any
	if err := json.Unmarshal(raw, &fields); err != nil {
		return ""
	}
	s, _ := fields[key].(string)
	return s
}

// toEmployerCompanyResponse combines the account with its company's curated profile. found
// is false when the company has no row yet (a claim on a brand-new company before its first
// SeedCompanyWebsite/is_reference insert) — the profile fields simply stay at their zero
// value, same as a company row with every curated field still unset.
func toEmployerCompanyResponse(acc employer.Account, company db.Company, found bool) employerCompanyResponse {
	resp := employerCompanyResponse{employerAccountResponse: toEmployerAccountResponse(acc)}
	if !found {
		return resp
	}
	resp.Tagline = company.Tagline.String
	resp.Description = companyInfoField(company.CompanyInfo, "description")
	resp.Website = companyInfoField(company.CompanyInfo, "website")
	resp.Industries = company.Industries
	if company.YearFounded.Valid {
		v := int(company.YearFounded.Int32)
		resp.YearFounded = &v
	}
	if company.EmployeeCount.Valid {
		v := int(company.EmployeeCount.Int32)
		resp.EmployeeCount = &v
	}
	resp.HqCountry = company.HqCountry.String
	resp.Subindustry = company.Subindustry.String
	return resp
}

// GetCompany returns the caller's own account (whatever status it is in) plus their
// company's current curated profile. Deliberately NOT gated on ActiveAccount, unlike every
// write in this file: a caller with a pending or revoked claim still needs to read their own
// status — it is the one read the claim-status page and the dashboard's own "am I active yet"
// check both need before there is anything to edit. Reads companies directly (no
// employer-package round trip needed for a plain read).
func (h *employerHandlers) GetCompany(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	acc, err := h.employer.MyAccount(c.Context(), userID)
	if err != nil {
		return employerError(err)
	}
	company, err := h.queries.GetCompany(c.Context(), acc.CompanySlug)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	return c.JSON(fiber.Map{"data": toEmployerCompanyResponse(acc, company, err == nil)})
}

// employerCompanyProfileRequest is the curated-profile PATCH body. A nil/absent field is
// left unchanged (see employer.CompanyProfilePatch); Industries replaces the whole curated
// set when present, even as an empty array.
type employerCompanyProfileRequest struct {
	Tagline       *string  `json:"tagline"`
	Description   *string  `json:"description"`
	Website       *string  `json:"website"`
	Industries    []string `json:"industries"`
	YearFounded   *int     `json:"year_founded"`
	EmployeeCount *int     `json:"employee_count"`
	HqCountry     *string  `json:"hq_country"`
	Subindustry   *string  `json:"subindustry"`
}

// UpdateCompany applies the caller's authoritative edit to their own company's curated
// profile. Active-account-only, like every other employer action.
func (h *employerHandlers) UpdateCompany(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in employerCompanyProfileRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	if _, err := h.employer.UpdateCompanyProfile(c.Context(), userID, employer.CompanyProfilePatch{
		Tagline:       in.Tagline,
		Description:   in.Description,
		Website:       in.Website,
		Industries:    in.Industries,
		YearFounded:   in.YearFounded,
		EmployeeCount: in.EmployeeCount,
		HqCountry:     in.HqCountry,
		Subindustry:   in.Subindustry,
	}); err != nil {
		return employerError(err)
	}
	return h.GetCompany(c)
}

// employerVacancyRequest is the create-vacancy body. url and title are required (validated
// by moderation.CreateInput inside employer.Service.CreateVacancy); company is deliberately
// absent — it is always the account's own locked company name.
type employerVacancyRequest struct {
	URL         string     `json:"url"`
	Title       string     `json:"title"`
	Location    string     `json:"location"`
	Remote      bool       `json:"remote"`
	Description string     `json:"description"`
	PostedAt    *time.Time `json:"posted_at"`

	Skills         []string `json:"skills"`
	Regions        []string `json:"regions"`
	Cities         []string `json:"cities"`
	WorkMode       string   `json:"work_mode"`
	EmploymentType string   `json:"employment_type"`
	Seniority      string   `json:"seniority"`
	SalaryMin      *int     `json:"salary_min"`
	SalaryMax      *int     `json:"salary_max"`
	SalaryCurrency string   `json:"salary_currency"`
	SalaryPeriod   string   `json:"salary_period"`
}

func (r employerVacancyRequest) toVacancyInput() employer.VacancyInput {
	return employer.VacancyInput{
		URL:            r.URL,
		Title:          r.Title,
		Location:       r.Location,
		Remote:         r.Remote,
		Description:    r.Description,
		PostedAt:       r.PostedAt,
		Skills:         r.Skills,
		Regions:        r.Regions,
		Cities:         r.Cities,
		WorkMode:       r.WorkMode,
		EmploymentType: r.EmploymentType,
		Seniority:      r.Seniority,
		SalaryMin:      r.SalaryMin,
		SalaryMax:      r.SalaryMax,
		SalaryCurrency: r.SalaryCurrency,
		SalaryPeriod:   r.SalaryPeriod,
	}
}

// ListVacancies returns every vacancy the caller has published through this path, newest
// first — the dashboard's own list. Active-account-only.
func (h *employerHandlers) ListVacancies(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	jobs, extras, err := h.employer.ListVacancies(c.Context(), userID)
	if err != nil {
		return employerError(err)
	}
	out := make([]jobview.Job, len(jobs))
	for i := range jobs {
		view, err := jobview.FromDomain(jobs[i], extras[i])
		if err != nil {
			return err
		}
		out[i] = view
	}
	return c.JSON(fiber.Map{"data": out})
}

// CreateVacancy publishes a new vacancy for the caller's own claimed company, or
// idempotently updates/reopens one the caller already owns at the same URL — see
// employer.Service.CreateVacancy. A URL already owned by a different employer is a 409.
func (h *employerHandlers) CreateVacancy(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in employerVacancyRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	domainJob, extras, err := h.employer.CreateVacancy(c.Context(), userID, in.toVacancyInput())
	if err != nil {
		return employerError(err)
	}
	view, err := jobview.FromDomain(domainJob, extras)
	if err != nil {
		return err
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": view})
}

// employerVacancyPatchRequest is the edit body: a nil field is left unchanged. The URL and
// company identity are not present — neither is editable.
type employerVacancyPatchRequest struct {
	Title       *string    `json:"title"`
	Location    *string    `json:"location"`
	Remote      *bool      `json:"remote"`
	Description *string    `json:"description"`
	PostedAt    *time.Time `json:"posted_at"`
}

// UpdateVacancy edits a vacancy the caller's own account owns, addressed by public slug.
// Any other slug (missing, another owner's, another source's) is a 404.
func (h *employerHandlers) UpdateVacancy(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in employerVacancyPatchRequest
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid request body")
	}
	domainJob, extras, err := h.employer.UpdateVacancy(c.Context(), userID, c.Params("slug"), employer.VacancyPatch{
		Title:       in.Title,
		Location:    in.Location,
		Remote:      in.Remote,
		Description: in.Description,
		PostedAt:    in.PostedAt,
	})
	if err != nil {
		return employerError(err)
	}
	view, err := jobview.FromDomain(domainJob, extras)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": view})
}

// CloseVacancy soft-closes a vacancy the caller's own account owns.
func (h *employerHandlers) CloseVacancy(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	if err := h.employer.CloseVacancy(c.Context(), userID, c.Params("slug")); err != nil {
		return employerError(err)
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"closed": true}})
}

// ListPendingClaims is the moderator review queue: every claim that could not
// auto-activate. Role-gated, so reaching this handler already implies a moderator.
func (h *employerHandlers) ListPendingClaims(c *fiber.Ctx) error {
	rows, err := h.employer.ListPendingClaims(c.Context())
	if err != nil {
		return err
	}
	out := make([]employerAccountResponse, len(rows))
	for i, r := range rows {
		out[i] = toEmployerAccountResponse(r)
	}
	return c.JSON(fiber.Map{"data": out})
}

// employerUserID parses the ":user_id" route param — the moderator/admin routes address a
// company_accounts row by its primary key, not a synthetic "id".
func employerUserID(c *fiber.Ctx) (int64, error) {
	id, err := c.ParamsInt("user_id")
	if err != nil {
		return 0, fiber.NewError(fiber.StatusBadRequest, "invalid user_id")
	}
	return int64(id), nil
}

// ApproveClaim activates a pending claim and, when the company's website was still blank,
// seeds it from the claim's confirmed work-email domain. Role-gated.
func (h *employerHandlers) ApproveClaim(c *fiber.Ctx) error {
	userID, err := employerUserID(c)
	if err != nil {
		return err
	}
	acc, err := h.employer.ApproveClaim(c.Context(), userID)
	if err != nil {
		return employerError(err)
	}
	return c.JSON(fiber.Map{"data": toEmployerAccountResponse(acc)})
}

// RejectClaim removes a pending claim, freeing its company slug. Role-gated.
func (h *employerHandlers) RejectClaim(c *fiber.Ctx) error {
	userID, err := employerUserID(c)
	if err != nil {
		return err
	}
	if err := h.employer.RejectClaim(c.Context(), userID); err != nil {
		return employerError(err)
	}
	return c.JSON(fiber.Map{"data": fiber.Map{"rejected": true}})
}

// RevokeAccount is the admin kill switch: it deactivates the account but keeps the row (and
// the company slug reservation) — see migrations/0174.
func (h *employerHandlers) RevokeAccount(c *fiber.Ctx) error {
	userID, err := employerUserID(c)
	if err != nil {
		return err
	}
	acc, err := h.employer.RevokeAccount(c.Context(), userID)
	if err != nil {
		return employerError(err)
	}
	return c.JSON(fiber.Map{"data": toEmployerAccountResponse(acc)})
}
