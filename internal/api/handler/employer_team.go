package handler

import (
	"context"

	"github.com/strelov1/freehire/internal/api/apigen"
	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/ingest/employer"
)

// The company team (internal/ingest/employer, team.go): who acts for the company, the
// invitations that bring colleagues in, and joining by an invitation link. Cookie-only like
// every employer route.

// GetEmployerTeam lists the caller's company team and its open invitations.
func (h *employerHandlers) GetEmployerTeam(ctx context.Context) (*apigen.GetEmployerTeamOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	team, err := h.employer.Team(ctx, userID)
	if err != nil {
		return nil, employerError(err)
	}
	out := apigen.EmployerTeam{
		Members:     make([]apigen.EmployerTeamMember, len(team.Members)),
		Invitations: make([]apigen.EmployerTeamInvitation, len(team.Invitations)),
	}
	for i, m := range team.Members {
		out.Members[i] = apigen.EmployerTeamMember{UserID: m.UserID, WorkEmail: m.WorkEmail, JoinedAt: m.JoinedAt, Holder: m.Holder}
	}
	for i, inv := range team.Invitations {
		out.Invitations[i] = employerInvitationOut(inv)
	}
	return &apigen.GetEmployerTeamOK{Data: out}, nil
}

func employerInvitationOut(inv employer.Invitation) apigen.EmployerTeamInvitation {
	return apigen.EmployerTeamInvitation{ID: inv.ID, Email: inv.Email, CreatedAt: inv.CreatedAt, ExpiresAt: inv.ExpiresAt}
}

// InviteEmployerTeamMember mails a colleague a link to join the caller's company team.
func (h *employerHandlers) InviteEmployerTeamMember(ctx context.Context, req *apigen.EmployerTeamInvite) (*apigen.InviteEmployerTeamMemberCreated, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	inv, err := h.employer.Invite(ctx, userID, req.Email.Value)
	if err != nil {
		return nil, employerError(err)
	}
	return &apigen.InviteEmployerTeamMemberCreated{Data: employerInvitationOut(inv)}, nil
}

// WithdrawEmployerTeamInvitation cancels one of the caller's company's invitations.
func (h *employerHandlers) WithdrawEmployerTeamInvitation(ctx context.Context, params apigen.WithdrawEmployerTeamInvitationParams) (*apigen.WithdrawEmployerTeamInvitationOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	id, err := contractPathID(params.ID)
	if err != nil {
		return nil, err
	}
	if err := h.employer.WithdrawInvitation(ctx, userID, id); err != nil {
		return nil, employerError(err)
	}
	return &apigen.WithdrawEmployerTeamInvitationOK{Data: apigen.EmployerRemoved{Removed: true}}, nil
}

// RemoveEmployerTeamMember takes a member out of the caller's company team.
func (h *employerHandlers) RemoveEmployerTeamMember(ctx context.Context, params apigen.RemoveEmployerTeamMemberParams) (*apigen.RemoveEmployerTeamMemberOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	memberID, err := contractPathID(params.UserID)
	if err != nil {
		return nil, err
	}
	if err := h.employer.RemoveMember(ctx, userID, memberID); err != nil {
		return nil, employerError(err)
	}
	return &apigen.RemoveEmployerTeamMemberOK{Data: apigen.EmployerRemoved{Removed: true}}, nil
}

// PreviewEmployerTeamInvitation says what an invitation link offers. The token travels in
// the body, not the query, so it never lands in an access log.
func (h *employerHandlers) PreviewEmployerTeamInvitation(ctx context.Context, req *apigen.EmployerTeamToken) (*apigen.PreviewEmployerTeamInvitationOK, error) {
	if _, err := requireUserIDFromContext(ctx); err != nil {
		return nil, err
	}
	p, err := h.employer.PreviewInvitation(ctx, req.Token.Value)
	if err != nil {
		return nil, employerError(err)
	}
	return &apigen.PreviewEmployerTeamInvitationOK{Data: apigen.EmployerTeamInvitationPreview{CompanyName: p.CompanyName, Email: p.Email}}, nil
}

// JoinEmployerTeam makes the caller a member of the team the link invites to.
func (h *employerHandlers) JoinEmployerTeam(ctx context.Context, req *apigen.EmployerTeamToken) (*apigen.JoinEmployerTeamOK, error) {
	userID, err := requireUserIDFromContext(ctx)
	if err != nil {
		return nil, err
	}
	acc, err := h.employer.AcceptInvitation(ctx, userID, req.Token.Value)
	if err != nil {
		return nil, employerError(err)
	}
	return &apigen.JoinEmployerTeamOK{Data: employerAccountOut(acc)}, nil
}

// teamInviteMailer translates employer's invitation into emailnotify's: the two packages
// share a layer and cannot import each other.
type teamInviteMailer struct{ mailer *emailnotify.TeamInviteMailer }

func (m teamInviteMailer) SendTeamInvitation(ctx context.Context, inv employer.TeamInvitationMail) error {
	return m.mailer.SendTeamInvitation(ctx, emailnotify.TeamInvitation{
		To: inv.To, CompanyName: inv.CompanyName, InvitedBy: inv.InvitedBy, Link: inv.Link, ExpiresAt: inv.ExpiresAt,
	})
}
