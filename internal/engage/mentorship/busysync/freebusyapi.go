package busysync

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/strelov1/freehire/internal/application/gmailsync"
)

// freeBusyURL reads the mentor's primary calendar only, as calsync does: other calendars
// are subscriptions, shared diaries and holidays, not their own availability.
const freeBusyURL = "https://www.googleapis.com/calendar/v3/freeBusy"

// APIReader reads busy time through Google's freeBusy endpoint (bounds only, by design).
type APIReader struct {
	client *http.Client
}

// NewAPIReader wraps a token-bearing client.
func NewAPIReader(client *http.Client) *APIReader { return &APIReader{client: client} }

// ReaderFactoryFor builds the per-mentor reader factory the sync Worker needs, minting a
// token-bearing client from each stored refresh token via gmailsync's Connector.
func ReaderFactoryFor(c *gmailsync.Connector) ReaderFactory {
	return func(ctx context.Context, refreshToken string) FreeBusyReader {
		return NewAPIReader(c.HTTPClient(ctx, refreshToken))
	}
}

type freeBusyRequest struct {
	TimeMin string           `json:"timeMin"`
	TimeMax string           `json:"timeMax"`
	Items   []freeBusyItemID `json:"items"`
}

type freeBusyItemID struct {
	ID string `json:"id"`
}

type freeBusyPeriod struct {
	Start string `json:"start"`
	End   string `json:"end"`
}

// freeBusyError is Google's shape for a calendar it could not read: the request succeeds
// (200) but the busy[] beside it is not a real answer.
type freeBusyError struct {
	Reason string `json:"reason"`
}

type freeBusyResponse struct {
	Calendars map[string]struct {
		Busy   []freeBusyPeriod `json:"busy"`
		Errors []freeBusyError  `json:"errors"`
	} `json:"calendars"`
}

// ListBusy returns the mentor's busy periods over a window from the primary calendar's
// free/busy state: no title, attendee or identifier.
func (r *APIReader) ListBusy(ctx context.Context, from, to time.Time) ([]BusyPeriod, error) {
	return r.listBusy(ctx, from, to, false)
}

// ListBusyStrict requires a complete primary-calendar answer for live bookings.
func (r *APIReader) ListBusyStrict(ctx context.Context, from, to time.Time) ([]BusyPeriod, error) {
	return r.listBusy(ctx, from, to, true)
}

func (r *APIReader) listBusy(ctx context.Context, from, to time.Time, strict bool) ([]BusyPeriod, error) {
	reqBody, err := json.Marshal(freeBusyRequest{
		TimeMin: from.Format(time.RFC3339),
		TimeMax: to.Format(time.RFC3339),
		Items:   []freeBusyItemID{{ID: "primary"}},
	})
	if err != nil {
		return nil, fmt.Errorf("calendar: encode freeBusy request: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, freeBusyURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("calendar: freeBusy: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// The status travels with the error: gmailsync.RevokedGrant is the one place a status
		// is read as a revocation (see calsync.APIReader.page).
		return nil, gmailsync.NewAPIError("calendar: freeBusy", resp)
	}
	var body freeBusyResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("calendar: decode freeBusy: %w", err)
	}
	primary := body.Calendars["primary"]
	if len(primary.Errors) > 0 {
		// A 200 with an empty busy[] is not "nothing is busy": Google could not read the
		// calendar, and reconciling on it would clear every stored conflict. Not a
		// gmailsync.APIError, so it is an ordinary sync failure, not a revocation.
		return nil, fmt.Errorf("calendar: freeBusy could not read the primary calendar: %s", primary.Errors[0].Reason)
	}
	if strict && primary.Busy == nil {
		return nil, fmt.Errorf("calendar: free/busy response omitted the primary calendar's busy periods")
	}
	out := make([]BusyPeriod, 0, len(primary.Busy))
	for _, p := range primary.Busy {
		start, startErr := time.Parse(time.RFC3339, p.Start)
		end, endErr := time.Parse(time.RFC3339, p.End)
		if strict && (startErr != nil || endErr != nil || !end.After(start)) {
			return nil, fmt.Errorf("calendar: invalid free/busy period")
		}
		if startErr != nil {
			// Busy periods are always timed instants, never date-only. One that fails to parse
			// is dropped, not fatal (it must not cost the mentor every other period), but
			// logged: a dropped period is a silently un-blocked slot.
			log.Printf("mentor-busy-sync: dropping a busy period with an unparseable start %q", p.Start)
			continue
		}
		if endErr != nil {
			log.Printf("mentor-busy-sync: dropping a busy period with an unparseable end %q", p.End)
			continue
		}
		out = append(out, BusyPeriod{Start: start, End: end})
	}
	return out, nil
}
