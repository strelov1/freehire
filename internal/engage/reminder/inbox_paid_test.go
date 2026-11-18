package reminder

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ai/plan"
)

func TestFreeCannotEnableInboxDigest(t *testing.T) {
	repo := &fakeRepo{}
	_, err := newService(repo).UpdateSettings(context.Background(), 7, Settings{Enabled: true, Channels: []string{"email"}, DigestFrequency: "daily", DigestTime: durPtr(9 * time.Hour), InboxDigestEnabled: true})
	if !errors.Is(err, plan.ErrPaymentRequired) {
		t.Fatalf("free enable error=%v, want payment required", err)
	}
	if repo.settings.InboxDigestEnabled {
		t.Fatal("free account enabled digest")
	}
}

func TestPaidInboxConsentAndPlanReadFailure(t *testing.T) {
	for _, tc := range []struct {
		name    string
		paid    bool
		readErr error
		want    error
	}{{"paid", true, nil, nil}, {"unavailable", false, errors.New("database unavailable"), ErrPlanUnavailable}} {
		t.Run(tc.name, func(t *testing.T) {
			repo := &fakeRepo{paid: tc.paid, planErr: tc.readErr}
			_, err := newService(repo).UpdateSettings(context.Background(), 7, Settings{Enabled: true, Channels: []string{"email"}, InboxDigestEnabled: true, DigestFrequency: "daily", DigestTime: durPtr(9 * time.Hour)})
			if !errors.Is(err, tc.want) {
				t.Fatalf("error=%v want=%v", err, tc.want)
			}
			if tc.want != nil && repo.settings.InboxDigestEnabled {
				t.Fatal("unverified entitlement enabled digest")
			}
		})
	}
}
