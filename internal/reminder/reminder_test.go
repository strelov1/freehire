package reminder

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeRepo records scheduling calls and serves canned settings.
type fakeRepo struct {
	settings    Settings
	settingsErr error

	jobIDBySlug map[string]int64

	upserted   []upsertCall
	cancelled  []jobKey
	rescheds   []reschedCall
	reschedErr error
}

type jobKey struct{ userID, jobID int64 }
type upsertCall struct {
	userID, jobID int64
	fireAt        time.Time
	channels      []string
}
type reschedCall struct {
	userID, jobID int64
	fireAt        time.Time
}

func (f *fakeRepo) JobIDBySlug(_ context.Context, slug string) (int64, error) {
	if id, ok := f.jobIDBySlug[slug]; ok {
		return id, nil
	}
	return 0, ErrJobNotFound
}
func (f *fakeRepo) GetSettings(_ context.Context, _ int64) (Settings, error) {
	return f.settings, f.settingsErr
}
func (f *fakeRepo) UpsertSettings(_ context.Context, _ int64, s Settings) (Settings, error) {
	f.settings = s
	return s, nil
}
func (f *fakeRepo) UpsertReminder(_ context.Context, userID, jobID int64, fireAt time.Time, channels []string) error {
	f.upserted = append(f.upserted, upsertCall{userID, jobID, fireAt, channels})
	return nil
}
func (f *fakeRepo) CancelReminder(_ context.Context, userID, jobID int64) error {
	f.cancelled = append(f.cancelled, jobKey{userID, jobID})
	return nil
}
func (f *fakeRepo) RescheduleReminder(_ context.Context, userID, jobID int64, fireAt time.Time) error {
	if f.reschedErr != nil {
		return f.reschedErr
	}
	f.rescheds = append(f.rescheds, reschedCall{userID, jobID, fireAt})
	return nil
}

// fixedClock is a deterministic now() for fire-time assertions.
var fixedNow = time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)

func newService(repo *fakeRepo) *Service {
	s := New(repo)
	s.now = func() time.Time { return fixedNow }
	return s
}

func TestScheduleOnSave_UsesAccountDefault(t *testing.T) {
	repo := &fakeRepo{settings: Settings{Enabled: true, DefaultDelayDays: 3, Channels: []string{"telegram"}}}
	svc := newService(repo)

	if err := svc.ScheduleOnSave(context.Background(), 7, 42, nil); err != nil {
		t.Fatalf("ScheduleOnSave: %v", err)
	}
	if len(repo.upserted) != 1 {
		t.Fatalf("want 1 upsert, got %d", len(repo.upserted))
	}
	got := repo.upserted[0]
	want := fixedNow.Add(3 * 24 * time.Hour)
	if !got.fireAt.Equal(want) {
		t.Errorf("fireAt = %v, want %v", got.fireAt, want)
	}
	if got.userID != 7 || got.jobID != 42 {
		t.Errorf("upsert key = (%d,%d), want (7,42)", got.userID, got.jobID)
	}
}

func TestScheduleOnSave_OverrideDelay(t *testing.T) {
	repo := &fakeRepo{settings: Settings{Enabled: true, DefaultDelayDays: 3, Channels: []string{"email"}}}
	svc := newService(repo)

	if err := svc.ScheduleOnSave(context.Background(), 1, 2, &Override{DelayDays: 1}); err != nil {
		t.Fatalf("ScheduleOnSave: %v", err)
	}
	if len(repo.upserted) != 1 {
		t.Fatalf("want 1 upsert, got %d", len(repo.upserted))
	}
	want := fixedNow.Add(1 * 24 * time.Hour)
	if !repo.upserted[0].fireAt.Equal(want) {
		t.Errorf("fireAt = %v, want %v (override should beat default)", repo.upserted[0].fireAt, want)
	}
}

func TestScheduleOnSave_OptOutCancels(t *testing.T) {
	repo := &fakeRepo{settings: Settings{Enabled: true, DefaultDelayDays: 3, Channels: []string{"email"}}}
	svc := newService(repo)

	if err := svc.ScheduleOnSave(context.Background(), 5, 9, &Override{Disabled: true}); err != nil {
		t.Fatalf("ScheduleOnSave: %v", err)
	}
	if len(repo.upserted) != 0 {
		t.Errorf("opt-out must not schedule, got %d upserts", len(repo.upserted))
	}
	if len(repo.cancelled) != 1 || repo.cancelled[0] != (jobKey{5, 9}) {
		t.Errorf("opt-out must cancel pending, got %v", repo.cancelled)
	}
}

