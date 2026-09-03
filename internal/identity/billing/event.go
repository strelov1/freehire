package billing

import (
	"encoding/json"
	"errors"
	"strconv"
)

// Provider names the payment provider a stored event came from. It is part of the
// idempotency key so that a second provider's event identifiers — opaque strings from an
// unrelated namespace — cannot collide with these.
const Provider = "revenuecat"

// Event is one webhook delivery, reduced to what we store and act on. Note what is NOT
// here: no expiry, no entitlement, no status. The event says something about this user
// changed; what it changed to is read from the provider, not from the message.
type Event struct {
	// ID is the provider's own event identifier. Retries reuse it, which is what makes it
	// the idempotency key rather than a hash of the body.
	ID string
	// AppUserID is the identifier as it arrived, unparsed. Stored verbatim because it is
	// evidence: an identifier that is not one of ours is exactly the thing worth keeping.
	AppUserID string
	// Type is recorded and never branched on.
	Type string
	// Payload is the event object as received, whole. What we read today will change;
	// what arrived will not.
	Payload []byte
}

var errNoEvent = errors.New("billing: delivery carries no event object")

// parseEvent reads the webhook envelope.
//
// It requires an id and an app user id and nothing else. Those two are the only fields the
// system depends on — one to be idempotent, one to know who this is about — and demanding
// more would turn every field the provider adds, renames or omits into a rejected delivery.
func parseEvent(raw []byte) (Event, error) {
	var envelope struct {
		Event json.RawMessage `json:"event"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return Event{}, err
	}
	if len(envelope.Event) == 0 {
		return Event{}, errNoEvent
	}

	var fields struct {
		ID        string `json:"id"`
		AppUserID string `json:"app_user_id"`
		Type      string `json:"type"`
	}
	if err := json.Unmarshal(envelope.Event, &fields); err != nil {
		return Event{}, err
	}
	if fields.ID == "" {
		// Without an id there is no idempotency key, so a redelivery would be recorded and
		// applied a second time. Refusing makes the provider retry, which is the safe
		// direction to fail in.
		return Event{}, errors.New("billing: event has no id")
	}
	if fields.AppUserID == "" {
		return Event{}, errors.New("billing: event has no app_user_id")
	}

	return Event{
		ID:        fields.ID,
		AppUserID: fields.AppUserID,
		Type:      fields.Type,
		Payload:   envelope.Event,
	}, nil
}

// userIDFromAppUserID resolves the provider's identifier back to one of our accounts.
//
// We only ever hand the provider a users.id, so anything else is an identifier that was
// never ours: a dashboard TEST event, an anonymous identifier from a client that purchased
// before it was identified, or a transfer from another integration. Those are recorded
// with a NULL user and reported — not dropped, and certainly not guessed at.
//
// Zero and negative are refused rather than accepted and left to fail on a foreign key,
// because "0" is what a broken client sends when it means "nobody".
func userIDFromAppUserID(appUserID string) (int64, bool) {
	id, err := strconv.ParseInt(appUserID, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}
