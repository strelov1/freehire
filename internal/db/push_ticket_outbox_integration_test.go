//go:build integration

// Integration tests for push_ticket_outbox's claim-by-age semantics — a
// ticket is only worth checking once it's old enough for Expo to have an
// answer, and this is SQL behavior (interval arithmetic) that can only be
// verified against a real Postgres. Run with:
// go test -tags=integration ./internal/db/
package db

import (
	"context"
	"testing"
)

func TestPushTicketOutboxQueries(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()

	t.Run("a ticket younger than the minimum age is not claimed", func(t *testing.T) {
		if err := q.EnqueuePushTicket(ctx, EnqueuePushTicketParams{
			Token: "ExponentPushToken[young]", TicketID: "ticket-young",
		}); err != nil {
			t.Fatalf("EnqueuePushTicket: %v", err)
		}

		claimed, err := q.ClaimDuePushTickets(ctx, ClaimDuePushTicketsParams{
			MinAgeMinutes: 15, BatchSize: 100,
		})
		if err != nil {
			t.Fatalf("ClaimDuePushTickets: %v", err)
		}
		for _, c := range claimed {
			if c.TicketID == "ticket-young" {
				t.Error("a freshly-enqueued ticket was claimed as due; want it left for a later pass")
			}
		}
	})

	t.Run("a ticket older than the minimum age is claimed", func(t *testing.T) {
		if err := q.EnqueuePushTicket(ctx, EnqueuePushTicketParams{
			Token: "ExponentPushToken[old]", TicketID: "ticket-old",
		}); err != nil {
			t.Fatalf("EnqueuePushTicket: %v", err)
		}
		// Backdate it directly — the queue has no "insert with a past
		// timestamp" query of its own, since nothing else needs one.
		if _, err := pool.Exec(ctx,
			`UPDATE push_ticket_outbox SET created_at = now() - interval '1 hour' WHERE ticket_id = 'ticket-old'`); err != nil {
			t.Fatalf("backdate ticket: %v", err)
		}

		claimed, err := q.ClaimDuePushTickets(ctx, ClaimDuePushTicketsParams{
			MinAgeMinutes: 15, BatchSize: 100,
		})
		if err != nil {
			t.Fatalf("ClaimDuePushTickets: %v", err)
		}
		var found bool
		for _, c := range claimed {
			if c.TicketID == "ticket-old" {
				found = true
				if c.Token != "ExponentPushToken[old]" {
					t.Errorf("token = %q, want ExponentPushToken[old]", c.Token)
				}
			}
		}
		if !found {
			t.Error("an hour-old ticket was not claimed as due")
		}
	})

	t.Run("deleting processed tickets removes them from future claims", func(t *testing.T) {
		if err := q.EnqueuePushTicket(ctx, EnqueuePushTicketParams{
			Token: "ExponentPushToken[gone]", TicketID: "ticket-gone",
		}); err != nil {
			t.Fatalf("EnqueuePushTicket: %v", err)
		}
		if _, err := pool.Exec(ctx,
			`UPDATE push_ticket_outbox SET created_at = now() - interval '1 hour' WHERE ticket_id = 'ticket-gone'`); err != nil {
			t.Fatalf("backdate ticket: %v", err)
		}

		claimed, err := q.ClaimDuePushTickets(ctx, ClaimDuePushTicketsParams{
			MinAgeMinutes: 15, BatchSize: 100,
		})
		if err != nil {
			t.Fatalf("ClaimDuePushTickets: %v", err)
		}
		var ids []int64
		for _, c := range claimed {
			ids = append(ids, c.ID)
		}
		if err := q.DeletePushTickets(ctx, ids); err != nil {
			t.Fatalf("DeletePushTickets: %v", err)
		}

		var rows int
		if err := pool.QueryRow(ctx, `SELECT count(*) FROM push_ticket_outbox WHERE ticket_id = 'ticket-gone'`).Scan(&rows); err != nil {
			t.Fatalf("count: %v", err)
		}
		if rows != 0 {
			t.Errorf("ticket-gone rows = %d, want 0 after delete", rows)
		}
	})
}
