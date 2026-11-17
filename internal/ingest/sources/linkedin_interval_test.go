package sources

import (
	"context"
	"strings"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestLinkedInRequestIntervalComesFromTheEnvironment(t *testing.T) {
	for _, tc := range []struct {
		env  string
		want time.Duration
	}{
		{"", 2 * time.Second},
		{"500ms", 500 * time.Millisecond},
		{"250ms", 250 * time.Millisecond},
	} {
		t.Setenv(linkedInIntervalEnv, tc.env)
		s := NewLinkedIn(nil).(linkedIn)
		lim, ok := s.pace.(*rate.Limiter)
		if s.configErr != nil || !ok || lim.Limit() != rate.Every(tc.want) {
			t.Errorf("%s=%q: limiter=%v err=%v, want one request per %s", linkedInIntervalEnv, tc.env, s.pace, s.configErr, tc.want)
		}
	}
}

func TestLinkedInRejectsAnUnsafeRequestInterval(t *testing.T) {
	for _, env := range []string{"fast", "0s", "-1s", "50ms", "1m"} {
		t.Setenv(linkedInIntervalEnv, env)
		s := NewLinkedIn(linkedInGetterFunc(nil)).(linkedIn)
		_, err := s.Fetch(context.Background(), CompanyEntry{Board: "Go", Region: "London"})
		if err == nil || !strings.Contains(err.Error(), linkedInIntervalEnv) {
			t.Errorf("%s=%q crawled instead of failing: %v", linkedInIntervalEnv, env, err)
		}
	}
}
