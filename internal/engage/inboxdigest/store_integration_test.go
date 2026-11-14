//go:build integration

package inboxdigest

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

// The runner over the real store: a paused send leaves the day open, the next run
// delivers once, and a later run the same local day sends nothing.
func TestDBStoreDeliversOncePerLocalDay(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	now := time.Date(2030, 1, 2, 14, 0, 0, 0, time.UTC) // 09:00 in New York
	var uid int64
	if err := pool.QueryRow(ctx, `INSERT INTO users(email,email_verified,last_active_at,timezone,pro_until_granted)
		VALUES('digest-store@example.test',true,now(),'America/New_York',$1) RETURNING id`, now.Add(24*time.Hour)).Scan(&uid); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO notification_settings(user_id,enabled,channels,digest_frequency,digest_time,inbox_digest_enabled,inbox_digest_enabled_at)
		VALUES($1,true,'{email}','daily','09:00',true,$2)`, uid, now.Add(-2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO emails(user_id,source,external_id,thread_id,subject,body_text,status_signal,classified_at,direction,received_at)
		VALUES($1,'gmail','m1','t1','Interview next week','Original excerpt','screening',now(),'incoming',$2)`, uid, now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	mailer := NewMailer("freehire@example.test", "https://freehire.test", strings.Repeat("x", 32))
	run := func(sendErr error) (Stats, *capturingSender) {
		t.Helper()
		sender := &capturingSender{err: sendErr}
		stats, err := New(NewDBStore(db.New(pool)), mailer, sender).Run(ctx, now)
		if err != nil {
			t.Fatal(err)
		}
		return stats, sender
	}

	if stats, _ := run(emailnotify.ErrSendingPaused); stats.Skipped != 1 {
		t.Fatalf("paused run: %+v", stats)
	}
	stats, sender := run(nil)
	if stats.Sent != 1 || len(sender.messages) != 1 || !strings.Contains(sender.messages[0].Text, "Interview next week") {
		t.Fatalf("delivery run: %+v %+v", stats, sender.messages)
	}
	if stats, sender := run(nil); stats.Sent != 0 || len(sender.messages) != 0 {
		t.Fatalf("same local day sent again: %+v", stats)
	}
}
