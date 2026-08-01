package calsync

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/strelov1/freehire/internal/gmailsync"
)

// eventsURL reads the candidate's primary calendar. Only the primary one: a person's
// other calendars are subscriptions, shared team diaries and holidays, and an interview
// they accepted lands here.
const eventsURL = "https://www.googleapis.com/calendar/v3/calendars/primary/events"

// maxEvents bounds one page. A ±90-day window on a working calendar fits comfortably;
// the cap is here so a shared diary cannot make one candidate's sync unbounded.
const maxEvents = 250

// APIReader reads meetings through the Google Calendar API.
type APIReader struct {
	client *http.Client
}

// NewAPIReader wraps a token-bearing client.
func NewAPIReader(client *http.Client) *APIReader { return &APIReader{client: client} }

// ReaderFactoryFor builds the per-candidate reader factory the worker needs, minting a
// token-bearing client from each stored refresh token. The Google OAuth plumbing lives in
// gmailsync's Connector and is not duplicated here — one grant covers both scopes.
func ReaderFactoryFor(c *gmailsync.Connector) ReaderFactory {
	return func(ctx context.Context, refreshToken string) CalendarReader {
		return NewAPIReader(c.HTTPClient(ctx, refreshToken))
	}
}

// calendarEvent is the slice of the API response this package reads. Everything absent
// from it is absent deliberately: attendees, description and location describe a person's
// life rather than their job search, and nothing here needs them.
type calendarEvent struct {
	ICalUID     string `json:"iCalUID"`
	Summary     string `json:"summary"`
	Status      string `json:"status"`
	HangoutLink string `json:"hangoutLink"`
	Organizer   struct {
		Email string `json:"email"`
	} `json:"organizer"`
	Start calendarTime `json:"start"`
	End   calendarTime `json:"end"`
}

// calendarTime is Google's either/or: a timed event carries dateTime, an all-day one
// carries date. Reading only the first would silently drop every all-day entry.
type calendarTime struct {
	DateTime string `json:"dateTime"`
	Date     string `json:"date"`
}

func (t calendarTime) parse() time.Time {
	if t.DateTime != "" {
		if at, err := time.Parse(time.RFC3339, t.DateTime); err == nil {
			return at
		}
	}
	if t.Date != "" {
		// An all-day entry has no clock. Midnight UTC is a placeholder the reader's own
		// timezone will move; the calendar view groups by local day anyway.
		if at, err := time.Parse("2006-01-02", t.Date); err == nil {
			return at
		}
	}
	return time.Time{}
}

// ListEvents returns the candidate's meetings over a window.
//
// singleEvents expands a recurring series into its occurrences, so a weekly slot does not
// arrive as one entry the candidate would see on the wrong day. showDeleted keeps
// cancellations, which are facts about meetings already stored rather than absences.
//
// Note on a series: Google's identifier for expanded occurrences is not guaranteed
// distinct across them, and the stored row is keyed on it. A repeating interview may
// therefore collapse to one row. Interview rounds are separate meetings in practice, so
// this is left as it falls rather than worked around on a guess.
func (r *APIReader) ListEvents(ctx context.Context, from, to time.Time) ([]Meeting, error) {
	q := url.Values{}
	q.Set("timeMin", from.Format(time.RFC3339))
	q.Set("timeMax", to.Format(time.RFC3339))
	q.Set("singleEvents", "true")
	q.Set("showDeleted", "true")
	q.Set("orderBy", "startTime")
	q.Set("maxResults", fmt.Sprint(maxEvents))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, eventsURL+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calendar: list events: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("calendar: list events: %s", resp.Status)
	}

	var body struct {
		Items []calendarEvent `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("calendar: decode events: %w", err)
	}

	out := make([]Meeting, 0, len(body.Items))
	for _, it := range body.Items {
		start := it.Start.parse()
		if it.ICalUID == "" || start.IsZero() {
			// Without an identifier there is nothing to match against and nothing to key
			// a row on; without a start there is no day to show it on.
			continue
		}
		out = append(out, Meeting{
			UID:       it.ICalUID,
			Title:     it.Summary,
			Organizer: it.Organizer.Email,
			StartsAt:  start,
			EndsAt:    it.End.parse(),
			JoinURL:   it.HangoutLink,
			Cancelled: it.Status == "cancelled",
		})
	}
	return out, nil
}
