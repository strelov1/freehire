//go:build integration

// Integration tests for the unread-message notifier.
// Run with: go test -tags=integration ./internal/engage/messaging/
package messaging_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
	"github.com/strelov1/freehire/internal/engage/messaging"
	"github.com/strelov1/freehire/internal/platform/db"
)

type fakeEmail struct {
	sent []emailnotify.Message
	err  error
}

func (f *fakeEmail) Send(_ context.Context, m emailnotify.Message) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, m)
	return nil
}

type fakeTelegram struct {
	sent []string
	err  error
}

func (f *fakeTelegram) SendMessage(_ context.Context, _ int64, html string) error {
	if f.err != nil {
		return f.err
	}
	f.sent = append(f.sent, html)
	return nil
}

func notifier(pool *pgxpool.Pool, em *fakeEmail, tg messaging.TelegramSender) *messaging.Notifier {
	return messaging.NewNotifier(db.New(pool), messaging.NotifierConfig{
		Email: em, From: "noreply@example.test", Telegram: tg,
		Links: emailprefs.NewLinks("test-secret-test-secret-test-secret-0123", "https://freehire.test"), Origin: "https://freehire.test",
	}, stubKind{})
}

// age moves every message of the conversation `by` before the real clock.
func age(t *testing.T, pool *pgxpool.Pool, convID int64, by time.Duration) {
	t.Helper()
	if _, err := pool.Exec(context.Background(),
		`UPDATE messages SET created_at = now() - make_interval(secs => $2) WHERE conversation_id = $1`,
		convID, by.Seconds()); err != nil {
		t.Fatal(err)
	}
}

func lastMessageID(t *testing.T, pool *pgxpool.Pool, convID int64) int64 {
	t.Helper()
	var id int64
	if err := pool.QueryRow(context.Background(), `SELECT max(id) FROM messages WHERE conversation_id = $1`, convID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func linkTelegram(t *testing.T, pool *pgxpool.Pool, userID int64) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), `INSERT INTO telegram_links (user_id, chat_id) VALUES ($1, 42)`, userID); err != nil {
		t.Fatal(err)
	}
}

func TestNotifyOncePerConversationAcrossRuns(t *testing.T) {
	pool, svc, a, b, id := openChat(t)
	ctx := context.Background()
	for _, body := range []string{"one", "two", "three"} {
		if _, err := svc.Send(ctx, a, id, body); err != nil {
			t.Fatal(err)
		}
	}
	age(t, pool, id, 10*time.Minute)
	em := &fakeEmail{}
	n := notifier(pool, em, nil)

	res, err := n.NotifyUnread(ctx, time.Now())
	if err != nil || res.Notified != 1 || res.Failed != 0 {
		t.Fatalf("first run = %+v, %v", res, err)
	}
	res, err = n.NotifyUnread(ctx, time.Now())
	if err != nil || res.Notified != 0 {
		t.Fatalf("second run = %+v, %v", res, err)
	}
	if len(em.sent) != 1 || em.sent[0].To != "b@example.test" {
		t.Fatalf("sent = %+v", em.sent)
	}
	if em.sent[0].Subject != "You have a new message" {
		t.Fatalf("subject = %q", em.sent[0].Subject)
	}
	if got := countRows(t, pool, `SELECT count(*) FROM user_notifications WHERE user_id = $1 AND kind = 'stub_message'`, b); got != 1 {
		t.Fatalf("in-app notifications = %d", got)
	}
}

func TestNotifyWaitsForFiveMinutes(t *testing.T) {
	pool, svc, a, _, id := openChat(t)
	ctx := context.Background()
	if _, err := svc.Send(ctx, a, id, "hi"); err != nil {
		t.Fatal(err)
	}
	age(t, pool, id, 2*time.Minute)
	n := notifier(pool, &fakeEmail{}, nil)

	if res, err := n.NotifyUnread(ctx, time.Now()); err != nil || res.Notified != 0 {
		t.Fatalf("young message notified: %+v, %v", res, err)
	}
	if res, err := n.NotifyUnread(ctx, time.Now().Add(4*time.Minute)); err != nil || res.Notified != 1 {
		t.Fatalf("aged message = %+v, %v", res, err)
	}
}

func TestNotifyAgainAfterReadAndNewMessage(t *testing.T) {
	pool, svc, a, b, id := openChat(t)
	ctx := context.Background()
	age(t, pool, id, 10*time.Minute)
	em := &fakeEmail{}
	n := notifier(pool, em, nil)
	// The first claim is stamped a minute back so the database's clock, which may trail
	// this process's, still writes the later message after it.
	if res, err := n.NotifyUnread(ctx, time.Now().Add(-time.Minute)); err != nil || res.Notified != 1 {
		t.Fatalf("first = %+v, %v", res, err)
	}

	if err := svc.MarkRead(ctx, b, id, lastMessageID(t, pool, id)); err != nil {
		t.Fatal(err)
	}
	if res, err := n.NotifyUnread(ctx, time.Now().Add(time.Hour)); err != nil || res.Notified != 0 {
		t.Fatalf("read conversation notified: %+v, %v", res, err)
	}
	if _, err := svc.Send(ctx, a, id, "are you there?"); err != nil {
		t.Fatal(err)
	}
	if res, err := n.NotifyUnread(ctx, time.Now().Add(time.Minute)); err != nil || res.Notified != 0 {
		t.Fatalf("young new message notified: %+v, %v", res, err)
	}
	if res, err := n.NotifyUnread(ctx, time.Now().Add(10*time.Minute)); err != nil || res.Notified != 1 {
		t.Fatalf("re-notify = %+v, %v", res, err)
	}
	if len(em.sent) != 2 {
		t.Fatalf("sent %d mails, want 2", len(em.sent))
	}
}

