package billing

import (
	"encoding/json"
	"errors"
	"strconv"
)

// Provider names the payment provider a stored event came from. It is part of the
// idempotency key so that a second provider's event identifiers — opaque strings from an
// unrelated namespace — cannot collide with these.
//
// The column exists because a second provider is a real prospect rather than a hypothetical
// one: mobile purchases cannot go through this provider at all, so the day an app ships,
// events from somewhere else land in the same table beside these.
const Provider = "stripe"

// Event is one webhook delivery, reduced to what we store and act on. Note what is NOT
// here: no status, no period, no price. The event says something about this customer
// changed; what it changed to is read from the provider, not from the message.
type Event struct {
	// ID is the provider's own event identifier. Redeliveries reuse it, which is what makes
	// it the idempotency key rather than a hash of the body.
	ID string
	// CustomerID is the provider's customer the event concerns, empty when the event is
	// about something else entirely.
	CustomerID string
	// UserRef is our own account id when the event carries it — set on a checkout
	// completion, which is the one event that arrives BEFORE any customer binding exists.
	UserRef string
	// Type is recorded and never branched on.
	Type string
	// Payload is the event object as received, whole. What we read today will change; what
	// arrived will not.
	Payload []byte
}

var errNoEvent = errors.New("billing: delivery carries no event data")

// parseEvent reads the webhook envelope.
//
// It requires an id and nothing else. That is the only field the system depends on to be
// idempotent, and demanding more would turn every event type the provider adds — and there
// are hundreds — into a rejected delivery.
func parseEvent(raw []byte) (Event, error) {
	var envelope struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Data struct {
			Object json.RawMessage `json:"object"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return Event{}, err
	}
	if envelope.ID == "" {
		// Without an id there is no idempotency key, so a redelivery would be recorded and
		// applied a second time. Refusing makes the provider retry, which is the safe
		// direction to fail in.
		return Event{}, errors.New("billing: event has no id")
	}
	if len(envelope.Data.Object) == 0 {
		return Event{}, errNoEvent
	}

	ev := Event{ID: envelope.ID, Type: envelope.Type, Payload: envelope.Data.Object}
	ev.CustomerID, ev.UserRef = subjectOf(envelope.Data.Object)
	return ev, nil
}

// subjectOf digs the customer and our own account reference out of the event's object.
//
// The provider's objects do not share one shape, but the two fields we need appear in the
// same places across the ones that matter to a subscription's life: `customer` on
// subscriptions, invoices and charges; `client_reference_id` on a completed checkout
// session, which is the ONLY event that arrives before a customer binding exists.
//
// An object carrying neither is not an error. It is an event about something we do not
// meter, and it is recorded and stamped rather than retried forever.
func subjectOf(object json.RawMessage) (customerID, userRef string) {
	var fields struct {
		Customer          string `json:"customer"`
		ClientReferenceID string `json:"client_reference_id"`
		Metadata          struct {
			UserID string `json:"freehire_user_id"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(object, &fields); err != nil {
		// A `customer` that is an expanded object rather than an id string lands here. The
		// caller still has the stored payload and the reconciler still has the account, so
		// this costs a pass rather than the event.
		return "", ""
	}

	userRef = fields.ClientReferenceID
	if userRef == "" {
		userRef = fields.Metadata.UserID
	}
	return fields.Customer, userRef
}

// userIDFromRef resolves our own account reference, as the provider echoed it back.
//
// We only ever send a users.id, so anything else was never ours: a dashboard test object,
// or an object created outside this integration. Those are recorded and reported — not
// dropped, and certainly not guessed at.
//
// Zero and negative are refused rather than accepted and left to fail on a foreign key,
// because "0" is what a broken client sends when it means "nobody".
func userIDFromRef(ref string) (int64, bool) {
	id, err := strconv.ParseInt(ref, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}
