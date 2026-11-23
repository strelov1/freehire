//go:build integration

// Integration tests for the employer's conversation with an applicant over the real rows.
// Run with: go test -tags=integration ./internal/ingest/directapply/
package directapply_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ingest/directapply"
	"github.com/strelov1/freehire/internal/platform/db"
)

type fakeMessageMailer struct {
	err     error
	sends   int
	replyTo string
}

func (f *fakeMessageMailer) SendCandidateMessage(_ context.Context, _, replyTo, _, _, _ string) error {
	f.sends++
	f.replyTo = replyTo
	return f.err
}

func TestApplicationMessages(t *testing.T) {
	f := newInboxFixture(t)
	ctx := context.Background()
	app := f.apply(t, f.vacancy, f.candidate, "cv", true)
	mailer := &fakeMessageMailer{}
	svc := directapply.NewApplicationMessages(db.New(f.pool), mailer, "mail.example.test")
	key := uuid.NewString()

	t.Run("validates input and scope", func(t *testing.T) {
		if _, err := svc.Send(ctx, f.employer, app, "bad", "hi"); !errors.Is(err, directapply.ErrInvalid) {
			t.Errorf("bad key err = %v", err)
		}
		if _, err := svc.Send(ctx, f.employer, app, key, "  "); !errors.Is(err, directapply.ErrInvalid) {
			t.Errorf("empty body err = %v", err)
		}
		if _, err := svc.Send(ctx, f.other, app, key, "hi"); !errors.Is(err, directapply.ErrNotFound) {
			t.Errorf("other employer err = %v", err)
		}
		if _, err := svc.History(ctx, f.employer, app, -1); !errors.Is(err, directapply.ErrInvalid) {
			t.Errorf("negative cursor err = %v", err)
		}
		if _, err := svc.History(ctx, f.other, app, 0); !errors.Is(err, directapply.ErrNotFound) {
			t.Errorf("other employer history err = %v", err)
		}
		if _, err := svc.MarkRead(ctx, f.employer, app, 0); !errors.Is(err, directapply.ErrInvalid) {
			t.Errorf("zero through err = %v", err)
		}
	})

	t.Run("send is idempotent per key and recorded", func(t *testing.T) {
		msg, err := svc.Send(ctx, f.employer, app, key, "Hello")
		if err != nil || msg.Status != "sent" || msg.RequestKey != key {
			t.Fatalf("Send = %+v, %v", msg, err)
		}
		again, err := svc.Send(ctx, f.employer, app, key, "Hello")
		if err != nil || again.ID != msg.ID || mailer.sends != 1 {
			t.Fatalf("retry = %+v, %v, sends %d", again, err, mailer.sends)
		}
		if _, err := svc.Send(ctx, f.employer, app, key, "Different"); !errors.Is(err, directapply.ErrMessageConflict) {
			t.Errorf("reused key err = %v", err)
		}
		h, err := svc.History(ctx, f.employer, app, 0)
		if err != nil || len(h.Messages) != 1 || !h.CanSend || h.HasMore {
			t.Fatalf("History = %+v, %v", h, err)
		}
	})

	t.Run("a failed delivery is recorded and can be retried", func(t *testing.T) {
		mailer.err = errors.New("smtp down")
		k := uuid.NewString()
		msg, err := svc.Send(ctx, f.employer, app, k, "Second")
		if err != nil || msg.Status != "failed" {
			t.Fatalf("Send = %+v, %v", msg, err)
		}
		mailer.err = nil
		retry, err := svc.Send(ctx, f.employer, app, k, "Second")
		if err != nil || retry.ID != msg.ID || retry.Status != "sent" {
			t.Fatalf("retry = %+v, %v", retry, err)
		}
	})

	t.Run("unread replies clear on read", func(t *testing.T) {
		var token uuid.UUID
		if err := f.pool.QueryRow(ctx, `SELECT reply_token FROM direct_application_messages WHERE application_id = $1 LIMIT 1`, app).Scan(&token); err != nil {
			t.Fatal(err)
		}
		if err := db.New(f.pool).InsertEmployerApplicationReply(ctx, db.InsertEmployerApplicationReplyParams{
			ReplyToken: uuidToPG(token), ExternalID: "ext-1", Sender: "c@example.test", Body: "Thanks",
		}); err != nil {
			t.Fatal(err)
		}
		h, err := svc.History(ctx, f.employer, app, 0)
		if err != nil || len(h.Messages) < 3 {
			t.Fatalf("History = %+v, %v", h, err)
		}
		unread, err := svc.MarkRead(ctx, f.employer, app, h.Messages[0].ID)
		if err != nil || unread {
			t.Errorf("MarkRead = %v, %v", unread, err)
		}
	})

	t.Run("unverified candidate or missing mailer is unavailable", func(t *testing.T) {
		off := directapply.NewApplicationMessages(db.New(f.pool), nil, "")
		if _, err := off.Send(ctx, f.employer, app, uuid.NewString(), "hi"); !errors.Is(err, directapply.ErrMessageUnavailable) {
			t.Errorf("err = %v", err)
		}
	})
}

func uuidToPG(u uuid.UUID) pgtype.UUID { return pgtype.UUID{Bytes: u, Valid: true} }
