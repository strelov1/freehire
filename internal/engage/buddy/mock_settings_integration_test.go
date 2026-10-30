//go:build integration

package buddy_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/engage/buddy"
	"github.com/strelov1/freehire/internal/engage/slots"
)

func TestMockSettingsRefuseAnUnknownZoneAndStoreNothing(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	ctx := context.Background()
	u, _ := joined(t, pool, svc, "a@example.test")

	_, err := mocks.SaveSettings(ctx, u, buddy.MockSettings{
		Timezone: "Mars/Olympus", MeetingURL: "https://meet.example.test/a", Topics: []string{buddy.TopicCoding},
	})
	if !errors.Is(err, buddy.ErrInvalidMockSettings) {
		t.Fatalf("err = %v, want ErrInvalidMockSettings", err)
	}
	if _, ok, err := mocks.Settings(ctx, u); err != nil || ok {
		t.Fatalf("settings stored after a refusal: ok=%v err=%v", ok, err)
	}
}

func TestMockSettingsRefuseAPlainHTTPLink(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	u, _ := joined(t, pool, svc, "a@example.test")

	_, err := mocks.SaveSettings(context.Background(), u, buddy.MockSettings{
		Timezone: "UTC", MeetingURL: "http://meet.example.test/a", Topics: []string{buddy.TopicCoding},
	})
	if !errors.Is(err, buddy.ErrInvalidMockSettings) {
		t.Fatalf("err = %v, want ErrInvalidMockSettings", err)
	}
}

func TestMockSettingsRoundTrip(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	ctx := context.Background()
	u, _ := joined(t, pool, svc, "a@example.test")

	if _, err := mocks.SaveSettings(ctx, u, buddy.MockSettings{
		Timezone: "Europe/Berlin", MeetingURL: "https://meet.example.test/a",
		Topics: []string{buddy.TopicBehavioral, buddy.TopicCoding}, Paused: true,
	}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := mocks.Settings(ctx, u)
	if err != nil || !ok {
		t.Fatalf("settings = %v, %v", ok, err)
	}
	if got.Timezone != "Europe/Berlin" || !got.Paused ||
		!slices.Equal(got.Topics, []string{buddy.TopicCoding, buddy.TopicBehavioral}) {
		t.Fatalf("settings = %+v", got)
	}
}

func TestMockSettingsNeedAnActiveMember(t *testing.T) {
	pool, _, mocks, _ := mockSetup(t)
	outsider := seedUser(t, pool, "outsider@example.test")

	_, err := mocks.SaveSettings(context.Background(), outsider, buddy.MockSettings{
		Timezone: "UTC", MeetingURL: "https://meet.example.test/a", Topics: []string{buddy.TopicCoding},
	})
	if !errors.Is(err, buddy.ErrNotMember) {
		t.Fatalf("err = %v, want ErrNotMember", err)
	}
}

func TestMockAvailabilityWeekAndOverrides(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	ctx := context.Background()
	u, _ := joined(t, pool, svc, "a@example.test")
	other, _ := joined(t, pool, svc, "b@example.test")

	evening := weekly(t, time.Tuesday, 19, 21)
	morning := weekly(t, time.Saturday, 9, 12)
	if err := mocks.ReplaceWeekly(ctx, u, []slots.Rule{evening, morning}); err != nil {
		t.Fatal(err)
	}
	day, err := slots.NewDate(2026, time.December, 1)
	if err != nil {
		t.Fatal(err)
	}
	midnight, err := slots.NewTimeOfDay(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	closure, err := slots.NewDatedRule(day, midnight, midnight)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := mocks.AddOverride(ctx, u, closure)
	if err != nil || stored.ID() == 0 {
		t.Fatalf("add override = %+v, %v; want a stored row id", stored, err)
	}

	// Replacing the week leaves the override alone.
	if err := mocks.ReplaceWeekly(ctx, u, []slots.Rule{evening}); err != nil {
		t.Fatal(err)
	}
	rules, err := mocks.Availability(ctx, u)
	if err != nil || len(rules) != 2 {
		t.Fatalf("availability = %d rules, %v; want the evening and the override", len(rules), err)
	}

	if err := mocks.DeleteRule(ctx, other, stored.ID()); !errors.Is(err, buddy.ErrNotFound) {
		t.Fatalf("deleting another member's rule: err = %v, want ErrNotFound", err)
	}
	if err := mocks.DeleteRule(ctx, u, stored.ID()); err != nil {
		t.Fatal(err)
	}
	if err := mocks.ReplaceWeekly(ctx, u, []slots.Rule{closure}); !errors.Is(err, slots.ErrInvalidSpan) {
		t.Fatalf("a dated rule in the week: err = %v, want ErrInvalidSpan", err)
	}
	if _, err := mocks.AddOverride(ctx, u, evening); !errors.Is(err, slots.ErrInvalidSpan) {
		t.Fatalf("a weekly rule as an override: err = %v, want ErrInvalidSpan", err)
	}
}
