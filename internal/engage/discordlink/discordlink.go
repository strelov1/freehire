// Package discordlink binds a freehire account to a Discord account and keeps the paid role
// on the community server in step with the subscription.
//
// It is outbound engagement, not billing. It never reads a payment provider: whether an
// account pays is plan.TierOf over the entitlement columns, which is the only answer that
// accounts for Stripe, the app stores and granted promo time alike. A component that asked
// Stripe directly would be wrong for every store subscriber.
//
// There is no bot process. Discord's REST API is enough for everything here — grant a role,
// revoke a role, add a member — so the work is done by cmd/discord-sync, a run-once-and-exit
// worker, and by the link routes. Nothing holds a gateway connection or reads a message.
//
// The file split: this file is the decision (who warrants the role, what reconciliation owes
// one binding) and holds no I/O, so the rule can be read and tested on its own; client.go
// speaks to Discord; repository.go is the store; service.go is the use case that joins them.
package discordlink

import "github.com/strelov1/freehire/internal/ai/plan"

// Link is one account's binding to a Discord account.
type Link struct {
	UserID        int64
	DiscordUserID string
	// RoleGranted is whether we believe the paid role is currently held. It is our record,
	// not Discord's — kept so reconciliation can skip an account whose state has not moved
	// instead of asking Discord about every account every hour.
	RoleGranted bool
}

// WarrantsPaidRole reports whether a tier should hold the paid role.
//
// Every paying tier gets the SAME role: the closed channels are a perk of paying, not of
// paying more. This is the one place that decides, so a future Ultra-only channel is a
// second role resolved here rather than a condition spread across the worker and the link
// path.
func WarrantsPaidRole(t plan.Tier) bool {
	return t != plan.TierFree
}

// Action is what reconciliation owes one binding.
type Action int

const (
	// ActionNone is the common case and the important one: the record already matches the
	// plan, so Discord is not called at all. Without it an hourly timer would rewrite every
	// role we manage, every hour, to change nothing.
	ActionNone Action = iota
	ActionGrant
	ActionRevoke
)

// Reconcile compares a binding's recorded role against the plan and reports what to do.
//
// It takes the record rather than asking Discord because the record is what a run can read
// for every account at once, in the same query that reads the plan. Where the two have
// drifted — somebody removed the role by hand — the next real change corrects it; a run that
// verified every account against Discord would cost one request per subscriber per hour to
// find nothing.
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

// String makes a failed test and a log line name the action instead of printing an integer.
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
