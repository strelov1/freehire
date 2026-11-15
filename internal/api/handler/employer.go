package handler

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/go-faster/jx"
	"github.com/gofiber/fiber/v2"
	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/identity/accounts"
	"github.com/strelov1/freehire/internal/identity/auth"
	"github.com/strelov1/freehire/internal/ingest/applyform"
	"github.com/strelov1/freehire/internal/ingest/directapply"
	"github.com/strelov1/freehire/internal/ingest/employer"
	"github.com/strelov1/freehire/internal/job/job"
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
	// inbox is the applicant inbox over applications sent on freehire; nil when the
	// deployment has no mail transport, since moving an application may email the
	// candidate. Its routes then answer 501 (employer_applications.go).
	inbox *directapply.Inbox
	// adminOnly lives here, not in the shared middleware: revoke is the only admin-gated route.
	adminOnly fiber.Handler
}

func newEmployerHandlers(queries *db.Queries, svc *employer.Service) *employerHandlers {
	return &employerHandlers{
		queries:   queries,
		employer:  svc,
		adminOnly: auth.RequireRole(queries, "admin"),
	}
}

func (h *employerHandlers) register(api fiber.Router, mw middleware, contract fiber.Handler) {
	// Cookie-only: employer actions are not open to API keys. Every route below requires
	// ActiveAccount except the claim/confirm/resend trio, which is how an account BECOMES active.
	api.Post("/employer/claim", mw.cookie, contract)
	api.Post("/employer/claim/confirm", mw.cookie, contract)
	api.Post("/employer/claim/resend", mw.cookie, contract)
	api.Get("/employer/company", mw.cookie, contract)
	api.Patch("/employer/company", mw.cookie, contract)
	api.Get("/employer/jobs", mw.cookie, contract)
	api.Post("/employer/jobs", mw.cookie, contract)
	api.Patch("/employer/jobs/:slug", mw.cookie, contract)
	api.Post("/employer/jobs/:slug/close", mw.cookie, contract)
	api.Post("/employer/jobs/:slug/reopen", mw.cookie, contract)
	api.Get("/employer/applications", mw.cookie, contract)
	api.Get("/employer/applications/:id/cv", mw.cookie, contract)
	api.Post("/employer/applications/:id/stage", mw.cookie, contract)
	api.Get("/employer/team", mw.cookie, contract)
	api.Post("/employer/team/invitations", mw.cookie, contract)
	api.Delete("/employer/team/invitations/:id", mw.cookie, contract)
	api.Delete("/employer/team/members/:user_id", mw.cookie, contract)
	api.Post("/employer/team/invitation-preview", mw.cookie, contract)
	api.Post("/employer/team/join", mw.cookie, contract)

	// Moderator review queue for a claim the domain check could not itself verify.
	api.Get("/employer/claims", mw.cookie, mw.moderator, contract)
	api.Post("/employer/claims/:user_id/approve", mw.cookie, mw.moderator, contract)
	api.Post("/employer/claims/:user_id/reject", mw.cookie, mw.moderator, contract)
	// Revocation is an admin action, not a moderator one.
	api.Post("/employer/claims/:user_id/revoke", mw.cookie, h.adminOnly, contract)
}

// employerError maps the employer package's sentinels, plus the accounts code-flow
// sentinels ConfirmClaim/Claim can surface (same shape as recoveryError), onto HTTP
// statuses. employer.ErrInvalid carries a user-facing message for the 400 body; anything
// else falls through to RenderError as a 500.
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
	case errors.Is(err, employer.ErrEmailAlreadyConfirmed):
		return fiber.NewError(fiber.StatusConflict, "work email is already confirmed")
	case errors.Is(err, employer.ErrJobNotFound):
		return fiber.NewError(fiber.StatusNotFound, "vacancy not found")
	case errors.Is(err, employer.ErrInvitationInvalid):
		return fiber.NewError(fiber.StatusNotFound, "this invitation link is invalid or has expired")
	case errors.Is(err, employer.ErrMemberNotFound):
		return fiber.NewError(fiber.StatusNotFound, "no such team member")
	case errors.Is(err, employer.ErrInvitationNotFound):
		return fiber.NewError(fiber.StatusNotFound, "no such invitation")
	case errors.Is(err, employer.ErrTooManyInvitations):
		return fiber.NewError(fiber.StatusTooManyRequests, "too many open invitations — withdraw one first")
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

