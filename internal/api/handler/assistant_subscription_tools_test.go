package handler

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/ai/assistant"
	"github.com/strelov1/freehire/internal/engage/subscription"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/observability"
	"github.com/strelov1/freehire/internal/search/savedsearch"
	"github.com/strelov1/freehire/internal/search/search"
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
	if got := savedsearch.EncodeQuery(vals); got != "q=rust&salary_min=80000&skills=go&work_mode=remote" {
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
	base := savedsearch.ParseQuery("q=backend&skills=go,rust&salary_min=80000&skills_exclude=php&visa_sponsorship=true")
	vals, _, err := applyAlertSearch(base, alertSearchArgs{Filters: &assistantFilterArgs{"seniority": {"senior"}}})
	if err != nil {
		t.Fatal(err)
	}
	if got := savedsearch.EncodeQuery(vals); got != "q=backend&salary_min=80000&seniority=senior&skills_exclude=php&visa_sponsorship=true" {
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

// A refusal the caller caused must reach the agent as invalid_request / conflict with words it
// can act on; subscribe answered internal_error for a q with too many OR alternatives (the
// saved-search parser refuses it) and for a push subscription with no device.
func TestAlertSavedSearchError_GivesUserCausedRefusalsACode(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		code string
	}{
		{"too many OR alternatives", search.ErrTooManyAlternatives, "invalid_request"},
		{"malformed query", fmt.Errorf("%w: visa_sponsorship must be true or false", search.ErrBadQuery), "invalid_request"},
		{"no push device", subscription.ErrNoPushDevice, "conflict"},
		{"unsupported channel", subscription.ErrInvalidChannel, "invalid_request"},
		{"duplicate name", savedsearch.ErrDuplicateName, "conflict"},
		{"a real fault", errors.New("connection reset"), "internal_error"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := observability.ErrorCode(alertSavedSearchError(tc.err, "x")); got != tc.code {
				t.Errorf("code = %q, want %q", got, tc.code)
			}
		})
	}
}

func TestUnknownFilterAndBadQueryNameTheirArgument(t *testing.T) {
	_, err := assistantFilterArgs{"bogus_facet": {"x"}}.values()
	if got := assistant.ArgField(err); got != "filters.bogus_facet" || observability.ErrorCode(err) != "invalid_request" {
		t.Errorf("unknown filter: field=%q code=%q", got, observability.ErrorCode(err))
	}
	err = searchToolError(search.ErrTooManyAlternatives)
	if got := assistant.ArgField(err); got != "query" || observability.ErrorCode(err) != "invalid_request" {
		t.Errorf("too many alternatives: field=%q code=%q", got, observability.ErrorCode(err))
	}
}
