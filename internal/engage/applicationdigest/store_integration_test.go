//go:build integration

package applicationdigest

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

func TestWeeklyDeliveryWithRealStore(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	now := time.Date(2030, 1, 7, 14, 0, 0, 0, time.UTC)
	var uid int64
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,email_verified,last_active_at,timezone,pro_until_granted)
 VALUES('weekly-store@example.test',true,now(),'America/New_York',$1) RETURNING id`, now.AddDate(0, 0, 14)).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO notification_settings(user_id,enabled,channels,digest_time,application_digest_enabled,application_digest_enabled_at)
 VALUES($1,true,'{email}','09:00',true,$2)`, uid, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO applications(user_id,company_slug,role_title,stage,applied_at,followed_up_at)
 VALUES($1,'acme','Engineer','applied',$2,$3)`, uid, now.AddDate(0, 0, -30), now.AddDate(0, 0, -2)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO application_events(user_id,company_slug,kind,source,occurred_at)
 VALUES($1,'acme','applied','user',$2)`, uid, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	mailer := NewMailer("freehire@example.test", "https://freehire.test", strings.Repeat("x", 32))
	run := func(at time.Time, sendErr error) (Stats, *capturingSender) {
		t.Helper()
		sender := &capturingSender{err: sendErr}
		stats, err := New(NewDBStore(db.New(pool)), mailer, sender).Run(ctx, at)
		if err != nil {
			t.Fatal(err)
		}
		return stats, sender
	}
	if stats, _ := run(now, emailnotify.ErrSendingPaused); stats.Skipped != 1 {
		t.Fatalf("pause: %+v", stats)
	}
	stats, sender := run(now, nil)
	if stats.Sent != 1 || len(sender.messages) != 1 {
		t.Fatalf("delivery: %+v", stats)
	}
	body := sender.messages[0].Text
	for _, want := range []string{"1 applications", "Engineer", "No reply recorded for 30 days", "Followed up:", "/my/tracking/a"} {
		if !strings.Contains(body, want) {
			t.Fatalf("missing %q in %s", want, body)
		}
	}
	if stats, _ := run(now, nil); stats.Sent != 0 {
		t.Fatalf("duplicate week: %+v", stats)
	}
	if stats, _ := run(now.AddDate(0, 0, 1), nil); stats.Sent != 0 {
		t.Fatalf("Tuesday delivery: %+v", stats)
	}
	if stats, _ := run(now.AddDate(0, 0, 7), nil); stats.Sent != 1 {
		t.Fatalf("next week: %+v", stats)
	}
}
