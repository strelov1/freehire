package calsync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/application/gmailsync"
)

// ErrConferencePending means the calendar event exists and Google is still creating its Meet link.
var ErrConferencePending = errors.New("interview calendar: conference pending")

// InterviewEvent identifies one interview. ID must be a stable Google event ID for this booking.
type InterviewEvent struct {
	ID       string
	Attendee string
	StartsAt time.Time
	EndsAt   time.Time
}

// InterviewWriter creates and invites attendees to candidate-owned calendar events.
type InterviewWriter struct {
	client *http.Client
}

func NewInterviewWriter(client *http.Client) *InterviewWriter {
	return &InterviewWriter{client: client}
}

type calendarAttendee struct {
	Email string `json:"email"`
}

type interviewResponse struct {
	ID          string             `json:"id"`
	ETag        string             `json:"etag"`
	Status      string             `json:"status"`
	HangoutLink string             `json:"hangoutLink"`
	Attendees   []calendarAttendee `json:"attendees"`
	Conference  struct {
		CreateRequest struct {
			Status struct {
				Code string `json:"statusCode"`
			} `json:"status"`
		} `json:"createRequest"`
	} `json:"conferenceData"`
}

// Invite first creates the event and its Meet conference, then adds the attendee.
// Repeating the call after an uncertain response uses the same event and checks whether
// the invitation was already sent. The recruiter only receives an event with a Meet URL.
func (w *InterviewWriter) Invite(ctx context.Context, in InterviewEvent) (string, error) {
	eventURL := eventsURL + "/" + url.PathEscape(in.ID)
	event, err := w.call(ctx, http.MethodPost, eventsURL+"?conferenceDataVersion=1&sendUpdates=none", "", map[string]any{
		"id":      in.ID,
		"summary": "Interview",
		"start":   map[string]string{"dateTime": in.StartsAt.UTC().Format(time.RFC3339)},
		"end":     map[string]string{"dateTime": in.EndsAt.UTC().Format(time.RFC3339)},
		"conferenceData": map[string]any{
			"createRequest": map[string]any{
				"requestId":             in.ID,
				"conferenceSolutionKey": map[string]string{"type": "hangoutsMeet"},
			},
		},
	})
	var apiErr *gmailsync.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusConflict {
		event, err = w.call(ctx, http.MethodGet, eventURL, "", nil)
	}
	if err != nil {
		return "", err
	}
	if event.ID != in.ID || event.Status == "cancelled" {
		return "", errors.New("interview calendar: event unavailable")
	}
	if event.Conference.CreateRequest.Status.Code == "failure" {
		return "", errors.New("interview calendar: meet creation failed")
	}
	if event.HangoutLink == "" {
		return "", ErrConferencePending
	}
	link, err := url.Parse(event.HangoutLink)
	if err != nil || link.Scheme != "https" || link.Host != "meet.google.com" || link.User != nil {
		return "", errors.New("interview calendar: invalid meet link")
	}
	if slices.ContainsFunc(event.Attendees, func(attendee calendarAttendee) bool {
		return strings.EqualFold(attendee.Email, in.Attendee)
	}) {
		return event.HangoutLink, nil
	}
	event.Attendees = append(event.Attendees, calendarAttendee{Email: in.Attendee})
	_, err = w.call(ctx, http.MethodPatch, eventURL+"?sendUpdates=all&conferenceDataVersion=1", event.ETag,
		map[string]any{"attendees": event.Attendees})
	if err != nil {
		return "", err
	}
	return event.HangoutLink, nil
}

func (w *InterviewWriter) call(ctx context.Context, method, endpoint, etag string, body any) (interviewResponse, error) {
	var event interviewResponse
	var input io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return event, err
		}
		input = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, input)
	if err != nil {
		return event, err
	}
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if etag != "" {
		req.Header.Set("If-Match", etag)
	}
	resp, err := w.client.Do(req)
	if err != nil {
		return event, fmt.Errorf("interview calendar: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return event, gmailsync.NewAPIError("interview calendar", resp)
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&event); err != nil {
		return event, fmt.Errorf("decode calendar event: %w", err)
	}
	return event, nil
}
