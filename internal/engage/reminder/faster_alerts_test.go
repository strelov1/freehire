package reminder

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/engage/notify"
)

func fasterUpdate(freq string, hours int) Settings {
	return Settings{
		Enabled: true, Channels: []string{notify.ChannelEmail},
		DigestFrequency: freq, DigestTime: durPtr(9 * time.Hour), DigestIntervalHours: hours,
	}
}

func TestFreeCannotChooseFasterThanDaily(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stored Settings
		in     Settings
	}{
		{"daily to instant", Settings{DigestFrequency: notify.FrequencyDaily}, fasterUpdate(notify.FrequencyInstant, 0)},
		{"daily to interval", Settings{DigestFrequency: notify.FrequencyDaily}, fasterUpdate(notify.FrequencyInterval, 3)},
		{"instant to interval", Settings{DigestFrequency: notify.FrequencyInstant}, fasterUpdate(notify.FrequencyInterval, 1)},
		{"interval hours changed", Settings{DigestFrequency: notify.FrequencyInterval, DigestIntervalHours: 6}, fasterUpdate(notify.FrequencyInterval, 2)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{settings: tc.stored}
			_, err := newService(repo).UpdateSettings(context.Background(), 7, tc.in)
			if !errors.Is(err, ErrFasterAlertsNeedPaidPlan) {
				t.Fatalf("error=%v, want ErrFasterAlertsNeedPaidPlan", err)
			}
			if repo.settings.DigestFrequency != tc.stored.DigestFrequency {
				t.Fatalf("refused write still stored %q", repo.settings.DigestFrequency)
			}
		})
	}
}

func TestFreeKeepsHistoricalFasterPreferenceOnUnrelatedEdits(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stored Settings
		in     Settings
	}{
		{"instant echoed back", Settings{DigestFrequency: notify.FrequencyInstant}, fasterUpdate(notify.FrequencyInstant, 0)},
		{"interval echoed back", Settings{DigestFrequency: notify.FrequencyInterval, DigestIntervalHours: 6}, fasterUpdate(notify.FrequencyInterval, 6)},
		{"interval hours omitted", Settings{DigestFrequency: notify.FrequencyInterval, DigestIntervalHours: 6}, fasterUpdate(notify.FrequencyInterval, 0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{settings: tc.stored}
			tc.in.Channels = []string{notify.ChannelTelegram} // the unrelated edit
			got, err := newService(repo).UpdateSettings(context.Background(), 7, tc.in)
			if err != nil {
				t.Fatalf("unrelated edit refused: %v", err)
			}
			if got.DigestFrequency != tc.stored.DigestFrequency {
				t.Fatalf("stored preference rewritten to %q, want %q kept for a later renewal", got.DigestFrequency, tc.stored.DigestFrequency)
			}
		})
	}
}

func TestFreeCanStillMoveToDailyAndOptOut(t *testing.T) {
	repo := &fakeRepo{settings: Settings{DigestFrequency: notify.FrequencyInstant}}
	if _, err := newService(repo).UpdateSettings(context.Background(), 7, fasterUpdate(notify.FrequencyDaily, 0)); err != nil {
		t.Fatalf("daily refused: %v", err)
	}
	off := fasterUpdate(notify.FrequencyDaily, 0)
	off.Enabled = false
	off.Channels = nil
	if _, err := newService(repo).UpdateSettings(context.Background(), 7, off); err != nil {
		t.Fatalf("opt-out refused: %v", err)
	}
}

func TestPaidMayChooseFasterAndPlanReadFailureFailsClosed(t *testing.T) {
	repo := &fakeRepo{paid: true}
	for _, in := range []Settings{fasterUpdate(notify.FrequencyInstant, 0), fasterUpdate(notify.FrequencyInterval, 4)} {
		got, err := newService(repo).UpdateSettings(context.Background(), 7, in)
		if err != nil || got.DigestFrequency != in.DigestFrequency {
			t.Fatalf("paid %q: got=%q err=%v", in.DigestFrequency, got.DigestFrequency, err)
		}
	}
	broken := &fakeRepo{planErr: errors.New("db down")}
	_, err := newService(broken).UpdateSettings(context.Background(), 7, fasterUpdate(notify.FrequencyInstant, 0))
	if !errors.Is(err, ErrPlanUnavailable) {
		t.Fatalf("error=%v, want ErrPlanUnavailable", err)
	}
	if broken.settings.DigestFrequency == notify.FrequencyInstant {
		t.Fatal("unverified plan stored a faster frequency")
	}
}