func TestScheduleOnSave_DisabledRuleSchedulesNothing(t *testing.T) {
	repo := &fakeRepo{settings: Settings{Enabled: false, DefaultDelayDays: 3, Channels: []string{"email"}}}
	svc := newService(repo)

	if err := svc.ScheduleOnSave(context.Background(), 1, 1, nil); err != nil {
		t.Fatalf("ScheduleOnSave: %v", err)
	}
	if len(repo.upserted) != 0 || len(repo.cancelled) != 0 {
		t.Errorf("off-by-default must be a no-op: upserts=%d cancels=%d", len(repo.upserted), len(repo.cancelled))
	}
}

func TestScheduleOnSave_ExplicitOverrideSchedulesWhenRuleDisabled(t *testing.T) {
	// Reminders are off globally, but an explicit per-save "remind me tomorrow" is an
	// opt-in that stands on its own — it schedules, falling back to the email channel.
	repo := &fakeRepo{settings: Settings{Enabled: false, DefaultDelayDays: 3, Channels: nil}}
	svc := newService(repo)

	if err := svc.ScheduleOnSave(context.Background(), 1, 2, &Override{DelayDays: 1}); err != nil {
		t.Fatalf("ScheduleOnSave: %v", err)
	}
	if len(repo.upserted) != 1 {
		t.Fatalf("explicit override must schedule even when disabled, got %d upserts", len(repo.upserted))
	}
	got := repo.upserted[0]
	if !got.fireAt.Equal(fixedNow.Add(24 * time.Hour)) {
		t.Errorf("fireAt = %v, want +1 day", got.fireAt)
	}
	if len(got.channels) != 1 || got.channels[0] != "email" {
		t.Errorf("channels = %v, want [email] fallback", got.channels)
	}
}

func TestScheduleOnSave_UsesSettingsChannels(t *testing.T) {
	repo := &fakeRepo{settings: Settings{Enabled: true, DefaultDelayDays: 2, Channels: []string{"telegram", "email"}}}
	svc := newService(repo)

	if err := svc.ScheduleOnSave(context.Background(), 1, 1, nil); err != nil {
		t.Fatalf("ScheduleOnSave: %v", err)
	}
	got := repo.upserted[0].channels
	if len(got) != 2 || got[0] != "telegram" || got[1] != "email" {
		t.Errorf("channels snapshot = %v, want [telegram email]", got)
	}
}

func TestUpdateSettings_RejectsEnabledWithoutChannels(t *testing.T) {
	svc := newService(&fakeRepo{})
	_, err := svc.UpdateSettings(context.Background(), 1, Settings{Enabled: true, DefaultDelayDays: 3, Channels: nil})
	if !errors.Is(err, ErrNoChannels) {
		t.Errorf("want ErrNoChannels, got %v", err)
	}
}

func TestUpdateSettings_RejectsUnknownChannel(t *testing.T) {
	svc := newService(&fakeRepo{})
	_, err := svc.UpdateSettings(context.Background(), 1, Settings{Enabled: true, DefaultDelayDays: 3, Channels: []string{"carrier-pigeon"}})
	if !errors.Is(err, ErrInvalidChannel) {
		t.Errorf("want ErrInvalidChannel, got %v", err)
	}
}

func TestUpdateSettings_RejectsOutOfRangeDelay(t *testing.T) {
	svc := newService(&fakeRepo{})
	_, err := svc.UpdateSettings(context.Background(), 1, Settings{Enabled: true, DefaultDelayDays: 9999, Channels: []string{"email"}})
	if !errors.Is(err, ErrInvalidDelay) {
		t.Errorf("want ErrInvalidDelay, got %v", err)
	}
}

func TestRescheduleBySlug_NoPendingIsNotFound(t *testing.T) {
	repo := &fakeRepo{jobIDBySlug: map[string]int64{"acme-dev": 3}, reschedErr: ErrNoReminder}
	svc := newService(repo)
	err := svc.RescheduleBySlug(context.Background(), 1, "acme-dev", 7)
	if !errors.Is(err, ErrNoReminder) {
		t.Errorf("want ErrNoReminder, got %v", err)
	}
}

func TestRescheduleBySlug_SetsNewFireAt(t *testing.T) {
	repo := &fakeRepo{jobIDBySlug: map[string]int64{"acme-dev": 3}}
	svc := newService(repo)
	if err := svc.RescheduleBySlug(context.Background(), 1, "acme-dev", 7); err != nil {
		t.Fatalf("RescheduleBySlug: %v", err)
	}
	if len(repo.rescheds) != 1 {
		t.Fatalf("want 1 reschedule, got %d", len(repo.rescheds))
	}
	want := fixedNow.Add(7 * 24 * time.Hour)
	if !repo.rescheds[0].fireAt.Equal(want) {
		t.Errorf("fireAt = %v, want %v", repo.rescheds[0].fireAt, want)
	}
}
