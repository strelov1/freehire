package mentorship

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/application/gmailsync"
	"github.com/strelov1/freehire/internal/platform/tokencrypt"
)

// ErrCalendarNotConnected reports that this mentor holds no usable calendar.events grant
// (no row, another status such as needs_reconsent, or scopes that never covered the
// write). Book() falls back to the mentor's static MeetingURL.
var ErrCalendarNotConnected = errors.New("mentorship: mentor has not connected a calendar")

// meetEventsURL is the mentor's primary-calendar events collection, written here rather
// than listed.
const meetEventsURL = "https://www.googleapis.com/calendar/v3/calendars/primary/events"

// MeetEventInput is what one booking needs turned into a calendar event.
type MeetEventInput struct {
	StartsAt    time.Time
	EndsAt      time.Time
	SeekerEmail string
	Summary     string
}

// CalendarLinker creates and deletes the Google Calendar event that carries a booking's
// Meet link. Nil-safe on Service like Notifier and Cache: with no Google client configured
// the static link is used.
type CalendarLinker interface {
	CreateMeetEvent(ctx context.Context, userID int64, in MeetEventInput) (eventID, meetLink string, err error)
	DeleteMeetEvent(ctx context.Context, userID int64, eventID string) error
}

// calendarGrantReader is the one Repository method GoogleCalendarLinker needs.
type calendarGrantReader interface {
	GetMentorCalendarGrant(ctx context.Context, userID int64) (refreshTokenEnc string, scopes []string, found bool, err error)
}

// GoogleCalendarLinker implements CalendarLinker against the real Google Calendar API.
type GoogleCalendarLinker struct {
	repo      calendarGrantReader
	connector *gmailsync.Connector
	cipher    *tokencrypt.Cipher
}

// NewGoogleCalendarLinker builds the linker. connector mints the token-bearing HTTP client
// from a decrypted refresh token.
func NewGoogleCalendarLinker(repo calendarGrantReader, connector *gmailsync.Connector, cipher *tokencrypt.Cipher) *GoogleCalendarLinker {
	return &GoogleCalendarLinker{repo: repo, connector: connector, cipher: cipher}
}

// hasCalendarWriteGrant reports whether a grant covers calendar.events. resolve() and
// HasConnectedCalendar share it so "connected" means one thing.
func hasCalendarWriteGrant(found bool, scopes []string) bool {
	return found && slices.Contains(scopes, gmailsync.CalendarEventsScope)
}

// resolve turns a mentor's grant into an authenticated meetAPI, or reports
// ErrCalendarNotConnected. Both CreateMeetEvent and DeleteMeetEvent go through it.
func (l *GoogleCalendarLinker) resolve(ctx context.Context, userID int64) (*meetAPI, error) {
	encToken, scopes, found, err := l.repo.GetMentorCalendarGrant(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !hasCalendarWriteGrant(found, scopes) {
		return nil, ErrCalendarNotConnected
	}
	refresh, err := l.cipher.Decrypt(encToken)
	if err != nil {
		return nil, fmt.Errorf("mentor calendar: decrypt token: %w", err)
	}
	return newMeetAPI(l.connector.HTTPClient(ctx, refresh)), nil
}

// HasConnectedCalendar reports whether userID holds a usable calendar.events grant, which
// decides whether the meeting link field is still required.
func (s *Service) HasConnectedCalendar(ctx context.Context, userID int64) (bool, error) {
	_, scopes, found, err := s.repo.GetMentorCalendarGrant(ctx, userID)
	if err != nil {
		return false, err
	}
	return hasCalendarWriteGrant(found, scopes), nil
}

// CreateMeetEvent books a calendar event with the seeker as an attendee and a Meet link
// requested via conferenceData; Book() snapshots the link onto the booking.
func (l *GoogleCalendarLinker) CreateMeetEvent(ctx context.Context, userID int64, in MeetEventInput) (string, string, error) {
	api, err := l.resolve(ctx, userID)
	if err != nil {
		return "", "", err
	}
	return api.createEvent(ctx, in)
}

// DeleteMeetEvent best-effort removes the event Cancel() no longer needs.
func (l *GoogleCalendarLinker) DeleteMeetEvent(ctx context.Context, userID int64, eventID string) error {
	api, err := l.resolve(ctx, userID)
	if err != nil {
		return err
	}
	return api.deleteEvent(ctx, eventID)
}

// meetAPI does the Google Calendar write calls over an authenticated client, separate from
// grant resolution so it is testable via httptest.
type meetAPI struct {
	client *http.Client
}

func newMeetAPI(client *http.Client) *meetAPI { return &meetAPI{client: client} }

// meetEvent is the slice of Google's event response this package reads: its own id (to
// delete the event later) and the Meet link conferenceDataVersion=1 minted.
type meetEvent struct {
	ID          string `json:"id"`
	HangoutLink string `json:"hangoutLink"`
}

func (a *meetAPI) createEvent(ctx context.Context, in MeetEventInput) (string, string, error) {
	payload, err := json.Marshal(map[string]any{
		"summary": in.Summary,
		"start":   map[string]string{"dateTime": in.StartsAt.Format(time.RFC3339)},
		"end":     map[string]string{"dateTime": in.EndsAt.Format(time.RFC3339)},
		"attendees": []map[string]string{
			{"email": in.SeekerEmail},
		},
		"conferenceData": map[string]any{
			"createRequest": map[string]any{
				"requestId":             uuid.NewString(),
				"conferenceSolutionKey": map[string]string{"type": "hangoutsMeet"},
			},
		},
	})
	if err != nil {
		return "", "", fmt.Errorf("mentor calendar: encode event: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		meetEventsURL+"?conferenceDataVersion=1", bytes.NewReader(payload))
	if err != nil {
		return "", "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := a.client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("mentor calendar: create event: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", "", gmailsync.NewAPIError("mentor calendar: create event", resp)
	}

	var out meetEvent
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", "", fmt.Errorf("mentor calendar: decode event: %w", err)
	}
	if out.HangoutLink == "" {
		// conferenceData.createRequest may still be PENDING when the insert answers 200/201
		// (the link is provisioned asynchronously). There is no later retry: the event is
		// kept and the booking's link stays empty; this logs the case.
		log.Printf("mentor calendar: event %s created with no hangoutLink yet (conference still pending)", out.ID)
	}
	return out.ID, out.HangoutLink, nil
}

// deleteEvent removes one event. A 404/410 means it is already gone and counts as success.
func (a *meetAPI) deleteEvent(ctx context.Context, eventID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete,
		meetEventsURL+"/"+url.PathEscape(eventID), nil)
	if err != nil {
		return err
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("mentor calendar: delete event: %w", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK, http.StatusNoContent, http.StatusNotFound, http.StatusGone:
		return nil
	default:
		return gmailsync.NewAPIError("mentor calendar: delete event", resp)
	}
}
