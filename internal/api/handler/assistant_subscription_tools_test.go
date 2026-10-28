package handler

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

func ptr[T any](v T) *T { return &v }

func TestApplyAlertSearch_DropsUnknownValuesAndRefusesUnknownNames(t *testing.T) {
	vals, ignored, err := applyAlertSearch(nil, alertSearchArgs{
		Query: ptr(" rust "),
		Filters: &assistantFilterArgs{
			"skills":    {"go", "golangg", "go"},
			"work_mode": {"remote", "sometimes"},
		},
		SalaryMin: ptr(80000),
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := encodeSavedQuery(vals); got != "q=rust&salary_min=80000&skills=go&work_mode=remote" {
		t.Errorf("query = %q", got)
	}
	if strings.Join(ignored, " ") != "skills=golangg work_mode=sometimes" {
		t.Errorf("ignored = %v", ignored)
	}
	if _, _, err := applyAlertSearch(nil, alertSearchArgs{Filters: &assistantFilterArgs{"specializations": {"x"}}}); err == nil {
		t.Error("an unknown filter name must be refused, as search_jobs refuses it")
	}
}

// An update replaces only what it names: new filters replace the facets but keep the
// text, the salary bound and what only the website sets; a 0 bound clears it.
func TestApplyAlertSearch_PartialUpdate(t *testing.T) {
	base := parseSavedQuery("q=backend&skills=go,rust&salary_min=80000&skills_exclude=php&visa_sponsorship=true")
	vals, _, err := applyAlertSearch(base, alertSearchArgs{Filters: &assistantFilterArgs{"seniority": {"senior"}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := encodeSavedQuery(vals); got != "q=backend&salary_min=80000&seniority=senior&skills_exclude=php&visa_sponsorship=true" {
		t.Errorf("filters update = %q", got)
	}
	vals, _, _ = applyAlertSearch(base, alertSearchArgs{SalaryMin: ptr(0)})
	if vals.Has("salary_min") || len(vals["skills"]) != 2 {
		t.Errorf("salary clear = %v", vals)
	}
}

func TestAlertSearchOf_IsTheShapeTheToolsTake(t *testing.T) {
	got := alertSearchOf("q=backend&skills=go,rust&salary_min=80000&skills_exclude=php&sort=posted_at:desc")
	if got.Query != "backend" || strings.Join(got.Filters["skills"], ",") != "go,rust" || got.SalaryMin != 80000 {
		t.Errorf("search = %+v", got)
	}
	if len(got.Other) != 1 || got.Other["skills_exclude"] != "php" {
		t.Errorf("other = %v, want only the website-only exclusion", got.Other)
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
	if !out[0].Delivered || len(out[0].Alerts) != 1 || out[0].URL != "https://freehire.me/jobs/a" {
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
