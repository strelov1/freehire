//go:build integration

// Integration tests for who a conversation still shows, and for the guards that live in
// the writes. Run with: go test -tags=integration ./internal/engage/buddy/
package buddy_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/strelov1/freehire/internal/engage/buddy"
	"github.com/strelov1/freehire/internal/engage/messaging"
)

func assertFormerMember(t *testing.T, pool *pgxpool.Pool, viewer, convID int64) {
	t.Helper()
	list, err := chat(pool).List(context.Background(), viewer)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != convID {
		t.Fatalf("list = %+v", list)
	}
	got := list[0]
	if got.Counterpart != (messaging.Counterpart{Persona: buddy.PersonaName}) {
		t.Fatalf("conversation must be a bare former member, got %+v", got)
	}
}

func TestLeftMemberIsAFormerMemberToTheOtherSide(t *testing.T) {
	pool, svc, a, b, id := openChat(t)
	ctx := context.Background()
	seedCV(t, pool, b)
	bHandle := handleOf(t, svc, b)
	if err := svc.Leave(ctx, b); err != nil {
		t.Fatal(err)
	}
	assertFormerMember(t, pool, a, id)
	if _, err := svc.Profile(ctx, a, bHandle); !errors.Is(err, buddy.ErrNotFound) {
		t.Fatalf("profile of a left member = %v, want ErrNotFound", err)
	}
}

func TestLeftMemberDeclinedInvitationDoesNotLeakToTheInitiator(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	a, _ := member(t, pool, svc, "a@example.test")
	b, bHandle := member(t, pool, svc, "b@example.test")
	c, err := svc.Invite(ctx, a, bHandle, "hi")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := chat(pool).Decline(ctx, b, c.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.Leave(ctx, b); err != nil {
		t.Fatal(err)
	}
	assertFormerMember(t, pool, a, c.ID)
}

func TestBlockedPairAreFormerMembersToEachOther(t *testing.T) {
	pool, _, a, b, id := openChat(t)
	if err := chat(pool).Block(context.Background(), a, id); err != nil {
		t.Fatal(err)
	}
	assertFormerMember(t, pool, a, id)
	assertFormerMember(t, pool, b, id)
}

func TestConversationListCarriesTheOtherCard(t *testing.T) {
	pool, svc, a, b, _ := openChat(t)
	seedCV(t, pool, a)
	list, err := chat(pool).List(context.Background(), b)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || !list[0].Counterpart.Reachable || list[0].Counterpart.Persona != buddy.PersonaName {
		t.Fatalf("list = %+v", list)
	}
	d := list[0].Counterpart.Detail.(buddy.Counterpart)
	if d.Handle != handleOf(t, svc, a) || d.Card.Category == "" {
		t.Fatalf("counterpart = %+v", d)
	}
}

// A member who has left sees their old conversations as history only: nobody on the other
// side is drawn for them either.
func TestLeftViewerSeesNoCounterpart(t *testing.T) {
	pool, svc, _, b, id := openChat(t)
	if err := svc.Leave(context.Background(), b); err != nil {
		t.Fatal(err)
	}
	assertFormerMember(t, pool, b, id)
}
