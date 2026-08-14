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

	upserted  []upsertCall
	cancelled []jobKey
}

type jobKey struct{ userID, jobID int64 }
type upsertCall struct {
	userID, jobID int64
	fireAt        time.Time
	channels      []string
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

// fixedClock is a deterministic now() for fire-time assertions.
var fixedNow = time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)

func newService(repo *fakeRepo) *Service {
	s := New(repo)
	s.now = func() time.Time { return fixedNow }
	return s
}

func TestScheduleOnSave_UsesFixedDelay(t *testing.T) {
	repo := &fakeRepo{settings: Settings{Enabled: true, Channels: []string{"telegram"}}}
	svc := newService(repo)

	if err := svc.ScheduleOnSave(context.Background(), 7, 42); err != nil {
		t.Fatalf("ScheduleOnSave: %v", err)
	}
	if len(repo.upserted) != 1 {
		t.Fatalf("want 1 upsert, got %d", len(repo.upserted))
	}
	got := repo.upserted[0]
	want := fixedNow.Add(DefaultDelayDays * 24 * time.Hour)
	if !got.fireAt.Equal(want) {
		t.Errorf("fireAt = %v, want %v", got.fireAt, want)
	}
	if got.userID != 7 || got.jobID != 42 {
		t.Errorf("upsert key = (%d,%d), want (7,42)", got.userID, got.jobID)
	}
}

func TestScheduleOnSave_DisabledRuleSchedulesNothing(t *testing.T) {
	repo := &fakeRepo{settings: Settings{Enabled: false, Channels: []string{"email"}}}
	svc := newService(repo)

	if err := svc.ScheduleOnSave(context.Background(), 1, 1); err != nil {
		t.Fatalf("ScheduleOnSave: %v", err)
	}
	if len(repo.upserted) != 0 || len(repo.cancelled) != 0 {
		t.Errorf("disabled rule must be a no-op: upserts=%d cancels=%d", len(repo.upserted), len(repo.cancelled))
	}
}

func TestScheduleOnSave_UsesSettingsChannels(t *testing.T) {
	repo := &fakeRepo{settings: Settings{Enabled: true, Channels: []string{"telegram", "email"}}}
	svc := newService(repo)

	if err := svc.ScheduleOnSave(context.Background(), 1, 1); err != nil {
		t.Fatalf("ScheduleOnSave: %v", err)
	}
	got := repo.upserted[0].channels
	if len(got) != 2 || got[0] != "telegram" || got[1] != "email" {
		t.Errorf("channels snapshot = %v, want [telegram email]", got)
	}
}

func TestUpdateSettings_RejectsEnabledWithoutChannels(t *testing.T) {
	svc := newService(&fakeRepo{})
	_, err := svc.UpdateSettings(context.Background(), 1, Settings{Enabled: true, Channels: nil})
	if !errors.Is(err, ErrNoChannels) {
		t.Errorf("want ErrNoChannels, got %v", err)
	}
}

func TestUpdateSettings_RejectsUnknownChannel(t *testing.T) {
	svc := newService(&fakeRepo{})
	_, err := svc.UpdateSettings(context.Background(), 1, Settings{Enabled: true, Channels: []string{"carrier-pigeon"}})
	if !errors.Is(err, ErrInvalidChannel) {
		t.Errorf("want ErrInvalidChannel, got %v", err)
	}
}

func TestUpdateSettings_RejectsUnknownFrequency(t *testing.T) {
	svc := newService(&fakeRepo{})
	_, err := svc.UpdateSettings(context.Background(), 1, Settings{DigestFrequency: "weekly"})
	if !errors.Is(err, ErrInvalidFrequency) {
		t.Errorf("want ErrInvalidFrequency, got %v", err)
	}
}

func TestUpdateSettings_DailyRequiresDigestTime(t *testing.T) {
	svc := newService(&fakeRepo{})
	_, err := svc.UpdateSettings(context.Background(), 1, Settings{DigestFrequency: "daily"})
	if !errors.Is(err, ErrMissingDigestTime) {
		t.Errorf("want ErrMissingDigestTime, got %v", err)
	}
}

func TestUpdateSettings_DailyWithDigestTimeAccepted(t *testing.T) {
	svc := newService(&fakeRepo{})
	digestTime := 9 * time.Hour
	_, err := svc.UpdateSettings(context.Background(), 1, Settings{DigestFrequency: "daily", DigestTime: &digestTime})
	if err != nil {
		t.Errorf("want no error, got %v", err)
	}
}

func TestUpdateSettings_InstantDefaultAccepted(t *testing.T) {
	svc := newService(&fakeRepo{})
	_, err := svc.UpdateSettings(context.Background(), 1, Settings{DigestFrequency: "instant"})
	if err != nil {
		t.Errorf("want no error, got %v", err)
	}
}

func TestUpdateSettings_RejectsPartialQuietHours(t *testing.T) {
	svc := newService(&fakeRepo{})
	start := 22 * time.Hour
	_, err := svc.UpdateSettings(context.Background(), 1, Settings{QuietHoursStart: &start})
	if !errors.Is(err, ErrIncompleteQuietHours) {
		t.Errorf("want ErrIncompleteQuietHours, got %v", err)
	}
}

func TestUpdateSettings_BothQuietHoursAccepted(t *testing.T) {
	svc := newService(&fakeRepo{})
	start, end := 22*time.Hour, 8*time.Hour
	_, err := svc.UpdateSettings(context.Background(), 1, Settings{QuietHoursStart: &start, QuietHoursEnd: &end})
	if err != nil {
		t.Errorf("want no error, got %v", err)
	}
}

func TestUpdateSettings_NeitherQuietHoursAccepted(t *testing.T) {
	svc := newService(&fakeRepo{})
	_, err := svc.UpdateSettings(context.Background(), 1, Settings{})
	if err != nil {
		t.Errorf("want no error, got %v", err)
	}
}

func TestCancel_IsIdempotent(t *testing.T) {
	repo := &fakeRepo{}
	svc := newService(repo)
	if err := svc.Cancel(context.Background(), 5, 9); err != nil {
		t.Fatalf("Cancel: %v", err)
	}
	if len(repo.cancelled) != 1 || repo.cancelled[0] != (jobKey{5, 9}) {
		t.Errorf("cancelled = %v, want one call for (5,9)", repo.cancelled)
	}
}
