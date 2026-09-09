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

// ErrCalendarNotConnected reports that this mentor holds no usable calendar.events grant —
// no row, a row in some other status (needs_reconsent included), or one whose scopes never
// covered the write. Book() treats it as the ordinary, overwhelmingly common case: fall back
// to the mentor's static MeetingURL, exactly as if this feature did not exist.
var ErrCalendarNotConnected = errors.New("mentorship: mentor has not connected a calendar")

// meetEventsURL is the same primary-calendar collection calsync reads, POSTed and DELETEd
// here instead of listed. One mentor's own calendar, never a shared or secondary one.
const meetEventsURL = "https://www.googleapis.com/calendar/v3/calendars/primary/events"

// MeetEventInput is what one booking needs turned into a calendar event.
type MeetEventInput struct {
	StartsAt    time.Time
	EndsAt      time.Time
	SeekerEmail string
	Summary     string
}

// CalendarLinker creates and deletes the Google Calendar event that carries a booking's
// Meet link. Behind an interface so Book()/Cancel() are tested without Google, and nil-safe
// on Service exactly as Notifier and Cache already are — a deployment with no Google client
// configured gets today's static-link behaviour, not a broken one.
type CalendarLinker interface {
	CreateMeetEvent(ctx context.Context, userID int64, in MeetEventInput) (eventID, meetLink string, err error)
	DeleteMeetEvent(ctx context.Context, userID int64, eventID string) error
}

// calendarGrantReader is the one Repository method GoogleCalendarLinker needs. Named
// narrowly rather than taking the full Repository so the linker's dependency is legible on
// its own constructor, though in production it is the same *QueriesRepository the Service
// holds.
type calendarGrantReader interface {
	GetMentorCalendarGrant(ctx context.Context, userID int64) (refreshTokenEnc string, scopes []string, found bool, err error)
}

// GoogleCalendarLinker implements CalendarLinker against the real Google Calendar API. It is
// the first place in this codebase that WRITES to that API — every other reader
// (internal/application/calsync) only lists.
type GoogleCalendarLinker struct {
	repo      calendarGrantReader
	connector *gmailsync.Connector
	cipher    *tokencrypt.Cipher
}

// NewGoogleCalendarLinker builds the linker. connector mints the token-bearing HTTP client
// from a decrypted refresh token, exactly as it already does for calsync's reader.
func NewGoogleCalendarLinker(repo calendarGrantReader, connector *gmailsync.Connector, cipher *tokencrypt.Cipher) *GoogleCalendarLinker {
	return &GoogleCalendarLinker{repo: repo, connector: connector, cipher: cipher}
}

// hasCalendarWriteGrant reports whether a grant read from GetMentorCalendarGrant actually
// covers calendar.events — the one test resolve() and HasConnectedCalendar share, so the
// booking flow's gate and the profile/frontend's "is this mentor connected?" question can
// never disagree about what "connected" means.
func hasCalendarWriteGrant(found bool, scopes []string) bool {
	return found && slices.Contains(scopes, gmailsync.CalendarEventsScope)
}

// resolve turns a mentor's grant into an authenticated meetAPI, or reports
// ErrCalendarNotConnected — the one gate both CreateMeetEvent and DeleteMeetEvent share, so
// a grant that stops qualifying refuses both identically.
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

// HasConnectedCalendar reports whether userID holds a usable calendar.events grant —
// what the profile validation and the frontend both need to decide whether the meeting
// link field is still required.
func (s *Service) HasConnectedCalendar(ctx context.Context, userID int64) (bool, error) {
	_, scopes, found, err := s.repo.GetMentorCalendarGrant(ctx, userID)
	if err != nil {
		return false, err
	}
	return hasCalendarWriteGrant(found, scopes), nil
}

// CreateMeetEvent books a calendar event with the seeker as an attendee and a Meet link
// requested via conferenceData, the one call in this package that mints the link Book()
// then snapshots onto the booking.
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

// meetAPI does the actual Google Calendar write calls over an already-authenticated
// client, separated from grant resolution so it is testable via httptest exactly as
// calsync.APIReader is — the OAuth handshake is oauth2's own well-tested library code,
// and what this package must get right is the request shape and the response parsing.
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
		return "", "", &gmailsync.APIError{
			Op: "mentor calendar: create event", StatusCode: resp.StatusCode, Status: resp.Status,
		}
	}

	var out meetEvent
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", "", fmt.Errorf("mentor calendar: decode event: %w", err)
	}
	if out.HangoutLink == "" {
		// Google's own docs allow conferenceData.createRequest to still be PENDING when
		// the event insert itself answers 200/201 — the conference is provisioned
		// asynchronously and the link is absent until it resolves. Retrying later is out
		// of scope (see design.md), so the event is kept and the booking's link stays
		// empty exactly as it would on any other failure — this just makes the otherwise
		// silent case visible in logs.
		log.Printf("mentor calendar: event %s created with no hangoutLink yet (conference still pending)", out.ID)
	}
	return out.ID, out.HangoutLink, nil
}

// deleteEvent removes one event. A 404/410 means it is already gone — from the mentor
// deleting it by hand, say — and that is success, not a failure to report: the outcome
// this call exists to produce already holds.
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
		return &gmailsync.APIError{
			Op: "mentor calendar: delete event", StatusCode: resp.StatusCode, Status: resp.Status,
		}
	}
}
