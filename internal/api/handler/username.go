package handler

import (
	"context"

	"github.com/strelov1/freehire/internal/api/apigen"
)

// CheckUsername reports whether a candidate username could be claimed right
// now (valid format, not reserved, not already held). Public — usable while
// picking a name before committing to a claim, or before the caller is even
// signed in.
func (h *authHandlers) CheckUsername(ctx context.Context, params apigen.CheckUsernameParams) (*apigen.CheckUsernameOK, error) {
	available, err := h.accounts.UsernameAvailable(ctx, params.Value.Value)
	if err != nil {
		return nil, err
	}
	return &apigen.CheckUsernameOK{Data: apigen.AccountUsernameAvailability{Available: available}}, nil
}

// accountUsername reads the caller's current username state. Username is null until
// the caller has claimed or been lazily allocated one; UpdatedAt is null for a lazy
// default that was never explicitly claimed.
func (h *authHandlers) accountUsername(ctx context.Context, userID int64) (apigen.AccountUsername, error) {
	name, updatedAt, ok, err := h.accounts.Username(ctx, userID)
	if err != nil {
		return apigen.AccountUsername{}, err
	}
	out := apigen.AccountUsername{Username: nilString(nil), UpdatedAt: nilDateTime(updatedAt)}
	if ok {
		out.Username = nilString(&name)
	}
	return out, nil
}

// GetUsername returns the caller's own username (null until claimed/allocated).
// Cookie-only, same reasoning as the other /me reads: it is the caller's own
// account state.
func (h *authHandlers) GetUsername(ctx context.Context) (*apigen.GetUsernameOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	out, err := h.accountUsername(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &apigen.GetUsernameOK{Data: out}, nil
}

// ClaimUsername claims or changes the caller's username. Cookie-only.
func (h *authHandlers) ClaimUsername(ctx context.Context, req *apigen.AccountUsernameClaim) (*apigen.ClaimUsernameOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	if err := h.accounts.ClaimUsername(ctx, userID, req.Username.Value); err != nil {
		return nil, accountsError(err)
	}
	out, err := h.accountUsername(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &apigen.ClaimUsernameOK{Data: out}, nil
}
