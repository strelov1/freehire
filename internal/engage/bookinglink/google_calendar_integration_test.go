//go:build integration

package bookinglink

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/application/gmailsync"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
	"github.com/strelov1/freehire/internal/platform/tokencrypt"
)

type calendarTransport func(*http.Request) (*http.Response, error)

func (f calendarTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func TestGoogleCalendarUsesOnlyTheOwnersUsableCalendarGrant(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	q := db.New(pool)
	var userID int64
	if err := pool.QueryRow(ctx, "INSERT INTO users (email) VALUES ('calendar-owner@example.test') RETURNING id").Scan(&userID); err != nil {
		t.Fatal(err)
	}
	cipher, err := tokencrypt.New(make([]byte, 32))
	if err != nil {
		t.Fatal(err)
	}
	enc, err := cipher.Encrypt("test-refresh")
	if err != nil {
		t.Fatal(err)
	}

	calendar := NewGoogleCalendar(q, gmailsync.NewConnector("test-id", "test-secret", "https://freehire.test"), cipher)
	calendar.client = func(_ context.Context, refresh string) *http.Client {
		if refresh != "test-refresh" {
			t.Errorf("unexpected refresh token")
		}
		return &http.Client{Transport: calendarTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(strings.NewReader(
				`{"calendars":{"primary":{"busy":[{"start":"2026-11-18T09:10:00Z","end":"2026-11-18T09:20:00Z"}]}}}`,
			))}, nil
		})}
	}

	from := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	to := from.Add(30 * time.Minute)
	if busy, err := calendar.Busy(ctx, userID, from, to); err != nil || len(busy) != 0 {
		t.Fatalf("no grant: busy = %v, err = %v", busy, err)
	}
	grant := db.UpsertGmailConnectionParams{
		UserID: userID, Email: "calendar-owner@example.test", RefreshTokenEnc: enc,
		Scopes: []string{gmailsync.GmailReadonlyScope},
	}
	if err := q.UpsertGmailConnection(ctx, grant); err != nil {
		t.Fatal(err)
	}
	if busy, err := calendar.Busy(ctx, userID, from, to); err != nil || len(busy) != 0 {
		t.Fatalf("mail-only grant: busy = %v, err = %v", busy, err)
	}

	grant.Scopes = append(grant.Scopes, gmailsync.CalendarScope)
	if err := q.UpsertGmailConnection(ctx, grant); err != nil {
		t.Fatal(err)
	}
	busy, err := calendar.Busy(ctx, userID, from, to)
	if err != nil || len(busy) != 1 || !busy[0].Start.Equal(from.Add(10*time.Minute)) ||
		!busy[0].End.Equal(from.Add(20*time.Minute)) {
		t.Fatalf("calendar grant: busy = %v, err = %v", busy, err)
	}
	if err := q.SetGmailStatus(ctx, db.SetGmailStatusParams{UserID: userID, Status: "needs_reconsent"}); err != nil {
		t.Fatal(err)
	}
	if busy, err := calendar.Busy(ctx, userID, from, to); err == nil || len(busy) != 0 {
		t.Fatalf("revoked calendar grant: busy = %v, err = %v", busy, err)
	}
}