// employerAccountOut is the public shape of a company_accounts row. UserID IS included,
// unlike most ownership ids: the moderator/admin endpoints (approve/reject/revoke) address
// a row by it. It leaks nothing on self-service reads, where the caller knows it is their own.
func employerAccountOut(a employer.Account) apigen.EmployerAccount {
	out := apigen.EmployerAccount{
		UserID:         a.UserID,
		CompanySlug:    a.CompanySlug,
		CompanyName:    a.CompanyName,
		WorkEmail:      a.WorkEmail,
		Status:         apigen.EmployerAccountStatus(a.Status),
		EmailConfirmed: a.EmailConfirmed(),
		CreatedAt:      a.CreatedAt,
		Member:         a.Member,
	}
	if a.VerifiedAt != nil {
		out.VerifiedAt = apigen.NewOptDateTime(*a.VerifiedAt)
	}
	return out
}

func employerAccountsOut(rows []employer.Account) []apigen.EmployerAccount {
	out := make([]apigen.EmployerAccount, len(rows))
	for i, r := range rows {
		out[i] = employerAccountOut(r)
	}
	return out
}

// ClaimEmployerCompany starts a company-account claim: resolves the company slug, refuses a
// public-webmail work email, reserves the slug, and mails a verification code. Returns the
// pending account with 201.
func (h *employerHandlers) ClaimEmployerCompany(ctx context.Context, req *apigen.EmployerClaim) (*apigen.ClaimEmployerCompanyCreated, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	acc, err := h.employer.Claim(ctx, userID, req.CompanyName.Or(""), req.WorkEmail.Or(""))
	if err != nil {
		return nil, employerError(err)
	}
	return &apigen.ClaimEmployerCompanyCreated{Data: employerAccountOut(acc)}, nil
}

// ConfirmEmployerClaim consumes the mailed code. The account activates immediately when the
// work email's domain matches the company's already-known website; otherwise it stays
// pending, visible to moderators. Either way the response is 200 with the (possibly still
// pending) account.
func (h *employerHandlers) ConfirmEmployerClaim(ctx context.Context, req *apigen.EmployerConfirm) (*apigen.ConfirmEmployerClaimOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	acc, err := h.employer.ConfirmClaim(ctx, userID, req.Code.Or(""))
	if err != nil {
		return nil, employerError(err)
	}
	return &apigen.ConfirmEmployerClaimOK{Data: employerAccountOut(acc)}, nil
}

// ResendEmployerClaimCode mails a fresh work-email code for the caller's still-unconfirmed
// claim. 409 when there is no pending claim or its email is already confirmed; 429 inside
// the resend cooldown.
func (h *employerHandlers) ResendEmployerClaimCode(ctx context.Context) (*apigen.ResendEmployerClaimCodeOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	acc, err := h.employer.ResendClaimCode(ctx, userID)
	if err != nil {
		return nil, employerError(err)
	}
	return &apigen.ResendEmployerClaimCodeOK{Data: employerAccountOut(acc)}, nil
}

// companyInfoField reads a string key out of a company's company_info JSONB, "" for a
// missing key, a non-string value, an absent company_info, or unreadable JSON (like
// internal/ingest/employer's websiteOf). It decodes into map[string]any because other keys
// hold non-string values (e.g. subsidiaries), and a map[string]string would fail the WHOLE
// object and read every key as absent.
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

