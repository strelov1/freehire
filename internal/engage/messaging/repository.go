package messaging

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgerr"
)

var _ Repository = (*QueriesRepository)(nil)

// QueriesRepository is the production Repository over sqlc's *db.Queries.
type QueriesRepository struct {
	q *db.Queries
}

// NewQueriesRepository builds a QueriesRepository.
func NewQueriesRepository(q *db.Queries) *QueriesRepository { return &QueriesRepository{q: q} }

func rowFrom(c db.Conversation) ConversationRow {
	out := ConversationRow{
		ID: c.ID, Kind: c.Kind, StarterID: c.StarterID, Status: c.Status, CreatedAt: c.CreatedAt.Time,
	}
	if c.AcceptedAt.Valid {
		out.AcceptedAt = &c.AcceptedAt.Time
	}
	return out
}

func pairKey(k string) pgtype.Text { return pgtype.Text{String: k, Valid: k != ""} }

func (r *QueriesRepository) Start(ctx context.Context, req StartRequest) (ConversationRow, error) {
	row, err := r.q.StartConversation(ctx, db.StartConversationParams{
		Kind: req.Kind, PairKey: pairKey(req.PairKey), StarterID: req.StarterID, StarterPersona: req.StarterPersona,
		RecipientID: req.RecipientID, RecipientPersona: req.RecipientPersona, Body: req.Body,
	})
	if err == nil {
		return rowFrom(db.Conversation(row)), nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		// The statement guards the block table itself.
		return ConversationRow{}, ErrUnavailable
	}
	if req.PairKey == "" || !pgerr.IsUniqueViolation(err) {
		return ConversationRow{}, err
	}
	return r.reopen(ctx, req)
}

// reopen answers a start on a pair key that already has a conversation: a closed one
// without a block comes back as pending, anything else stays as it is.
func (r *QueriesRepository) reopen(ctx context.Context, req StartRequest) (ConversationRow, error) {
	row, err := r.q.ReopenConversation(ctx, db.ReopenConversationParams{
		Kind: req.Kind, PairKey: req.PairKey, StarterID: req.StarterID,
		RecipientID: req.RecipientID, Body: req.Body,
	})
	if err == nil {
		return rowFrom(db.Conversation(row)), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ConversationRow{}, err
	}
	// Nothing matched: still closed means a block (not revealed), any other status means
	// the pair is already talking or was declined.
	cur, ok, err := r.ConversationByPairKey(ctx, req.Kind, req.PairKey)
	if err != nil {
		return ConversationRow{}, err
	}
	if !ok || cur.Status == StatusClosed {
		return ConversationRow{}, ErrUnavailable
	}
	return ConversationRow{}, ErrExists
}

func (r *QueriesRepository) ConversationByPairKey(ctx context.Context, kind, key string) (ConversationRow, bool, error) {
	row, err := r.q.GetConversationByPairKey(ctx, db.GetConversationByPairKeyParams{Kind: kind, PairKey: pairKey(key)})
	return found(row, err)
}

func (r *QueriesRepository) ConversationFor(ctx context.Context, userID, conversationID int64) (ConversationRow, bool, error) {
	row, err := r.q.GetConversationForUser(ctx, db.GetConversationForUserParams{UserID: userID, ID: conversationID})
	return found(row, err)
}

func found(row db.Conversation, err error) (ConversationRow, bool, error) {
	if errors.Is(err, pgx.ErrNoRows) {
		return ConversationRow{}, false, nil
	}
	if err != nil {
		return ConversationRow{}, false, err
	}
	return rowFrom(row), true, nil
}

func (r *QueriesRepository) Participants(ctx context.Context, conversationID int64) ([]Participant, error) {
	rows, err := r.q.ListConversationParticipants(ctx, conversationID)
	if err != nil {
		return nil, err
	}
	out := make([]Participant, len(rows))
	for i, row := range rows {
		out[i] = Participant{UserID: row.UserID, Persona: row.Persona}
	}
	return out, nil
}

func (r *QueriesRepository) StartedSince(ctx context.Context, kind string, userID int64, since time.Time) (int, time.Time, error) {
	row, err := r.q.CountConversationsStartedSince(ctx, db.CountConversationsStartedSinceParams{
		StarterID: userID, Kind: kind, Since: pgtype.Timestamptz{Time: since, Valid: true},
	})
	if err != nil {
		return 0, time.Time{}, err
	}
	return int(row.Started), row.Oldest.Time, nil
}

func (r *QueriesRepository) Blocked(ctx context.Context, a, b int64) (bool, error) {
	return r.q.UsersBlocked(ctx, db.UsersBlockedParams{A: a, B: b})
}

// state says why a write that matched no row did not land: ErrNotFound when userID does
// not take part in the conversation, ErrState otherwise.
func (r *QueriesRepository) state(ctx context.Context, userID, conversationID int64) error {
	_, ok, err := r.ConversationFor(ctx, userID, conversationID)
	switch {
	case err != nil:
		return err
	case !ok:
		return ErrNotFound
	}
	return ErrState
}

