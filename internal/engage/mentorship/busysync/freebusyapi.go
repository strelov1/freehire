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

// freeBusyURL reads the mentor's PRIMARY calendar's occupied time only — the same
// "primary calendar alone" rule calsync's eventsURL follows, and for the same reason: a
// person's other calendars are subscriptions, shared team diaries and holidays, not
// their own availability.
const freeBusyURL = "https://www.googleapis.com/calendar/v3/freeBusy"

// APIReader reads busy time through Google's freeBusy endpoint — bounds only, since that
// is all this endpoint can return. See the package doc for why that is the point, not a
// limitation worked around.
type APIReader struct {
	client *http.Client
}

// NewAPIReader wraps a token-bearing client.
func NewAPIReader(client *http.Client) *APIReader { return &APIReader{client: client} }

// ReaderFactoryFor builds the per-mentor reader factory the sync Worker needs, minting a
// token-bearing client from each stored refresh token. The Google OAuth plumbing lives in
// gmailsync's Connector and is not duplicated here.
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

type freeBusyResponse struct {
	Calendars map[string]struct {
		Busy []freeBusyPeriod `json:"busy"`
	} `json:"calendars"`
}

// ListBusy returns the mentor's busy periods over a window, read from the primary
// calendar's free/busy state — no title, no attendee, no identifier: the endpoint has
// nothing else to offer, which is exactly why this worker asks it and not events.list.
func (r *APIReader) ListBusy(ctx context.Context, from, to time.Time) ([]BusyPeriod, error) {
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
		// The status travels with the error rather than being decided here — see
		// calsync.APIReader.page for why: gmailsync.RevokedGrant is the one place a
		// status is read as a revocation, since the flag it sets is shared with every
		// other Google feature this account may use.
		return nil, &gmailsync.APIError{
			Op:         "calendar: freeBusy",
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
		}
	}
	var body freeBusyResponse
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, fmt.Errorf("calendar: decode freeBusy: %w", err)
	}
	primary := body.Calendars["primary"]
	out := make([]BusyPeriod, 0, len(primary.Busy))
	for _, p := range primary.Busy {
		start, err := time.Parse(time.RFC3339, p.Start)
		if err != nil {
			// Every busy period Google documents is a timed instant, never an all-day
			// date-only shape — unlike calsync's events, which genuinely have both. A
			// period here that fails to parse is not a known API variant, so it is
			// dropped rather than failing the whole read (one bad period must not cost
			// a mentor every other one), but logged: a silently dropped period is a
			// silently un-blocked slot, and that must leave a trace somewhere.
			log.Printf("mentor-busy-sync: dropping a busy period with an unparseable start %q", p.Start)
			continue
		}
		end, err := time.Parse(time.RFC3339, p.End)
		if err != nil {
			log.Printf("mentor-busy-sync: dropping a busy period with an unparseable end %q", p.End)
			continue
		}
		out = append(out, BusyPeriod{Start: start, End: end})
	}
	return out, nil
}