// employerCompanyOut combines the account with its company's curated profile, never the
// job-derived facets (company_types/company_sizes etc.). found is false when the company
// has no row yet (a claim on a brand-new company); the profile fields then stay unset.
func employerCompanyOut(acc employer.Account, company db.Company, found bool) apigen.EmployerCompany {
	a := employerAccountOut(acc)
	out := apigen.EmployerCompany{
		UserID: a.UserID, CompanySlug: a.CompanySlug, CompanyName: a.CompanyName, WorkEmail: a.WorkEmail,
		Status: a.Status, VerifiedAt: a.VerifiedAt, EmailConfirmed: a.EmailConfirmed, CreatedAt: a.CreatedAt,
		Member: a.Member,
	}
	if !found {
		return out
	}
	out.Tagline = optString(company.Tagline.String)
	out.Description = optString(companyInfoField(company.CompanyInfo, "description"))
	out.Website = optString(companyInfoField(company.CompanyInfo, "website"))
	out.Industries = nilIfEmpty(company.Industries)
	if company.YearFounded.Valid {
		out.YearFounded = apigen.NewOptInt64(int64(company.YearFounded.Int32))
	}
	if company.EmployeeCount.Valid {
		out.EmployeeCount = apigen.NewOptInt64(int64(company.EmployeeCount.Int32))
	}
	out.HqCountry = optString(company.HqCountry.String)
	out.Subindustry = optString(company.Subindustry.String)
	return out
}

// GetEmployerCompany returns the caller's own account (whatever status it is in) plus their
// company's current curated profile. Deliberately NOT gated on ActiveAccount, unlike every
// write here: a pending or revoked claim still needs to read its own status.
func (h *employerHandlers) GetEmployerCompany(ctx context.Context) (*apigen.GetEmployerCompanyOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	company, err := h.companyFor(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &apigen.GetEmployerCompanyOK{Data: company}, nil
}

func (h *employerHandlers) companyFor(ctx context.Context, userID int64) (apigen.EmployerCompany, error) {
	acc, err := h.employer.MyAccount(ctx, userID)
	if err != nil {
		return apigen.EmployerCompany{}, employerError(err)
	}
	company, err := h.queries.GetCompany(ctx, acc.CompanySlug)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return apigen.EmployerCompany{}, err
	}
	return employerCompanyOut(acc, company, err == nil), nil
}

