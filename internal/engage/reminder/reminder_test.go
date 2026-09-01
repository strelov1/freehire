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
	schedule    ScheduleContext
	scheduleErr error

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
func (f *fakeRepo) GetScheduleContext(_ context.Context, _ int64) (ScheduleContext, error) {
	return f.schedule, f.scheduleErr
}
func (f *fakeRepo) UpsertReminder(_ context.Context, userID, jobID int64, fireAt time.Time, channels []string) error {
	f.upserted = append(f.upserted, upsertCall{userID, jobID, fireAt, channels})
	return nil
}
func (f *fakeRepo) CancelReminder(_ context.Context, userID, jobID int64) error {
	f.cancelled = append(f.cancelled, jobKey{userID, jobID})
	return nil
}

// durPtr is the test-side spelling of an optional duration setting.
func durPtr(d time.Duration) *time.Duration { return &d }

// fixedClock is a deterministic now() for fire-time assertions.
var fixedNow = time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)

func newService(repo *fakeRepo) *Service {
	s := New(repo)
	s.now = func() time.Time { return fixedNow }
	return s
}

// An unconfigured account rounds to the DefaultNotificationHour in UTC: the delay
// lands at 12:00 on 22 July, and the first 09:00 at or after that is the 23rd.
func TestScheduleOnSave_RoundsToTheDefaultNotificationHour(t *testing.T) {
	repo := &fakeRepo{settings: Settings{Enabled: true, Channels: []string{"telegram"}}}
	svc := newService(repo)

	if err := svc.ScheduleOnSave(context.Background(), 7, 42); err != nil {
		t.Fatalf("ScheduleOnSave: %v", err)
	}
	if len(repo.upserted) != 1 {
		t.Fatalf("want 1 upsert, got %d", len(repo.upserted))
	}
	got := repo.upserted[0]
	want := time.Date(2026, 7, 23, 9, 0, 0, 0, time.UTC)
	if !got.fireAt.Equal(want) {
		t.Errorf("fireAt = %v, want %v", got.fireAt, want)
	}
	if got.userID != 7 || got.jobID != 42 {
		t.Errorf("upsert key = (%d,%d), want (7,42)", got.userID, got.jobID)
	}
}

// The configured hour is read in the account's own zone, not in UTC: 12:00 UTC on
// the 22nd is 14:00 in Berlin, so that same day's 18:00 Berlin (16:00 UTC) is the
// first notification hour at or after the delay.
func TestScheduleOnSave_RoundsToTheConfiguredHourInTheAccountZone(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	repo := &fakeRepo{
		settings: Settings{Enabled: true, Channels: []string{"email"}},
		schedule: ScheduleContext{NotificationHour: durPtr(18 * time.Hour), Location: berlin},
	}
	svc := newService(repo)

	if err := svc.ScheduleOnSave(context.Background(), 7, 42); err != nil {
		t.Fatalf("ScheduleOnSave: %v", err)
	}
	want := time.Date(2026, 7, 22, 18, 0, 0, 0, berlin)
	if got := repo.upserted[0].fireAt; !got.Equal(want) {
		t.Errorf("fireAt = %v, want %v", got, want)
	}
}

// The point of the rounding: saves hours apart on one day become one delivery.
func TestScheduleOnSave_SameDaySavesShareOneFireTime(t *testing.T) {
	repo := &fakeRepo{settings: Settings{Enabled: true, Channels: []string{"email"}}}
	svc := newService(repo)

	svc.now = func() time.Time { return time.Date(2026, 7, 19, 10, 14, 0, 0, time.UTC) }
	if err := svc.ScheduleOnSave(context.Background(), 7, 1); err != nil {
		t.Fatalf("ScheduleOnSave: %v", err)
	}
	svc.now = func() time.Time { return time.Date(2026, 7, 19, 23, 41, 0, 0, time.UTC) }
	if err := svc.ScheduleOnSave(context.Background(), 7, 2); err != nil {
		t.Fatalf("ScheduleOnSave: %v", err)
	}

	if len(repo.upserted) != 2 {
		t.Fatalf("want 2 upserts, got %d", len(repo.upserted))
	}
	if !repo.upserted[0].fireAt.Equal(repo.upserted[1].fireAt) {
		t.Errorf("fire times diverged: %v vs %v", repo.upserted[0].fireAt, repo.upserted[1].fireAt)
	}
}

// Rounding only ever moves forward — a reminder never fires before its delay.
func TestScheduleOnSave_NeverFiresBeforeTheDelay(t *testing.T) {
	repo := &fakeRepo{
		settings: Settings{Enabled: true, Channels: []string{"email"}},
		// An hour the delay lands exactly on, and one just before it: both must
		// still be at or after now+delay.
		schedule: ScheduleContext{NotificationHour: durPtr(12 * time.Hour), Location: time.UTC},
	}
	svc := newService(repo)
	if err := svc.ScheduleOnSave(context.Background(), 7, 1); err != nil {
		t.Fatalf("ScheduleOnSave: %v", err)
	}
	repo.schedule.NotificationHour = durPtr(11 * time.Hour)
	if err := svc.ScheduleOnSave(context.Background(), 7, 2); err != nil {
		t.Fatalf("ScheduleOnSave: %v", err)
	}

	earliest := fixedNow.Add(DefaultDelayDays * 24 * time.Hour)
	for _, call := range repo.upserted {
		if call.fireAt.Before(earliest) {
			t.Errorf("fireAt %v is before the delay deadline %v", call.fireAt, earliest)
		}
	}
	// The hour the delay lands exactly on is that instant, not the day after.
	if got := repo.upserted[0].fireAt; !got.Equal(earliest) {
		t.Errorf("fireAt = %v, want the delay deadline %v", got, earliest)
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
