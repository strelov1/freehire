package buddy

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

func (r *QueriesRepository) Block(ctx context.Context, blocker, conversationID int64) error {
	_, err := r.q.BlockBuddyConversation(ctx, db.BlockBuddyConversationParams{ConversationID: conversationID, BlockerID: blocker})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func (r *QueriesRepository) Report(ctx context.Context, reporter, conversationID int64, reason, note string) error {
	_, err := r.q.InsertBuddyReport(ctx, db.InsertBuddyReportParams{
		ConversationID: conversationID, ReporterID: reporter, Reason: reason,
		Note: pgtype.Text{String: note, Valid: note != ""},
	})
	if err == nil {
		return nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	// Nothing inserted: either the conversation is not the reporter's or they already reported it.
	cur, ok, err := r.Conversation(ctx, conversationID)
	if err != nil {
		return err
	}
	if !ok || (cur.UserLow != reporter && cur.UserHigh != reporter) {
		return ErrNotFound
	}
	return ErrAlreadyReported
}

func (r *QueriesRepository) Reports(ctx context.Context, limit, offset int, includeResolved bool) ([]ReportRow, error) {
	rows, err := r.q.ListBuddyReports(ctx, db.ListBuddyReportsParams{
		IncludeResolved: includeResolved, PageLimit: int32(limit), PageOffset: int32(offset),
	})
	if err != nil {
		return nil, err
	}
	out := make([]ReportRow, len(rows))
	for i, row := range rows {
		out[i] = ReportRow{
			ID: row.ID, ConversationID: row.ConversationID, Reason: row.Reason, Note: row.Note.String,
			CreatedAt: row.CreatedAt.Time, ReporterHandle: row.ReporterHandle, ReportedHandle: row.ReportedHandle,
		}
		if row.ResolvedAt.Valid {
			out[i].ResolvedAt = &row.ResolvedAt.Time
		}
	}
	return out, nil
}

func (r *QueriesRepository) ReportMessages(ctx context.Context, conversationIDs []int64) ([]ReportMessageRow, error) {
	rows, err := r.q.ListBuddyMessagesForConversations(ctx, conversationIDs)
	if err != nil {
		return nil, err
	}
	out := make([]ReportMessageRow, len(rows))
	for i, row := range rows {
		out[i] = ReportMessageRow{
			ConversationID: row.ConversationID,
			ReportMessage:  ReportMessage{SenderHandle: row.SenderHandle, Body: row.Body, CreatedAt: row.CreatedAt.Time},
		}
	}
	return out, nil
}

func (r *QueriesRepository) ResolveReport(ctx context.Context, moderatorID, reportID int64) error {
	_, err := r.q.ResolveBuddyReport(ctx, db.ResolveBuddyReportParams{ID: reportID, ModeratorID: moderatorID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}
