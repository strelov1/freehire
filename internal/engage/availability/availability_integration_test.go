//go:build integration

package availability_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/engage/availability"
	"github.com/strelov1/freehire/internal/engage/slots"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestStoreRoundTripAndOneZone(t *testing.T) {
	ctx := context.Background()
	pool := testdb.Pool(t)
	store := availability.New(db.New(pool))

	var user int64
	if err := pool.QueryRow(ctx, `INSERT INTO users (email) VALUES ('avail@example.test') RETURNING id`).Scan(&user); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{
		`INSERT INTO buddy_members (user_id, handle) VALUES ($1, 'avail')`,
		`INSERT INTO buddy_mock_settings (user_id, timezone, meeting_url, topics) VALUES ($1, 'UTC', 'https://meet.example/x', '{coding}')`,
	} {
		if _, err := pool.Exec(ctx, q, user); err != nil {
			t.Fatal(err)
		}
	}

	sc, err := store.Load(ctx, user)
	if err != nil || sc.Settings != nil || len(sc.Rules) != 0 {
		t.Fatalf("fresh user = %+v, %v; want nothing saved", sc, err)
	}

	clock := func(h int) slots.TimeOfDay { c, _ := slots.NewTimeOfDay(h, 0); return c }
	mon, _ := slots.NewWeeklyRule(time.Monday, clock(9), clock(12))
	tue, _ := slots.NewWeeklyRule(time.Tuesday, clock(13), clock(17))
	if err := store.ReplaceWeekly(ctx, user, []slots.Rule{mon, tue}); err != nil {
		t.Fatal(err)
	}
	day, _ := slots.NewDate(2030, time.January, 7)
	closed, _ := slots.NewDatedRule(day, clock(0), clock(0))
	added, err := store.Add(ctx, user, closed)
	if err != nil {
		t.Fatal(err)
	}

	// FollowZone with no settings row changes nothing; SetZone creates it and moves the
	// product zone with it.
	if err := store.FollowZone(ctx, user, "Asia/Tokyo"); err != nil {
		t.Fatal(err)
	}
	if sc, _ := store.Load(ctx, user); sc.Settings != nil {
		t.Fatalf("FollowZone created settings: %+v", sc)
	}
	if err := store.SetZone(ctx, user, "Europe/Berlin"); err != nil {
		t.Fatal(err)
	}
	var buddyZone string
	if err := pool.QueryRow(ctx, `SELECT timezone FROM buddy_mock_settings WHERE user_id = $1`, user).Scan(&buddyZone); err != nil || buddyZone != "Europe/Berlin" {
		t.Fatalf("buddy zone = %q, %v; want it to follow the shared zone", buddyZone, err)
	}

	if err := store.SaveSettings(ctx, user, "Europe/Berlin", availability.Settings{Duration: 20 * time.Minute}); !errors.Is(err, availability.ErrInvalidSettings) {
		t.Fatalf("20 min = %v; want ErrInvalidSettings", err)
	}
	if err := store.SaveSettings(ctx, user, "Europe/Berlin", availability.Settings{Duration: time.Hour, Notice: 48 * time.Hour}); err != nil {
		t.Fatal(err)
	}

	sc, err = store.Load(ctx, user)
	if err != nil || len(sc.Rules) != 3 || sc.Zone != "Europe/Berlin" ||
		sc.Settings == nil || sc.Settings.Duration != time.Hour || sc.Settings.Notice != 48*time.Hour {
		t.Fatalf("saved schedule = %+v, %v", sc, err)
	}

	if ok, err := store.Delete(ctx, user, added.ID()); err != nil || !ok {
		t.Fatalf("delete own rule = %v, %v", ok, err)
	}
	if ok, _ := store.Delete(ctx, user+1, added.ID()); ok {
		t.Fatal("deleted a rule twice / of another user")
	}

	busy, err := store.Busy(ctx, []int64{user}, time.Now(), time.Now().Add(time.Hour))
	if err != nil || len(busy[user]) != 0 || len(availability.Intervals(busy[user])) != 0 {
		t.Fatalf("busy = %v, %v; want none", busy, err)
	}
}
