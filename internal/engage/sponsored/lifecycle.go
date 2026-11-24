// Package sponsored runs pay-per-click sponsored job campaigns: the lifecycle state
// machine and the click charge. Money is integer USD cents.
package sponsored

import "slices"

type Status string

const (
	StatusDraft          Status = "draft"
	StatusPendingPayment Status = "pending_payment"
	StatusPendingReview  Status = "pending_review"
	StatusActive         Status = "active"
	StatusPaused         Status = "paused"
	StatusExhausted      Status = "exhausted"
	StatusRejected       Status = "rejected"
	StatusEnded          Status = "ended"
)

// transitions is the whole lifecycle (design D3). A draft goes straight to pending_review
// when its first top-up lands without a prior pending_payment step.
var transitions = map[Status][]Status{
	StatusDraft:          {StatusPendingPayment, StatusPendingReview, StatusEnded},
	StatusPendingPayment: {StatusPendingReview, StatusEnded},
	StatusPendingReview:  {StatusActive, StatusRejected, StatusEnded},
	StatusActive:         {StatusPaused, StatusExhausted, StatusEnded},
	StatusPaused:         {StatusActive, StatusEnded},
	StatusExhausted:      {StatusActive, StatusEnded},
}

func CanTransition(from, to Status) bool {
	return slices.Contains(transitions[from], to)
}

// ownerFrom lists, per target status, the statuses a campaign's owner may move it from. Narrower
// than transitions: only a moderator approves (pending_review -> active) and only a top-up
// reactivates an exhausted campaign, so an owner can do neither.
var ownerFrom = map[Status][]Status{
	StatusPaused: {StatusActive},
	StatusActive: {StatusPaused},
	StatusEnded:  {StatusDraft, StatusPendingPayment, StatusPendingReview, StatusActive, StatusPaused, StatusExhausted},
}

func CanOwnerTransition(from, to Status) bool {
	return slices.Contains(ownerFrom[to], from)
}
