package handler

import (
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/engage/mentorship"
)

// A moderator (and the owner) can see when a profile was submitted; the public response
// never carries it — nothing outside the cabinet/queue needs it, and it is not part of
// what makes a mentor look trustworthy on the public card.
func TestCreatedAtIsModeratorAndOwnerOnly(t *testing.T) {
	submitted := time.Date(2026, time.September, 1, 12, 0, 0, 0, time.UTC)
	p := mentorship.Profile{CreatedAt: submitted}

	if got := toMentorResponse(p).CreatedAt; got != nil {
		t.Errorf("public response carries created_at = %v, want nil", got)
	}
	if got := toModeratorMentorResponse(p).CreatedAt; got == nil || !got.Equal(submitted) {
		t.Errorf("moderator response created_at = %v, want %v", got, submitted)
	}
	if got := toOwnMentorResponse(p).CreatedAt; got == nil || !got.Equal(submitted) {
		t.Errorf("owner response created_at = %v, want %v", got, submitted)
	}
}
