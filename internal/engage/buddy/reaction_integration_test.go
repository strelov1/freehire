//go:build integration

// Integration tests for reactions to feed events. Run with:
// go test -tags=integration ./internal/engage/buddy/ -run React
package buddy_test

import (
	"context"
	"errors"
	"testing"

	"github.com/strelov1/freehire/internal/engage/buddy"
)

// reactionsOn reads the viewer's feed and returns the reactions on one event.
func reactionsOn(t *testing.T, svc *buddy.Service, viewer, eventID int64) buddy.Reactions {
	t.Helper()
	page, err := svc.Feed(context.Background(), viewer, buddy.FeedAll, "", buddy.MaxFeedLimit)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range page.Entries {
		if e.ID == eventID {
			return e.Reactions
		}
	}
	t.Fatalf("event %d not in the feed", eventID)
	return buddy.Reactions{}
}

func TestReactCountsOnePerMemberAndShowsYours(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	author, a, b := seedUser(t, pool, "au@example.test"), seedUser(t, pool, "a@example.test"), seedUser(t, pool, "b@example.test")
	for _, u := range []int64{author, a, b} {
		joinWith(t, pool, svc, u, "", nil, nil)
	}
	ev := event(t, pool, author, newApp(t, pool, author, "acme"), "acme", "stage_set", "offer", 1)

	if _, err := svc.React(ctx, a, ev, buddy.ReactionCongrats); err != nil {
		t.Fatal(err)
	}
	got, err := svc.React(ctx, b, ev, buddy.ReactionCongrats)
	if err != nil {
		t.Fatal(err)
	}
	if got.Counts[buddy.ReactionCongrats] != 2 || got.Mine != buddy.ReactionCongrats {
		t.Fatalf("after two congrats = %+v", got)
	}
	// Choosing another kind replaces b's reaction rather than adding one.
	if _, err := svc.React(ctx, b, ev, buddy.ReactionLuck); err != nil {
		t.Fatal(err)
	}
	r := reactionsOn(t, svc, a, ev)
	if r.Counts[buddy.ReactionCongrats] != 1 || r.Counts[buddy.ReactionLuck] != 1 || r.Mine != buddy.ReactionCongrats {
		t.Fatalf("a sees %+v", r)
	}
	// Clearing removes it.
	if _, err := svc.React(ctx, b, ev, ""); err != nil {
		t.Fatal(err)
	}
	if r := reactionsOn(t, svc, a, ev); r.Counts[buddy.ReactionLuck] != 0 || len(r.Counts) != 1 {
		t.Fatalf("after clearing, a sees %+v", r)
	}
}

// A member can react only to what their own feed shows: never their own event, never one
// across a block, never a hidden one; an unknown kind is refused.
func TestReactOnlyToWhatYourFeedShows(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	author, viewer := seedUser(t, pool, "au@example.test"), seedUser(t, pool, "v@example.test")
	joinWith(t, pool, svc, author, "", nil, nil)
	joinWith(t, pool, svc, viewer, "", nil, nil)
	app := newApp(t, pool, author, "acme")
	ev := event(t, pool, author, app, "acme", "applied", "", 1)

	if _, err := svc.React(ctx, viewer, ev, "love"); !errors.Is(err, buddy.ErrBadReaction) {
		t.Errorf("unknown kind: err = %v, want ErrBadReaction", err)
	}
	if _, err := svc.React(ctx, author, ev, buddy.ReactionCongrats); !errors.Is(err, buddy.ErrNotFound) {
		t.Errorf("own event: err = %v, want ErrNotFound", err)
	}
	if err := svc.HideApplication(ctx, author, app); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.React(ctx, viewer, ev, buddy.ReactionCongrats); !errors.Is(err, buddy.ErrNotFound) {
		t.Errorf("hidden application: err = %v, want ErrNotFound", err)
	}
	if err := svc.ShowApplication(ctx, author, app); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_blocks (blocker_id, blocked_id) VALUES ($1, $2)`, author, viewer); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.React(ctx, viewer, ev, buddy.ReactionCongrats); !errors.Is(err, buddy.ErrNotFound) {
		t.Errorf("blocked: err = %v, want ErrNotFound", err)
	}
}

// A reaction is as visible as the member who gave it: one from a member who left, or from
// someone on either side of a block with the viewer, does not count.
func TestReactionCountsSkipLeftAndBlockedMembers(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	author, viewer, gone, blocked := seedUser(t, pool, "au@example.test"), seedUser(t, pool, "v@example.test"),
		seedUser(t, pool, "g@example.test"), seedUser(t, pool, "b@example.test")
	for _, u := range []int64{author, viewer, gone, blocked} {
		joinWith(t, pool, svc, u, "", nil, nil)
	}
	ev := event(t, pool, author, newApp(t, pool, author, "acme"), "acme", "stage_set", "interview", 1)
	for _, u := range []int64{gone, blocked} {
		if _, err := svc.React(ctx, u, ev, buddy.ReactionLuck); err != nil {
			t.Fatal(err)
		}
	}
	if err := svc.Leave(ctx, gone); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO user_blocks (blocker_id, blocked_id) VALUES ($1, $2)`, viewer, blocked); err != nil {
		t.Fatal(err)
	}
	if r := reactionsOn(t, svc, viewer, ev); len(r.Counts) != 0 {
		t.Fatalf("viewer sees %+v, want no reactions", r)
	}
}

// The author hears about a reaction once per member: a member changing their mind (or
// taking it back and giving it again) adds nothing, another member adds one more. The
// notice names the reaction and the news, never who reacted.
func TestReactNotifiesTheAuthorOncePerMember(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	author, a, b := seedUser(t, pool, "au@example.test"), seedUser(t, pool, "a@example.test"), seedUser(t, pool, "b@example.test")
	for _, u := range []int64{author, a, b} {
		joinWith(t, pool, svc, u, "", nil, nil)
	}
	ev := event(t, pool, author, newApp(t, pool, author, "acme"), "acme", "stage_set", "offer", 1)

	for _, kind := range []string{buddy.ReactionCongrats, buddy.ReactionLuck, "", buddy.ReactionCongrats} {
		if _, err := svc.React(ctx, a, ev, kind); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.React(ctx, b, ev, buddy.ReactionRooting); err != nil {
		t.Fatal(err)
	}

	rows, err := pool.Query(ctx, `SELECT user_id, kind, title, body FROM user_notifications ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type notice struct {
		user              int64
		kind, title, body string
	}
	var got []notice
	for rows.Next() {
		var n notice
		if err := rows.Scan(&n.user, &n.kind, &n.title, &n.body); err != nil {
			t.Fatal(err)
		}
		got = append(got, n)
	}
	if len(got) != 2 {
		t.Fatalf("notices = %+v, want 2 (one per reacting member)", got)
	}
	for _, n := range got {
		if n.user != author || n.kind != "buddy_reaction" {
			t.Fatalf("notice %+v, want kind buddy_reaction for the author", n)
		}
	}
	if got[0].body != `A buddy reacted “Congrats” to your offer from acme.` {
		t.Errorf("first body = %q", got[0].body)
	}
	if got[1].body != `A buddy reacted “Rooting for you” to your offer from acme.` {
		t.Errorf("second body = %q", got[1].body)
	}
}