// UpdateEmployerCompany applies the caller's authoritative edit to their own company's
// curated profile. Active-account-only, like every other employer action. An absent or null
// field is left unchanged (see employer.CompanyProfilePatch); Industries replaces the whole
// curated set when present, even as an empty array.
func (h *employerHandlers) UpdateEmployerCompany(ctx context.Context, req *apigen.EmployerCompanyPatch) (*apigen.UpdateEmployerCompanyOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if _, err := h.employer.UpdateCompanyProfile(ctx, userID, employer.CompanyProfilePatch{
		Tagline:       ptrOf(req.Tagline.Get),
		Description:   ptrOf(req.Description.Get),
		Website:       ptrOf(req.Website.Get),
		Industries:    req.Industries.Value,
		YearFounded:   intPtrOf(req.YearFounded.Get),
		EmployeeCount: intPtrOf(req.EmployeeCount.Get),
		HqCountry:     ptrOf(req.HqCountry.Get),
		Subindustry:   ptrOf(req.Subindustry.Get),
	}); err != nil {
		return nil, employerError(err)
	}
	company, err := h.companyFor(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &apigen.UpdateEmployerCompanyOK{Data: company}, nil
}

func toEmployerQuestions(in []apigen.EmployerQuestion) []applyform.EmployerQuestion {
	out := make([]applyform.EmployerQuestion, len(in))
	for i, q := range in {
		out[i] = applyform.EmployerQuestion{Label: q.Label.Or(""), Type: applyform.FieldType(q.Type.Or("")), Required: q.Required.Or(false), Options: q.Options}
	}
	return out
}

// vacancyInput reads the create-vacancy body. title is required, and url too unless
// apply_mode is "freehire" (absent means "external"); questions belong to a freehire
// vacancy only. Validated inside employer.Service.CreateVacancy. company is deliberately
// absent — it is always the account's own locked company name.
func vacancyInput(r *apigen.EmployerVacancyCreate) employer.VacancyInput {
	return employer.VacancyInput{
		ApplyMode:      employer.ApplyMode(r.ApplyMode.Or("")),
		Questions:      toEmployerQuestions(r.Questions),
		URL:            r.URL.Or(""),
		Title:          r.Title.Or(""),
		Location:       r.Location.Or(""),
		Remote:         r.Remote.Or(false),
		Description:    r.Description.Or(""),
		PostedAt:       ptrOf(r.PostedAt.Get),
		Skills:         r.Skills,
		Regions:        r.Regions,
		Cities:         r.Cities,
		WorkMode:       r.WorkMode.Or(""),
		EmploymentType: r.EmploymentType.Or(""),
		Seniority:      r.Seniority.Or(""),
		SalaryMin:      intPtrOf(r.SalaryMin.Get),
		SalaryMax:      intPtrOf(r.SalaryMax.Get),
		SalaryCurrency: r.SalaryCurrency.Or(""),
		SalaryPeriod:   r.SalaryPeriod.Or(""),
	}
}

// vacancyRaw is a vacancy as the shared job view, sent through as the bytes it marshals
// to: its shape is jobview's and search's, not this group's.
func vacancyRaw(j job.Job, extras job.Extras) (jx.Raw, error) {
	view, err := jobview.FromDomain(j, extras)
	if err != nil {
		return nil, err
	}
	return json.Marshal(view)
}

// ListEmployerVacancies returns every vacancy the caller has published through this path,
// newest first — the dashboard's own list. Active-account-only.
func (h *employerHandlers) ListEmployerVacancies(ctx context.Context) (*apigen.ListEmployerVacanciesOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	jobs, extras, err := h.employer.ListVacancies(ctx, userID)
	if err != nil {
		return nil, employerError(err)
	}
	out := make([]jx.Raw, len(jobs))
	for i := range jobs {
		if out[i], err = vacancyRaw(jobs[i], extras[i]); err != nil {
			return nil, err
		}
	}
	return &apigen.ListEmployerVacanciesOK{Data: out}, nil
}

// CreateEmployerVacancy publishes a new vacancy for the caller's own claimed company, or
// idempotently updates/reopens one the caller already owns at the same URL — see
// employer.Service.CreateVacancy. A URL already owned by a different employer is a 409.
func (h *employerHandlers) CreateEmployerVacancy(ctx context.Context, req *apigen.EmployerVacancyCreate) (*apigen.CreateEmployerVacancyCreated, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	domainJob, extras, err := h.employer.CreateVacancy(ctx, userID, vacancyInput(req))
	if err != nil {
		return nil, employerError(err)
	}
	raw, err := vacancyRaw(domainJob, extras)
	if err != nil {
		return nil, err
	}
	return &apigen.CreateEmployerVacancyCreated{Data: raw}, nil
}

// UpdateEmployerVacancy edits a vacancy the caller's own account owns, addressed by public
// slug. Any other slug (missing, another owner's, another source's) is a 404. An absent or
// null field is left unchanged. The URL and company identity are not editable; apply_mode may
// only restate the vacancy's mode; questions replace a freehire vacancy's questions.
func (h *employerHandlers) UpdateEmployerVacancy(ctx context.Context, req *apigen.EmployerVacancyPatch, params apigen.UpdateEmployerVacancyParams) (*apigen.UpdateEmployerVacancyOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	patch := employer.VacancyPatch{
		Title:       ptrOf(req.Title.Get),
		Location:    ptrOf(req.Location.Get),
		Remote:      ptrOf(req.Remote.Get),
		Description: ptrOf(req.Description.Get),
		PostedAt:    ptrOf(req.PostedAt.Get),
	}
	if mode, ok := req.ApplyMode.Get(); ok {
		m := employer.ApplyMode(mode)
		patch.ApplyMode = &m
	}
	if in, ok := req.Questions.Get(); ok {
		qs := toEmployerQuestions(in)
		patch.Questions = &qs
	}
	domainJob, extras, err := h.employer.UpdateVacancy(ctx, userID, params.Slug, patch)
	if err != nil {
		return nil, employerError(err)
	}
	raw, err := vacancyRaw(domainJob, extras)
	if err != nil {
		return nil, err
	}
	return &apigen.UpdateEmployerVacancyOK{Data: raw}, nil
}

// CloseEmployerVacancy soft-closes a vacancy the caller's own account owns.
func (h *employerHandlers) CloseEmployerVacancy(ctx context.Context, params apigen.CloseEmployerVacancyParams) (*apigen.CloseEmployerVacancyOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.employer.CloseVacancy(ctx, userID, params.Slug); err != nil {
		return nil, employerError(err)
	}
	return &apigen.CloseEmployerVacancyOK{Data: apigen.EmployerClosed{Closed: true}}, nil
}

// ReopenEmployerVacancy undoes the caller's own close of their vacancy — the only way back
// for a vacancy taking applications on freehire, which has no URL to re-create under.
func (h *employerHandlers) ReopenEmployerVacancy(ctx context.Context, params apigen.ReopenEmployerVacancyParams) (*apigen.ReopenEmployerVacancyOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.employer.ReopenVacancy(ctx, userID, params.Slug); err != nil {
		return nil, employerError(err)
	}
	return &apigen.ReopenEmployerVacancyOK{Data: apigen.EmployerClosed{Closed: false}}, nil
}

// ListEmployerPendingClaims is the moderator review queue: every claim that could not
// auto-activate. Role-gated, so reaching this handler already implies a moderator.
func (h *employerHandlers) ListEmployerPendingClaims(ctx context.Context) (*apigen.ListEmployerPendingClaimsOK, error) {
	rows, err := h.employer.ListPendingClaims(ctx)
	if err != nil {
		return nil, err
	}
	return &apigen.ListEmployerPendingClaimsOK{Data: employerAccountsOut(rows)}, nil
}

// employerUserID parses the "user_id" route param — the moderator/admin routes address a
// company_accounts row by its primary key, not a synthetic "id".
func employerUserID(raw string) (int64, error) {
	id, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fiber.NewError(fiber.StatusBadRequest, "invalid user_id")
	}
	return int64(id), nil
}

