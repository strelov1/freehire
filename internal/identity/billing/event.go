package billing

import (
	"encoding/json"
	"errors"
	"strconv"
)

// The provider name is part of the idempotency key so two providers' event ids cannot
// collide; read it through provider.name().

// Event is one webhook delivery, reduced to what we store and act on. It carries no status,
// period or price: what changed is read from the provider, not the message.
type Event struct {
	// ID is the provider's event identifier. Redeliveries reuse it, so it is the idempotency key.
	ID string
	// CustomerID is the provider's customer the event concerns, empty if none.
	CustomerID string
	// UserRef is our account id when the event carries it: set on a checkout completion, the
	// one event that arrives before any customer binding exists.
	UserRef string
	// Type is recorded and never branched on.
	Type string
	// Payload is the event object as received, whole.
	Payload []byte
}

var errNoEvent = errors.New("billing: delivery carries no event data")

// parseEvent reads the webhook envelope. It requires only an id, the one field idempotency
// depends on; demanding more would reject every new event type.
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
		// Without an id a redelivery would be applied twice; refusing makes the provider retry.
		return Event{}, errors.New("billing: event has no id")
	}
	if len(envelope.Data.Object) == 0 {
		return Event{}, errNoEvent
	}

	ev := Event{ID: envelope.ID, Type: envelope.Type, Payload: envelope.Data.Object}
	ev.CustomerID, ev.UserRef = subjectOf(envelope.Data.Object)
	return ev, nil
}

// subjectOf extracts the customer and our account reference from the event's object:
// `customer` on subscriptions, invoices and charges; `client_reference_id` on a completed
// checkout session. An object with neither is not an error: it is recorded and stamped
// rather than retried forever.
func subjectOf(object json.RawMessage) (customerID, userRef string) {
	var fields struct {
		Customer          string `json:"customer"`
		ClientReferenceID string `json:"client_reference_id"`
		Metadata          struct {
			UserID string `json:"freehire_user_id"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal(object, &fields); err != nil {
		// An expanded `customer` object lands here; the stored payload and the reconciler still
		// cover it.
		return "", ""
	}

	userRef = fields.ClientReferenceID
	if userRef == "" {
		userRef = fields.Metadata.UserID
	}
	return fields.Customer, userRef
}

// userIDFromRef resolves our account reference as the provider echoed it. We only send a
// users.id, so anything else (dashboard test object, foreign object) is reported, not
// guessed at. Zero and negative are refused up front rather than failing a foreign key.
func userIDFromRef(ref string) (int64, bool) {
	id, err := strconv.ParseInt(ref, 10, 64)
	if err != nil || id <= 0 {
		return 0, false
	}
	return id, true
}
