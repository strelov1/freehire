package cv

// AutopilotStatus is what an autopilot run made of one requirement. The vocabulary is
// fixed and small on purpose: the panel renders each value, and a value it cannot render
// is worse than a refused write. Nothing is coerced — an unrecognised status is reported
// back to the model with the valid ones named, which is how it corrects itself mid-turn.
type AutopilotStatus string

const (
	// AutopilotClosedBank: the experience bank already held evidence, and the run
	// rewrote the CV around it.
	AutopilotClosedBank AutopilotStatus = "closed_bank"
	// AutopilotClosedCandidate: the candidate confirmed it in conversation after the
	// run, their words were banked, and the bullet cites that record.
	AutopilotClosedCandidate AutopilotStatus = "closed_candidate"
	// AutopilotOpen: the bank holds nothing for it. This is the remainder worth asking about.
	AutopilotOpen AutopilotStatus = "open"
	// AutopilotNotReached: the run ended (step cap, cancellation) before reaching it.
	AutopilotNotReached AutopilotStatus = "not_reached"
)

// AutopilotStatuses is the vocabulary as data, for the tool schema and the error message.
var AutopilotStatuses = []string{
	string(AutopilotClosedBank),
	string(AutopilotClosedCandidate),
	string(AutopilotOpen),
	string(AutopilotNotReached),
}

// AutopilotEntry is one requirement and what the run made of it.
type AutopilotEntry struct {
	Requirement string          `json:"requirement"`
	Status      AutopilotStatus `json:"status"`
	Note        string          `json:"note,omitempty"`
}
