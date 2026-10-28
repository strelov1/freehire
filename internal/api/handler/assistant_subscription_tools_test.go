package handler

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

func TestSubscriptionSearch_DropsUnknownValuesAndRefusesUnknownNames(t *testing.T) {
	vals, ignored, err := subscriptionSearch(" rust ", assistantFilterArgs{
		"skills":    {"go", "golangg", "go"},
		"work_mode": {"remote", "sometimes"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := encodeSavedQuery(vals); got != "q=rust&skills=go&work_mode=remote" {
		t.Errorf("query = %q", got)
	}
	if strings.Join(ignored, " ") != "skills=golangg work_mode=sometimes" {
		t.Errorf("ignored = %v", ignored)
	}
	if _, _, err := subscriptionSearch("", assistantFilterArgs{"specializations": {"x"}}); err == nil {
		t.Error("an unknown filter name must be refused, as search_jobs refuses it")
	}
}

func TestSameSelection_IgnoresOrderListFormAndSort(t *testing.T) {
	if !sameSelection("skills=go,rust&company_slug=stripe", "company_slug=stripe&skills=rust&skills=go&sort=posted_at:desc") {
		t.Error("the same selection written two ways must compare equal")
	}
	if sameSelection("skills=go", "skills=go,rust") {
		t.Error("different value sets must not compare equal")
	}
}

func TestFilterSummaryAndAlertName(t *testing.T) {
	if got := filterSummary("skills=go,rust&q=backend&company_slug=stripe"); got != `"backend" · company_slug: stripe · skills: go, rust` {
		t.Errorf("summary = %q", got)
	}
	long := alertName("q=" + strings.Repeat("x", 200))
	if n := len([]rune(long)); n != alertNameMaxRunes {
		t.Errorf("long name has %d runes, want %d", n, alertNameMaxRunes)
	}
}

func TestDeliverableChannels(t *testing.T) {
	if got := deliverableChannels(db.GetAlertChannelStateRow{}); len(got) != 0 {
		t.Errorf("nothing connected = %v", got)
	}
	got := deliverableChannels(db.GetAlertChannelStateRow{EmailAlertsEnabled: true, TelegramLinked: true, WebhookEnabled: true})
	if strings.Join(got, ",") != "email,telegram,webhook" {
		t.Errorf("channels = %v", got)
	}
}

func TestFoldUpdates_OnePostingPerSlug(t *testing.T) {
	ts := pgtype.Timestamptz{Time: time.Now(), Valid: true}
	rows := []db.ListSubscriptionUpdatesRow{
		{PublicSlug: "a", SavedSearchName: "Go", MatchedAt: ts},
		{PublicSlug: "a", SavedSearchName: "Go", MatchedAt: ts, NotifiedAt: ts},
		{PublicSlug: "b", SavedSearchName: "Rust", MatchedAt: ts},
		{PublicSlug: "c", SavedSearchName: "Rust", MatchedAt: ts},
	}
	out, truncated := foldUpdates(rows, "https://freehire.me", 2)
	if len(out) != 2 || !truncated {
		t.Fatalf("out = %d truncated = %v, want 2 and true", len(out), truncated)
	}
	if !out[0].Delivered || len(out[0].Subscriptions) != 1 || out[0].URL != "https://freehire.me/jobs/a" {
		t.Errorf("folded = %+v", out[0])
	}
}

func TestUpdatesSince(t *testing.T) {
	now := time.Date(2026, 10, 27, 12, 0, 0, 0, time.UTC)
	if got, _ := updatesSince(now, "", 0); !got.Equal(now.AddDate(0, 0, -subscriptionUpdatesDefaultDays)) {
		t.Errorf("default = %v", got)
	}
	if got, _ := updatesSince(now, "", 365); !got.Equal(now.AddDate(0, 0, -subscriptionUpdatesMaxDays)) {
		t.Errorf("capped = %v", got)
	}
	if _, err := updatesSince(now, "yesterday", 0); err == nil {
		t.Error("a non-RFC 3339 since must be refused")
	}
}
