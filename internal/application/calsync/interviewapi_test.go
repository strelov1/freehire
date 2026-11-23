package calsync

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"
	"time"
)

type interviewTransport struct {
	host *url.URL
	base http.RoundTripper
}

func (t interviewTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rewritten := req.Clone(req.Context())
	rewritten.URL.Host = t.host.Host
	rewritten.URL.Scheme = t.host.Scheme
	return t.base.RoundTrip(rewritten)
}

func TestInterviewInviteWaitsForMeetAndRecoversAnAlreadySentInvite(t *testing.T) {
	const eventID = "freehireabc123"
	const email = "recruiter@example.test"
	const meet = "https://meet.google.com/abc-defg-hij"
	var calls []string
	invited := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		calls = append(calls, req.Method)
		w.Header().Set("Content-Type", "application/json")
		switch req.Method {
		case http.MethodPost:
			var body struct {
				ID             string `json:"id"`
				Attendees      []any  `json:"attendees"`
				ConferenceData struct {
					CreateRequest struct {
						RequestID string `json:"requestId"`
					} `json:"createRequest"`
				} `json:"conferenceData"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil || body.ID != eventID ||
				body.ConferenceData.CreateRequest.RequestID != eventID || len(body.Attendees) != 0 ||
				req.URL.Query().Get("conferenceDataVersion") != "1" {
				t.Errorf("unexpected insert: body=%+v query=%s err=%v", body, req.URL.RawQuery, err)
			}
			if len(calls) != 1 {
				w.WriteHeader(http.StatusConflict)
				return
			}
			_, _ = w.Write([]byte(`{"id":"freehireabc123","conferenceData":{"createRequest":{"status":{"statusCode":"pending"}}}}`))
		case http.MethodGet:
			if req.URL.Path != "/calendar/v3/calendars/primary/events/"+eventID {
				t.Errorf("unexpected get path: %s", req.URL.Path)
			}
			if invited {
				_, _ = w.Write([]byte(`{"id":"freehireabc123","hangoutLink":"https://meet.google.com/abc-defg-hij","attendees":[{"email":"recruiter@example.test"}]}`))
				return
			}
			_, _ = w.Write([]byte(`{"id":"freehireabc123","hangoutLink":"https://meet.google.com/abc-defg-hij"}`))
		case http.MethodPatch:
			var body struct {
				Attendees []struct{ Email string } `json:"attendees"`
			}
			if err := json.NewDecoder(req.Body).Decode(&body); err != nil ||
				len(body.Attendees) != 1 || body.Attendees[0].Email != email ||
				req.URL.Query().Get("sendUpdates") != "all" {
				t.Errorf("unexpected invite: body=%+v query=%s err=%v", body, req.URL.RawQuery, err)
			}
			invited = true
			// Google accepted the write but its response was lost.
			w.WriteHeader(http.StatusServiceUnavailable)
		default:
			t.Errorf("unexpected method %s", req.Method)
		}
	}))
	t.Cleanup(srv.Close)
	host, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: interviewTransport{host: host, base: srv.Client().Transport}}
	writer := NewInterviewWriter(client)
	start := time.Date(2026, 12, 4, 9, 0, 0, 0, time.UTC)
	in := InterviewEvent{ID: eventID, Attendee: email, StartsAt: start, EndsAt: start.Add(30 * time.Minute)}
	if _, err := writer.Invite(context.Background(), in); !errors.Is(err, ErrConferencePending) {
		t.Fatalf("while provisioning Meet: %v", err)
	}
	if _, err := writer.Invite(context.Background(), in); err == nil {
		t.Fatal("lost invite response must be retried")
	}
	if got, err := writer.Invite(context.Background(), in); err != nil || got != meet {
		t.Fatalf("reconciled invitation: link = %q, err = %v", got, err)
	}
	if want := []string{"POST", "POST", "GET", "PATCH", "POST", "GET"}; !slices.Equal(calls, want) {
		t.Fatalf("calendar calls = %v, want %v", calls, want)
	}
}