// ApproveEmployerClaim activates a pending claim and, when the company's website was still
// blank, seeds it from the claim's confirmed work-email domain. Role-gated.
func (h *employerHandlers) ApproveEmployerClaim(ctx context.Context, params apigen.ApproveEmployerClaimParams) (*apigen.ApproveEmployerClaimOK, error) {
	userID, err := employerUserID(params.UserID)
	if err != nil {
		return nil, err
	}
	acc, err := h.employer.ApproveClaim(ctx, userID)
	if err != nil {
		return nil, employerError(err)
	}
	return &apigen.ApproveEmployerClaimOK{Data: employerAccountOut(acc)}, nil
}

// RejectEmployerClaim removes a pending claim, freeing its company slug. Role-gated.
func (h *employerHandlers) RejectEmployerClaim(ctx context.Context, params apigen.RejectEmployerClaimParams) (*apigen.RejectEmployerClaimOK, error) {
	userID, err := employerUserID(params.UserID)
	if err != nil {
		return nil, err
	}
	if err := h.employer.RejectClaim(ctx, userID); err != nil {
		return nil, employerError(err)
	}
	return &apigen.RejectEmployerClaimOK{Data: apigen.EmployerRejected{Rejected: true}}, nil
}

// RevokeEmployerAccount is the admin kill switch: it deactivates the account but keeps the
// row (and the company slug reservation) — see migrations/0174.
func (h *employerHandlers) RevokeEmployerAccount(ctx context.Context, params apigen.RevokeEmployerAccountParams) (*apigen.RevokeEmployerAccountOK, error) {
	userID, err := employerUserID(params.UserID)
	if err != nil {
		return nil, err
	}
	acc, err := h.employer.RevokeAccount(ctx, userID)
	if err != nil {
		return nil, employerError(err)
	}
	return &apigen.RevokeEmployerAccountOK{Data: employerAccountOut(acc)}, nil
}
