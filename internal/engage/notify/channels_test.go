package notify

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

func TestPush_CarriesADeepLinkOnlyWhenTheMessageNamesOneItem(t *testing.T) {
	// The rule every engine's render follows: a deep link needs one destination.
	render := func(m []string) (title, body, slug string) {
		if len(m) == 1 {
			return "1 job", "body", m[0]
		}
		return "several jobs", "body", ""
	}
	transport := &fakePushTransport{}
	n := NewPush(&fakePushTokenLister{tokens: map[int64][]db.UserPushToken{42: {{Token: "tok-1"}}}}, transport, render)

	if err := n.Send(context.Background(), ChannelPush, "42", []string{"go-dev-acme"}); err != nil {
		t.Fatalf("Send of one item: %v", err)
	}
	if err := n.Send(context.Background(), ChannelPush, "42", []string{"a", "b"}); err != nil {
		t.Fatalf("Send of a batch: %v", err)
	}
	if got := transport.calls[0].data["slug"]; got != "go-dev-acme" {
		t.Errorf("one item: data[slug] = %q, want the item's slug", got)
	}
	if got := transport.calls[1].data; len(got) != 0 {
		t.Errorf("a batch: data = %v, want none because no one item can be named", got)
	}
}

func TestRecipient_NeedsTheChannelToBeReachable(t *testing.T) {
	chat := pgtype.Int8{Int64: 77, Valid: true}
	for _, tc := range []struct {
		name    string
		channel string
		info    RecipientInfo
		dest    string
		ok      bool
	}{
		{"telegram linked", ChannelTelegram, RecipientInfo{TelegramChatID: chat}, "77", true},
		{"telegram unlinked", ChannelTelegram, RecipientInfo{}, "", false},
		{"email set", ChannelEmail, RecipientInfo{AccountEmail: "u@x.com"}, "u@x.com", true},
		{"email missing", ChannelEmail, RecipientInfo{}, "", false},
		{"push with a device", ChannelPush, RecipientInfo{UserID: 42, HasPushDevice: true}, "42", true},
		{"push with no device", ChannelPush, RecipientInfo{UserID: 42}, "", false},
		{"unknown channel", "webhook", RecipientInfo{UserID: 42, HasPushDevice: true}, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dest, ok := Recipient(tc.channel, tc.info)
			if dest != tc.dest || ok != tc.ok {
				t.Errorf("Recipient(%s) = (%q, %v), want (%q, %v)", tc.channel, dest, ok, tc.dest, tc.ok)
			}
		})
	}
}

func TestRelease_RunsOnALiveContextAfterTheRunIsCancelled(t *testing.T) {
	// A cancelled run (SIGTERM) must still let go of the claim it holds, so the
	// release sees a context that is not the run's.
	run, cancel := context.WithCancel(context.Background())
	cancel()
	var released []int64
	Release(run, "nudge", 7, func(ctx context.Context, id int64) error {
		if err := ctx.Err(); err != nil {
			t.Errorf("release context is done: %v, want it live", err)
		}
		released = append(released, id)
		return nil
	})
	if len(released) != 1 || released[0] != 7 {
		t.Errorf("released %v, want [7]", released)
	}

	// A failing release is logged and swallowed: the lease expires on its own.
	Release(context.Background(), "nudge", 8, func(context.Context, int64) error { return errors.New("boom") })
}

func TestJobsSnapshotOf_ProjectsEveryItemInOrder(t *testing.T) {
	type item struct{ title, slug string }
	got := JobsSnapshotOf([]item{{"Go Dev", "go-dev"}, {"Rust Dev", "rust-dev"}},
		func(i item) SnapshotJob { return SnapshotJob{Title: i.title, Slug: i.slug} })

	var jobs []SnapshotJob
	if err := json.Unmarshal(got, &jobs); err != nil {
		t.Fatalf("snapshot is not a JSON job list: %v (%s)", err, got)
	}
	want := []SnapshotJob{{Title: "Go Dev", Slug: "go-dev"}, {Title: "Rust Dev", Slug: "rust-dev"}}
	if len(jobs) != 2 || jobs[0] != want[0] || jobs[1] != want[1] {
		t.Errorf("jobs = %+v, want %+v", jobs, want)
	}
}
