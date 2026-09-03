package billing

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseEvent(t *testing.T) {
	cases := []struct {
		name          string
		body          string
		wantErr       bool
		wantID        string
		wantAppUserID string
		wantType      string
	}{
		{
			name:          "an ordinary renewal",
			body:          `{"api_version":"1.0","event":{"id":"evt_1","app_user_id":"601","type":"RENEWAL","expiration_at_ms":1790000000000}}`,
			wantID:        "evt_1",
			wantAppUserID: "601",
			wantType:      "RENEWAL",
		},
		{
			// The provider's event vocabulary is theirs and it grows. Nothing here branches
			// on the type, so an unfamiliar one must parse like any other and trigger the
			// same re-read of subscriber state.
			name:          "an event type we have never seen",
			body:          `{"api_version":"1.0","event":{"id":"evt_2","app_user_id":"601","type":"SOMETHING_NEW_IN_2027"}}`,
			wantID:        "evt_2",
			wantAppUserID: "601",
			wantType:      "SOMETHING_NEW_IN_2027",
		},
		{
			name:    "no event object",
			body:    `{"api_version":"1.0"}`,
			wantErr: true,
		},
		{
			// Without an id there is no idempotency key, so a redelivery would be recorded
			// again and applied again. Better to make the provider retry.
			name:    "event with no id",
			body:    `{"api_version":"1.0","event":{"app_user_id":"601","type":"RENEWAL"}}`,
			wantErr: true,
		},
		{
			name:    "event with no app_user_id",
			body:    `{"api_version":"1.0","event":{"id":"evt_3","type":"RENEWAL"}}`,
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
			if got.AppUserID != tc.wantAppUserID {
				t.Errorf("app_user_id: want %q, got %q", tc.wantAppUserID, got.AppUserID)
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
	body := `{"api_version":"1.0","event":{"id":"evt_4","app_user_id":"601","type":"RENEWAL","store":"stripe","price_in_purchased_currency":4.99}}`

	got, err := parseEvent([]byte(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for _, want := range []string{"price_in_purchased_currency", "store"} {
		if !strings.Contains(string(got.Payload), want) {
			t.Errorf("payload dropped %q: %s", want, got.Payload)
		}
	}
}

// TestUserIDFromAppUserID covers the identifier round-trip. We put users.id in, so we
// expect it back — but the provider will also hand us identifiers that were never ours,
// and those have to be recorded rather than rejected.
func TestUserIDFromAppUserID(t *testing.T) {
	cases := []struct {
		in     string
		want   int64
		wantOK bool
	}{
		{in: "601", want: 601, wantOK: true},
		{in: "$RCAnonymousID:9f8c", wantOK: false},
		{in: "", wantOK: false},
		{in: "601abc", wantOK: false},
		{in: "-1", wantOK: false},
		{in: "0", wantOK: false},
	}

	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, ok := userIDFromAppUserID(tc.in)
			if ok != tc.wantOK {
				t.Fatalf("ok: want %v, got %v", tc.wantOK, ok)
			}
			if ok && got != tc.want {
				t.Fatalf("want %d, got %d", tc.want, got)
			}
		})
	}
}
