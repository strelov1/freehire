package billing

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseEvent(t *testing.T) {
	cases := []struct {
		name     string
		body     string
		wantErr  bool
		wantID   string
		wantCust string
		wantRef  string
		wantType string
	}{
		{
			name:     "an ordinary renewal",
			body:     `{"id":"evt_1","type":"invoice.paid","data":{"object":{"customer":"cus_9","status":"paid"}}}`,
			wantID:   "evt_1",
			wantCust: "cus_9",
			wantType: "invoice.paid",
		},
		{
			// The one event that arrives BEFORE any customer binding exists, which is why it
			// carries our own account id and why we put it there.
			name:     "a completed checkout carries our account reference",
			body:     `{"id":"evt_2","type":"checkout.session.completed","data":{"object":{"customer":"cus_9","client_reference_id":"601"}}}`,
			wantID:   "evt_2",
			wantCust: "cus_9",
			wantRef:  "601",
			wantType: "checkout.session.completed",
		},
		{
			name:     "the account reference also travels in metadata",
			body:     `{"id":"evt_3","type":"customer.subscription.updated","data":{"object":{"customer":"cus_9","metadata":{"freehire_user_id":"601"}}}}`,
			wantID:   "evt_3",
			wantCust: "cus_9",
			wantRef:  "601",
			wantType: "customer.subscription.updated",
		},
		{
			// The provider has hundreds of event types and adds more. Nothing here branches on
			// the type, so an unfamiliar one must parse like any other.
			name:     "an event type we have never seen",
			body:     `{"id":"evt_4","type":"something.new.in.2027","data":{"object":{"customer":"cus_9"}}}`,
			wantID:   "evt_4",
			wantCust: "cus_9",
			wantType: "something.new.in.2027",
		},
		{
			// Not everything the provider sends is about a customer. Recording it and moving
			// on beats retrying it forever.
			name:     "an event about nobody",
			body:     `{"id":"evt_5","type":"price.updated","data":{"object":{"id":"price_x"}}}`,
			wantID:   "evt_5",
			wantType: "price.updated",
		},
		{
			name:    "no data object",
			body:    `{"id":"evt_6","type":"invoice.paid"}`,
			wantErr: true,
		},
		{
			// Without an id there is no idempotency key, so a redelivery would be applied
			// twice. Refusing makes the provider retry, which is the safe direction.
			name:    "event with no id",
			body:    `{"type":"invoice.paid","data":{"object":{"customer":"cus_9"}}}`,
			wantErr: true,
		},
		{
			name:    "not JSON at all",
			body:    `<html>gateway timeout</html>`,
			wantErr: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseEvent([]byte(tc.body))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want an error, got %+v", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("want no error, got %v", err)
			}
			if got.ID != tc.wantID {
				t.Errorf("id: want %q, got %q", tc.wantID, got.ID)
			}
			if got.CustomerID != tc.wantCust {
				t.Errorf("customer: want %q, got %q", tc.wantCust, got.CustomerID)
			}
			if got.UserRef != tc.wantRef {
				t.Errorf("ref: want %q, got %q", tc.wantRef, got.UserRef)
			}
			if got.Type != tc.wantType {
				t.Errorf("type: want %q, got %q", tc.wantType, got.Type)
			}
			if !json.Valid(got.Payload) {
				t.Errorf("payload is not valid JSON: %s", got.Payload)
			}
		})
	}
}

// TestParseEventKeepsFieldsItDoesNotRead is why the payload is stored at all. What we read
// today will change; what arrived will not. A field we have no name for must survive into
// the stored record, or the audit trail answers only the questions we already thought of.
func TestParseEventKeepsFieldsItDoesNotRead(t *testing.T) {
	body := `{"id":"evt_7","type":"invoice.paid","data":{"object":{"customer":"cus_9","amount_paid":500,"currency":"usd"}}}`

	got, err := parseEvent([]byte(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, want := range []string{"amount_paid", "currency"} {
		if !strings.Contains(string(got.Payload), want) {
			t.Errorf("payload dropped %q: %s", want, got.Payload)
		}
	}
}

// TestUserIDFromRef covers the identifier round-trip. We only ever send a users.id, so
// anything else was never ours and must be reported rather than guessed at.
func TestUserIDFromRef(t *testing.T) {
	cases := []struct {
		in     string
		want   int64
		wantOK bool
	}{
		{in: "601", want: 601, wantOK: true},
		{in: "", wantOK: false},
		{in: "601abc", wantOK: false},
		{in: "-1", wantOK: false},
		{in: "0", wantOK: false},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, ok := userIDFromRef(tc.in)
			if ok != tc.wantOK {
				t.Fatalf("ok: want %v, got %v", tc.wantOK, ok)
			}
			if ok && got != tc.want {
				t.Fatalf("want %d, got %d", tc.want, got)
			}
		})
	}
}
