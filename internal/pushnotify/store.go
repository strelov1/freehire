package pushnotify

import (
	"context"

	"github.com/strelov1/freehire/internal/db"
)

// QueriesStore adapts *db.Queries to the TokenPruner/TicketQueuer/TicketStore
// seams ExpoNotifier needs, translating between this package's plain-typed
// interfaces and db's generated Params/model types.
type QueriesStore struct {
	q *db.Queries
}

// NewQueriesStore wraps q as a QueriesStore.
func NewQueriesStore(q *db.Queries) *QueriesStore {
	return &QueriesStore{q: q}
}

func (s *QueriesStore) PruneDeadPushToken(ctx context.Context, token string) error {
	return s.q.PruneDeadPushToken(ctx, token)
}

func (s *QueriesStore) EnqueuePushTicket(ctx context.Context, token, ticketID string) error {
	return s.q.EnqueuePushTicket(ctx, db.EnqueuePushTicketParams{Token: token, TicketID: ticketID})
}

func (s *QueriesStore) ClaimDuePushTickets(ctx context.Context, minAgeMinutes, batchSize int32) ([]Ticket, error) {
	rows, err := s.q.ClaimDuePushTickets(ctx, db.ClaimDuePushTicketsParams{
		MinAgeMinutes: minAgeMinutes,
		BatchSize:     batchSize,
	})
	if err != nil {
		return nil, err
	}
	tickets := make([]Ticket, len(rows))
	for i, r := range rows {
		tickets[i] = Ticket{ID: r.ID, Token: r.Token, TicketID: r.TicketID}
	}
	return tickets, nil
}

func (s *QueriesStore) DeletePushTickets(ctx context.Context, ids []int64) error {
	return s.q.DeletePushTickets(ctx, ids)
}
