package handler

import (
	"context"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/ingest/employer"
)

// ClaimEmployerCompany starts a company-account claim: resolves the company slug, refuses a
// public-webmail work email, reserves the slug, and mails a verification code. Returns the
// pending account with 201.
func (h *employerHandlers) ClaimEmployerCompany(ctx context.Context, req *apigen.EmployerClaim) (*apigen.ClaimEmployerCompanyCreated, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	var acc employer.Account
	if slug := req.CompanySlug.Or(""); slug != "" {
		acc, err = h.employer.ClaimBySlug(ctx, userID, slug, req.WorkEmail.Or(""))
	} else {
		acc, err = h.employer.Claim(ctx, userID, req.CompanyName.Or(""), req.WorkEmail.Or(""))
	}
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
