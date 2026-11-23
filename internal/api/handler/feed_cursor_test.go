package handler

import (
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/search/feed"
)

func TestFeed_ChangedProfileOnAnOffsetZeroRestCursorIsSourceChanged(t *testing.T) {
	seen := time.Now().Add(-time.Hour).Truncate(time.Second)
	fake := &scriptedSearcher{newHits: docs("n1", "n2"), restHits: docs("r1")}
	app, iss := feedStateApp(fake, &feedExcludedAt{}, feedUpdated, &feedStateFake{seen: &seen})
	_, body := deckGet(t, app, iss, "/api/v1/me/feed?limit=2")
	cur := nextCursorOf(body)
	edited, iss2 := feedStateApp(fake, &feedExcludedAt{}, feedUpdated.Add(time.Second), &feedStateFake{seen: &seen})
	fake.calls = nil
	_, body = deckGet(t, edited, iss2, "/api/v1/me/feed?limit=2&cursor="+cur)
	if body["meta"].(map[string]any)["reason"] != feedReasonSourceChanged || len(fake.calls) != 0 {
		t.Errorf("meta=%v calls=%d", body["meta"], len(fake.calls))
	}
}

func TestFeed_CursorWithoutPhaseIs400(t *testing.T) {
	app, iss := feedPagingApp(&fakeSearcher{}, &feedExcludedAt{}, feedUpdated)
	now := time.Now()
	old := feed.NewSigner([]byte("cursor-key")).Sign(feed.Cursor{UserID: 7, Source: "profile", SnapshotAt: now.Unix(), Offset: 20, ExpiresAt: now.Add(time.Hour).Unix()})
	if status, _ := deckGet(t, app, iss, "/api/v1/me/feed?cursor="+old); status != fiber.StatusBadRequest {
		t.Errorf("status = %d, want 400", status)
	}
}
