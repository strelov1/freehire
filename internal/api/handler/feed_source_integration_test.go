//go:build integration

package handler

import (
	"context"
	"fmt"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestFeedSourceRejectsIneligibleSavedSearches(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	q := db.New(pool)

	newUser := func(email string) int64 {
		t.Helper()
		var id int64
		if err := pool.QueryRow(ctx, "INSERT INTO users (email) VALUES ($1) RETURNING id", email).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	owner := newUser("feed-owner@example.test")
	other := newUser("feed-other@example.test")

	createSearch := func(userID int64, name string, derived bool) int64 {
		t.Helper()
		row, err := q.CreateSavedSearch(ctx, db.CreateSavedSearchParams{
			UserID: userID, Name: name, Query: "q=go", DerivedFromProfile: derived,
		})
		if err != nil {
			t.Fatal(err)
		}
		return row.ID
	}
	active := createSearch(owner, "Active", false)
	foreign := createSearch(other, "Foreign", false)
	derived := createSearch(owner, "Derived", true)
	app, issuer := feedStateApp(oneJob(), &feedExcludedAt{}, feedUpdated, feedStateDB{q: q})
	pick := func(id int64) int {
		return putFeedSource(t, app, issuer, owner, fmt.Sprintf(`{"source":"saved_search","saved_search_id":%d}`, id))
	}
	if status := pick(active); status != fiber.StatusNoContent {
		t.Fatalf("pick owned search: status = %d", status)
	}
	for name, id := range map[string]int64{"foreign": foreign, "profile-derived": derived} {
		t.Run(name, func(t *testing.T) {
			if status := pick(id); status != fiber.StatusNotFound {
				t.Errorf("status = %d, want 404", status)
			}
			chosen, err := q.GetFeedSource(ctx, owner)
			if err != nil || !chosen.Valid || chosen.Int64 != active {
				t.Fatalf("chosen source = %+v, %v; want %d", chosen, err, active)
			}
		})
	}
}
