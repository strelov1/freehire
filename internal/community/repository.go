package community

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/pgconv"
	"github.com/strelov1/freehire/internal/pgerr"
)

// Compile-time proof that QueriesRepository satisfies both ports.
var (
	_ Repository     = (*QueriesRepository)(nil)
	_ SubjectChecker = (*QueriesRepository)(nil)
)

// QueriesRepository is the production Repository (and SubjectChecker) backed by
// sqlc-generated *db.Queries. Every write is a single statement; the unique / no-row
// guards live in SQL and are mapped to sentinel errors here.
type QueriesRepository struct {
	q *db.Queries
}

// NewQueriesRepository constructs a QueriesRepository.
func NewQueriesRepository(q *db.Queries) *QueriesRepository { return &QueriesRepository{q: q} }

func (r *QueriesRepository) GetPersona(ctx context.Context, userID int64) (Persona, error) {
	row, err := r.q.GetCommunityPersona(ctx, userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Persona{}, ErrPersonaNotFound
		}
		return Persona{}, err
	}
	return Persona{UserID: row.UserID, Handle: row.Handle, CreatedAt: row.CreatedAt.Time}, nil
}

func (r *QueriesRepository) InsertPersona(ctx context.Context, userID int64, handle string) (Persona, error) {
	row, err := r.q.InsertCommunityPersona(ctx, db.InsertCommunityPersonaParams{UserID: userID, Handle: handle})
	if err != nil {
		// A handle collision with another user (not caught by ON CONFLICT (user_id)).
		if pgerr.IsUniqueViolation(err) {
			return Persona{}, ErrHandleTaken
		}
		// ON CONFLICT (user_id) DO NOTHING returned no row: this user already has a
		// persona (concurrent mint) — return the existing winner.
		if errors.Is(err, pgx.ErrNoRows) {
			return r.GetPersona(ctx, userID)
		}
		return Persona{}, err
	}
	return Persona{UserID: row.UserID, Handle: row.Handle, CreatedAt: row.CreatedAt.Time}, nil
}

func (r *QueriesRepository) InsertThread(ctx context.Context, subjectType, subjectRef, title, body string, authorUserID int64) (Thread, error) {
	row, err := r.q.InsertThread(ctx, db.InsertThreadParams{
		SubjectType: subjectType, SubjectRef: subjectRef, Title: title, Body: body, AuthorUserID: authorUserID,
	})
	if err != nil {
		return Thread{}, err
	}
	return Thread{
		ID: row.ID, SubjectType: row.SubjectType, SubjectRef: row.SubjectRef,
		AnchorPath: pgconv.TextString(row.AnchorPath), Title: row.Title, Body: row.Body,
		ReplyCount: row.ReplyCount, Status: row.Status, CreatedAt: row.CreatedAt.Time,
	}, nil
}

func (r *QueriesRepository) GetThread(ctx context.Context, id int64) (Thread, error) {
	row, err := r.q.GetCommunityThread(ctx, id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return Thread{}, ErrThreadNotFound
		}
		return Thread{}, err
	}
	return Thread{
		ID: row.ID, SubjectType: row.SubjectType, SubjectRef: row.SubjectRef,
		AnchorPath: pgconv.TextString(row.AnchorPath), Title: row.Title, Body: row.Body,
		AuthorHandle: row.AuthorHandle, ReplyCount: row.ReplyCount, Status: row.Status,
		CreatedAt: row.CreatedAt.Time,
	}, nil
}

