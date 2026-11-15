// Package discordlink binds a freehire account to a Discord account and keeps the paid role
// on the community server in step with the subscription.
//
// It is outbound engagement, not billing. Whether an account pays is
// plan.TierOf over the entitlement columns, the only answer that covers Stripe,
// the app stores and granted promo time; never ask Stripe directly.
//
// There is no bot process: Discord's REST API covers grant, revoke and add, so
// the work is done by cmd/discord-sync (run-once) and the link routes.
//
// This file is the decision (who warrants the role, what reconciliation owes one
// binding) with no I/O; client.go speaks to Discord, repository.go is the store,
// service.go joins them.
package discordlink

import "github.com/strelov1/freehire/internal/ai/plan"

// Link is one account's binding to a Discord account.
type Link struct {
	UserID        int64
	DiscordUserID string
	// RoleGranted is whether we believe the paid role is held. It is our record, not
	// Discord's, so reconciliation can skip accounts whose state has not moved.
	RoleGranted bool
}

// WarrantsPaidRole reports whether a tier should hold the paid role.
// Every paying tier gets the same role: the closed channels are a perk of
// paying, not of paying more. This is the one place that decides, so a future
// Ultra-only channel is a second role resolved here.
func WarrantsPaidRole(t plan.Tier) bool {
	return t != plan.TierFree
}

// Action is what reconciliation owes one binding.
type Action int

const (
	// ActionNone is the common case: the record already matches the plan, so
	// Discord is not called.
	ActionNone Action = iota
	ActionGrant
	ActionRevoke
)

// Reconcile compares a binding's recorded role against the plan and reports what to do.
// It takes the record rather than asking Discord, since a run can read the
// record for every account in the query that reads the plan. A role removed by
// hand is corrected by the next real change; verifying every account against
// Discord would cost a request per subscriber per hour.
func Reconcile(tier plan.Tier, roleGranted bool) Action {
	warrants := WarrantsPaidRole(tier)
	switch {
	case warrants && !roleGranted:
		return ActionGrant
	case !warrants && roleGranted:
		return ActionRevoke
	default:
		return ActionNone
	}
}

// String names the action in failed tests and logs.
func (a Action) String() string {
	switch a {
	case ActionGrant:
		return "grant"
	case ActionRevoke:
		return "revoke"
	default:
		return "none"
	}
}
