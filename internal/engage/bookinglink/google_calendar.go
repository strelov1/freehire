package bookinglink

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/application/gmailsync"
	"github.com/strelov1/freehire/internal/engage/mentorship/busysync"
	"github.com/strelov1/freehire/internal/engage/slots"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/tokencrypt"
)

const calendarTimeout = 5 * time.Second

// GoogleCalendar reads bounds from the candidate's primary Google Calendar.
type GoogleCalendar struct {
	queries *db.Queries
	client  func(context.Context, string) *http.Client
	cipher  *tokencrypt.Cipher
}

func NewGoogleCalendar(queries *db.Queries, connector *gmailsync.Connector, cipher *tokencrypt.Cipher) *GoogleCalendar {
	return &GoogleCalendar{queries: queries, client: connector.HTTPClient, cipher: cipher}
}

func (c *GoogleCalendar) Busy(ctx context.Context, userID int64, from, to time.Time) ([]slots.Interval, error) {
	ctx, cancel := context.WithTimeout(ctx, calendarTimeout)
	defer cancel()

	grant, err := c.queries.GetGoogleGrant(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read calendar grant: %w", err)
	}
	if !slices.Contains(grant.Scopes, gmailsync.CalendarScope) {
		return nil, nil
	}
	if grant.Status != "connected" {
		return nil, errors.New("calendar grant needs reconnection")
	}

	refreshToken, err := c.cipher.Decrypt(grant.RefreshTokenEnc)
	if err != nil {
		return nil, fmt.Errorf("decrypt calendar grant: %w", err)
	}
	reader := busysync.NewAPIReader(c.client(ctx, refreshToken))
	periods, err := reader.ListBusyStrict(ctx, from, to)
	if err != nil {
		return nil, err
	}
	busy := make([]slots.Interval, 0, len(periods))
	for _, period := range periods {
		busy = append(busy, slots.Interval{Start: period.Start, End: period.End})
	}
	return busy, nil
}
