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

// maxEvents bounds ONE PAGE, not the window. With singleEvents a recurring meeting
// expands into an instance per occurrence, so a single daily standup is ~180 rows over a
// ±90-day window and an ordinary working calendar is several hundred. Reading one page
// and stopping would have truncated at the OLDEST end — the results come back ascending
// from now-90d — so the future would simply never arrive, silently, in the feature whose
// entire purpose is to show it.
const maxEvents = 250

// maxPages caps a run so one shared diary cannot spin forever. Enough for thousands of
// entries; a calendar past it is not one we can usefully read anyway.
const maxPages = 20

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
	// ID is Google's own event id, and the only field a deleted event is guaranteed to
	// carry. It is not the meeting's identity across systems — that is iCalUID — but it
	// is enough to say WHICH stored meeting was called off.
	ID          string `json:"id"`
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
	var items []calendarEvent
	for page, token := 0, ""; page < maxPages; page++ {
		q := url.Values{}
		q.Set("timeMin", from.Format(time.RFC3339))
		q.Set("timeMax", to.Format(time.RFC3339))
		q.Set("singleEvents", "true")
		q.Set("showDeleted", "true")
		q.Set("orderBy", "startTime")
		q.Set("maxResults", fmt.Sprint(maxEvents))
		if token != "" {
			q.Set("pageToken", token)
		}

		body, err := r.page(ctx, q)
		if err != nil {
			return nil, err
		}
		items = append(items, body.Items...)
		if body.NextPageToken == "" {
			break
		}
		token = body.NextPageToken
	}

	out := make([]Meeting, 0, len(items))
	for _, it := range items {
		start := it.Start.parse()
		cancelled := it.Status == "cancelled"
		if cancelled {
			// A cancellation is a fact about a meeting ALREADY stored, so it needs
			// neither a time nor a title — and Google does not send them. Its own
			// documentation says a deleted event is only guaranteed to carry `id`, and a
			// cancelled occurrence of a series only `id`, `recurringEventId` and
			// `originalStartTime`. The live-event rules below would have discarded every
			// one of them before this flag was ever read, so the cancellation path was
			// dead: a called-off interview stood on the calendar forever.
			if it.ICalUID != "" || it.ID != "" {
				out = append(out, Meeting{
					UID: it.ICalUID, ProviderID: it.ID, StartsAt: start, Cancelled: true,
				})
			}
			continue
		}
		if it.ICalUID == "" || start.IsZero() {
			// A live meeting does need both: without an identifier there is nothing to
			// match against and nothing to key a row on, and without a start there is no
			// day to show it on.
			continue
		}
		out = append(out, Meeting{
			UID:        it.ICalUID,
			ProviderID: it.ID,
			Title:      it.Summary,
			Organizer:  it.Organizer.Email,
			StartsAt:   start,
			EndsAt:     it.End.parse(),
			JoinURL:    it.HangoutLink,
			Cancelled:  it.Status == "cancelled",
		})
	}
	return out, nil
}

// eventsPage is one response page.
type eventsPage struct {
	Items         []calendarEvent `json:"items"`
	NextPageToken string          `json:"nextPageToken"`
}

func (r *APIReader) page(ctx context.Context, q url.Values) (eventsPage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, eventsURL+"?"+q.Encode(), nil)
	if err != nil {
		return eventsPage{}, err
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return eventsPage{}, fmt.Errorf("calendar: list events: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return eventsPage{}, fmt.Errorf("calendar: list events: %s", resp.Status)
	}
	var body eventsPage
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return eventsPage{}, fmt.Errorf("calendar: decode events: %w", err)
	}
	return body, nil
}