func (r *QueriesRepository) ListOpenThreads(ctx context.Context, subjectType, subjectRef string, cur Cursor, limit int32) ([]Thread, error) {
	if cur.IsZero() {
		rows, err := r.q.ListOpenThreadsFirst(ctx, db.ListOpenThreadsFirstParams{
			SubjectType: subjectType, SubjectRef: subjectRef, Limit: limit,
		})
		if err != nil {
			return nil, err
		}
		out := make([]Thread, len(rows))
		for i, row := range rows {
			out[i] = Thread{
				ID: row.ID, SubjectType: row.SubjectType, SubjectRef: row.SubjectRef,
				AnchorPath: pgconv.TextString(row.AnchorPath), Title: row.Title, Body: row.Body,
				AuthorHandle: row.AuthorHandle, ReplyCount: row.ReplyCount, Status: row.Status,
				CreatedAt: row.CreatedAt.Time,
			}
		}
		return out, nil
	}
	rows, err := r.q.ListOpenThreadsAfter(ctx, db.ListOpenThreadsAfterParams{
		SubjectType: subjectType, SubjectRef: subjectRef,
		CursorCreatedAt: pgconv.Timestamptz(&cur.CreatedAt), CursorID: cur.ID, PageLimit: limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Thread, len(rows))
	for i, row := range rows {
		out[i] = Thread{
			ID: row.ID, SubjectType: row.SubjectType, SubjectRef: row.SubjectRef,
			AnchorPath: pgconv.TextString(row.AnchorPath), Title: row.Title, Body: row.Body,
			AuthorHandle: row.AuthorHandle, ReplyCount: row.ReplyCount, Status: row.Status,
			CreatedAt: row.CreatedAt.Time,
		}
	}
	return out, nil
}

func (r *QueriesRepository) CountOpenThreads(ctx context.Context, subjectType, subjectRef string) (int64, error) {
	return r.q.CountOpenThreadsBySubject(ctx, db.CountOpenThreadsBySubjectParams{
		SubjectType: subjectType, SubjectRef: subjectRef,
	})
}

func (r *QueriesRepository) CloseThread(ctx context.Context, id int64) error {
	return r.q.CloseCommunityThread(ctx, id)
}

func (r *QueriesRepository) InsertReply(ctx context.Context, threadID, parentReplyID, authorUserID int64, body string) (Reply, error) {
	parent := pgtype.Int8{}
	if parentReplyID != 0 {
		parent = pgtype.Int8{Int64: parentReplyID, Valid: true}
	}
	row, err := r.q.InsertThreadReply(ctx, db.InsertThreadReplyParams{
		ThreadID: threadID, ParentReplyID: parent,
		AuthorUserID: pgtype.Int8{Int64: authorUserID, Valid: true}, Body: body,
	})
	if err != nil {
		return Reply{}, err
	}
	return Reply{
		ID: row.ID, ThreadID: row.ThreadID, ParentID: row.ParentReplyID.Int64,
		IsAI: row.IsAi, Body: row.Body, CreatedAt: row.CreatedAt.Time,
	}, nil
}

func (r *QueriesRepository) IncrementReplyCount(ctx context.Context, threadID int64) error {
	return r.q.IncrementThreadReplyCount(ctx, threadID)
}

func (r *QueriesRepository) ListReplies(ctx context.Context, threadID int64, cur Cursor, limit int32) ([]Reply, error) {
	if cur.IsZero() {
		rows, err := r.q.ListThreadRepliesFirst(ctx, db.ListThreadRepliesFirstParams{ThreadID: threadID, Limit: limit})
		if err != nil {
			return nil, err
		}
		out := make([]Reply, len(rows))
		for i, row := range rows {
			out[i] = Reply{
				ID: row.ID, ThreadID: row.ThreadID, ParentID: row.ParentReplyID.Int64,
				AuthorHandle: pgconv.TextString(row.AuthorHandle),
				IsAI:         row.IsAi, Body: row.Body, CreatedAt: row.CreatedAt.Time,
			}
		}
		return out, nil
	}
	rows, err := r.q.ListThreadRepliesAfter(ctx, db.ListThreadRepliesAfterParams{
		ThreadID: threadID, CursorCreatedAt: pgconv.Timestamptz(&cur.CreatedAt), CursorID: cur.ID, PageLimit: limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Reply, len(rows))
	for i, row := range rows {
		out[i] = Reply{
			ID: row.ID, ThreadID: row.ThreadID, ParentID: row.ParentReplyID.Int64,
			AuthorHandle: pgconv.TextString(row.AuthorHandle),
			IsAI:         row.IsAi, Body: row.Body, CreatedAt: row.CreatedAt.Time,
		}
	}
	return out, nil
}

func (r *QueriesRepository) CountRecentThreads(ctx context.Context, userID int64, since time.Time) (int64, error) {
	return r.q.CountRecentThreadsByUser(ctx, db.CountRecentThreadsByUserParams{
		AuthorUserID: userID, CreatedAt: pgconv.Timestamptz(&since),
	})
}

func (r *QueriesRepository) CountRecentReplies(ctx context.Context, userID int64, since time.Time) (int64, error) {
	return r.q.CountRecentRepliesByUser(ctx, db.CountRecentRepliesByUserParams{
		AuthorUserID: pgtype.Int8{Int64: userID, Valid: true}, CreatedAt: pgconv.Timestamptz(&since),
	})
}

// SubjectExists reports whether slug names an existing company or job.
func (r *QueriesRepository) SubjectExists(ctx context.Context, subjectType, slug string) (bool, error) {
	switch subjectType {
	case SubjectCompany:
		return r.q.CompanyExists(ctx, slug)
	case SubjectJob:
		_, err := r.q.GetJobIDBySlug(ctx, slug)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	default:
		return false, ErrUnsupportedSubject
	}
}