func (r *QueriesRepository) Answer(ctx context.Context, userID, conversationID int64, status string) (ConversationRow, error) {
	row, err := r.q.AnswerConversation(ctx, db.AnswerConversationParams{Status: status, ID: conversationID, UserID: userID})
	if err == nil {
		return rowFrom(row), nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return ConversationRow{}, err
	}
	return ConversationRow{}, r.state(ctx, userID, conversationID)
}

func (r *QueriesRepository) InsertMessage(ctx context.Context, userID, conversationID int64, body string) (MessageRow, error) {
	row, err := r.q.InsertMessage(ctx, db.InsertMessageParams{SenderID: userID, Body: body, ConversationID: conversationID})
	if err == nil {
		return MessageRow{ID: row.ID, SenderID: row.SenderID, Body: row.Body, CreatedAt: row.CreatedAt.Time}, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return MessageRow{}, err
	}
	return MessageRow{}, r.state(ctx, userID, conversationID)
}

func (r *QueriesRepository) Messages(ctx context.Context, conversationID, after int64, limit int) ([]MessageRow, error) {
	rows, err := r.q.ListMessages(ctx, db.ListMessagesParams{ConversationID: conversationID, AfterID: after, PageLimit: int32(limit)})
	if err != nil {
		return nil, err
	}
	out := make([]MessageRow, len(rows))
	for i, row := range rows {
		out[i] = MessageRow{ID: row.ID, SenderID: row.SenderID, Body: row.Body, CreatedAt: row.CreatedAt.Time}
	}
	return out, nil
}

func (r *QueriesRepository) MarkRead(ctx context.Context, userID, conversationID, upTo int64) error {
	_, err := r.q.MarkConversationRead(ctx, db.MarkConversationReadParams{UserID: userID, UpTo: upTo, ConversationID: conversationID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func (r *QueriesRepository) Conversations(ctx context.Context, userID int64) ([]ListRow, error) {
	rows, err := r.q.ListConversations(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]ListRow, len(rows))
	for i, row := range rows {
		c := ConversationRow{
			ID: row.ID, Kind: row.Kind, StarterID: row.StarterID, Status: row.Status, CreatedAt: row.CreatedAt.Time,
		}
		if row.AcceptedAt.Valid {
			c.AcceptedAt = &row.AcceptedAt.Time
		}
		out[i] = ListRow{
			ConversationRow: c, Other: Participant{UserID: row.OtherID, Persona: row.OtherPersona},
			Unread: int(row.Unread), LastMessageAt: row.LastMessageAt.Time,
		}
	}
	return out, nil
}

func (r *QueriesRepository) Block(ctx context.Context, blocker, conversationID int64) error {
	_, err := r.q.BlockFromConversation(ctx, db.BlockFromConversationParams{ConversationID: conversationID, BlockerID: blocker})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func (r *QueriesRepository) Report(ctx context.Context, reporter, conversationID int64, reason, note string) error {
	_, err := r.q.InsertConversationReport(ctx, db.InsertConversationReportParams{
		ConversationID: conversationID, ReporterID: reporter, Reason: reason,
		Note: pgtype.Text{String: note, Valid: note != ""},
	})
	if err == nil {
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	// Nothing inserted: either the reporter does not take part or they already reported it.
	if err := r.state(ctx, reporter, conversationID); !errors.Is(err, ErrState) {
		return err
	}
	return ErrAlreadyReported
}

func (r *QueriesRepository) Reports(ctx context.Context, limit, offset int, includeResolved bool) ([]ReportRow, error) {
	rows, err := r.q.ListConversationReports(ctx, db.ListConversationReportsParams{
		IncludeResolved: includeResolved, PageLimit: int32(limit), PageOffset: int32(offset),
	})
	if err != nil {
		return nil, err
	}
	out := make([]ReportRow, len(rows))
	for i, row := range rows {
		out[i] = ReportRow{
			ID: row.ID, ConversationID: row.ConversationID, Kind: row.Kind, Reason: row.Reason,
			Note: row.Note.String, CreatedAt: row.CreatedAt.Time,
			Reporter: Participant{UserID: row.ReporterID, Persona: row.ReporterPersona},
			Reported: Participant{UserID: row.ReportedID, Persona: row.ReportedPersona},
		}
		if row.ResolvedAt.Valid {
			out[i].ResolvedAt = &row.ResolvedAt.Time
		}
	}
	return out, nil
}

func (r *QueriesRepository) ReportMessages(ctx context.Context, conversationIDs []int64) ([]ReportMessageRow, error) {
	rows, err := r.q.ListMessagesForConversations(ctx, conversationIDs)
	if err != nil {
		return nil, err
	}
	out := make([]ReportMessageRow, len(rows))
	for i, row := range rows {
		out[i] = ReportMessageRow{
			ConversationID: row.ConversationID, Sender: Participant{UserID: row.SenderID, Persona: row.SenderPersona},
			Body: row.Body, CreatedAt: row.CreatedAt.Time,
		}
	}
	return out, nil
}

func (r *QueriesRepository) ResolveReport(ctx context.Context, moderatorID, reportID int64) error {
	_, err := r.q.ResolveConversationReport(ctx, db.ResolveConversationReportParams{ID: reportID, ModeratorID: moderatorID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
