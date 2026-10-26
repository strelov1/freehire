//go:build integration

// Shared setup for the messaging integration tests. Every conversation here is of the one
// stored kind with both participants speaking as the one stored persona, resolved by stubs,
// so the core is exercised without any feature's own rules on top.
// Run with: go test -tags=integration ./internal/engage/messaging/
package messaging_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/engage/messaging"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// stubPersona draws every account speaking as it as reachable, with its account id as the
// detail, unless it is in hidden; it names an account "user-<id>". (A real persona must
// never hand out the account id — the stub does so only to make the test's assertions
// easy to read.)
type stubPersona struct {
	hidden map[int64]bool
}

func (stubPersona) Name() string { return "buddy" }

func (p stubPersona) Counterparts(_ context.Context, _ int64, ids []int64) (map[int64]messaging.Counterpart, error) {
	out := make(map[int64]messaging.Counterpart, len(ids))
	for _, id := range ids {
		out[id] = messaging.Counterpart{Reachable: !p.hidden[id], Detail: id}
	}
	return out, nil
}

func (stubPersona) Names(_ context.Context, ids []int64) (map[int64]string, error) {
	out := make(map[int64]string, len(ids))
	for _, id := range ids {
		out[id] = fmt.Sprintf("user-%d", id)
	}
	return out, nil
}

// stubKind words the notices of the one stored kind.
type stubKind struct{}

func (stubKind) Name() string { return "buddy" }

func (stubKind) Notice(id int64, pending bool) messaging.Notice {
	n := messaging.Notice{
		Title: "You have a new message", Line: "You have a new message.",
		Path: fmt.Sprintf("/my/inbox?c=%d", id), NotificationKind: "stub_message", Footer: "Because you joined.",
	}
	if pending {
		n.Title, n.Line, n.NotificationKind = "You have a new request", "You have a new request.", "stub_request"
	}
	return n
}

func setup(t *testing.T) (*pgxpool.Pool, *messaging.Service) {
	t.Helper()
	pool := testdb.Pool(t)
	return pool, messaging.New(messaging.NewQueriesRepository(db.New(pool)), stubPersona{})
}

func seedUser(t *testing.T, pool *pgxpool.Pool, email string) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(), `INSERT INTO users (email) VALUES ($1) RETURNING id`, email).Scan(&id); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	return id
}

func countRows(t *testing.T, pool *pgxpool.Pool, query string, args ...any) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// pairKey is one conversation per pair, whoever starts it — the rule a two-person kind sets.
func pairKey(a, b int64) string { return fmt.Sprintf("%d:%d", min(a, b), max(a, b)) }

// start opens a conversation of the stub kind from one account to another.
func start(ctx context.Context, svc *messaging.Service, from, to int64, body string) (messaging.Conversation, error) {
	return svc.Start(ctx, messaging.StartRequest{
		Kind: "buddy", StarterID: from, StarterPersona: "buddy", RecipientID: to, RecipientPersona: "buddy",
		PairKey: pairKey(from, to), Body: body,
	})
}

// openChat seeds two accounts and an accepted conversation between them, A having started it.
func openChat(t *testing.T) (pool *pgxpool.Pool, svc *messaging.Service, a, b, convID int64) {
	t.Helper()
	pool, svc = setup(t)
	ctx := context.Background()
	a = seedUser(t, pool, "a@example.test")
	b = seedUser(t, pool, "b@example.test")
	c, err := start(ctx, svc, a, b, "hello")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Accept(ctx, b, c.ID); err != nil {
		t.Fatal(err)
	}
	return pool, svc, a, b, c.ID
}