func TestNotifySkipsDeclinedAndClosed(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name string
		stmt string
		arg  func(id, b int64) int64
	}{
		{"declined", `UPDATE conversations SET status = 'declined' WHERE id = $1`, func(id, _ int64) int64 { return id }},
		{"closed", `UPDATE conversations SET status = 'closed' WHERE id = $1`, func(id, _ int64) int64 { return id }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool, svc, a, b, id := openChat(t)
			if _, err := svc.Send(ctx, a, id, "hi"); err != nil {
				t.Fatal(err)
			}
			age(t, pool, id, time.Hour)
			if _, err := pool.Exec(ctx, tc.stmt, tc.arg(id, b)); err != nil {
				t.Fatal(err)
			}
			em := &fakeEmail{}
			if res, err := notifier(pool, em, nil).NotifyUnread(ctx, time.Now()); err != nil || res.Notified != 0 || len(em.sent) != 0 {
				t.Fatalf("notified: %+v, %v, mails %d", res, err, len(em.sent))
			}
		})
	}
}

func TestStarterNotNotifiedAboutOwnPendingRequest(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	a := seedUser(t, pool, "a@example.test")
	b := seedUser(t, pool, "b@example.test")
	c, err := start(ctx, svc, a, b, "hello there")
	if err != nil {
		t.Fatal(err)
	}
	age(t, pool, c.ID, time.Hour)
	em := &fakeEmail{}
	res, err := notifier(pool, em, nil).NotifyUnread(ctx, time.Now())
	if err != nil || res.Notified != 1 || len(em.sent) != 1 {
		t.Fatalf("run = %+v, %v, mails %d", res, err, len(em.sent))
	}
	if em.sent[0].To != "b@example.test" || em.sent[0].Subject != "You have a new request" {
		t.Fatalf("mail = %+v", em.sent[0])
	}
	if got := countRows(t, pool, `SELECT count(*) FROM user_notifications WHERE user_id = $1 AND kind = 'stub_request'`, b); got != 1 {
		t.Fatalf("invitation notifications = %d", got)
	}
}

func TestFailureReleasesClaimAndNextRunSends(t *testing.T) {
	pool, svc, a, _, id := openChat(t)
	ctx := context.Background()
	if _, err := svc.Send(ctx, a, id, "hi"); err != nil {
		t.Fatal(err)
	}
	age(t, pool, id, time.Hour)
	em := &fakeEmail{err: errors.New("ses down")}
	n := notifier(pool, em, nil)

	res, err := n.NotifyUnread(ctx, time.Now())
	if err != nil || res.Failed != 1 || res.Notified != 0 {
		t.Fatalf("failing run = %+v, %v", res, err)
	}
	em.err = nil
	res, err = n.NotifyUnread(ctx, time.Now())
	if err != nil || res.Notified != 1 || len(em.sent) != 1 {
		t.Fatalf("retry = %+v, %v, mails %d", res, err, len(em.sent))
	}
}

func TestPartialChannelSuccessCountsAsSent(t *testing.T) {
	pool, svc, a, b, id := openChat(t)
	ctx := context.Background()
	if _, err := svc.Send(ctx, a, id, "hi"); err != nil {
		t.Fatal(err)
	}
	age(t, pool, id, time.Hour)
	linkTelegram(t, pool, b)
	tg := &fakeTelegram{}
	n := notifier(pool, &fakeEmail{err: errors.New("ses down")}, tg)
	if res, err := n.NotifyUnread(ctx, time.Now()); err != nil || res.Notified != 1 || res.Failed != 0 {
		t.Fatalf("run = %+v, %v", res, err)
	}
	if len(tg.sent) != 1 {
		t.Fatalf("telegram sent %d", len(tg.sent))
	}
	if res, err := n.NotifyUnread(ctx, time.Now()); err != nil || res.Notified != 0 {
		t.Fatalf("claim was released after partial success: %+v, %v", res, err)
	}
}

func TestNoticeCarriesNoMessageTextOrAccount(t *testing.T) {
	pool, svc, a, b, id := openChat(t)
	ctx := context.Background()
	if _, err := svc.Send(ctx, a, id, "my secret-body-marker"); err != nil {
		t.Fatal(err)
	}
	age(t, pool, id, time.Hour)
	linkTelegram(t, pool, b)
	em, tg := &fakeEmail{}, &fakeTelegram{}
	if _, err := notifier(pool, em, tg).NotifyUnread(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(em.sent) != 1 || len(tg.sent) != 1 {
		t.Fatalf("channels fired: mail %d, telegram %d", len(em.sent), len(tg.sent))
	}
	var title, body string
	if err := pool.QueryRow(ctx, `SELECT title, body FROM user_notifications WHERE user_id = $1`, b).Scan(&title, &body); err != nil {
		t.Fatal(err)
	}
	texts := []string{em.sent[0].Subject, em.sent[0].HTML, em.sent[0].Text, tg.sent[0], title, body}
	for _, s := range texts {
		if strings.Contains(s, "secret-body-marker") || strings.Contains(s, "a@example.test") {
			t.Fatalf("notice leaks content: %q", s)
		}
	}
	if !strings.Contains(em.sent[0].Text, "https://freehire.test/my/inbox?c=") {
		t.Fatalf("no chat link in %q", em.sent[0].Text)
	}
}
