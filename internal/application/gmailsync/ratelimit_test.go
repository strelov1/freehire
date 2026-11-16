package gmailsync

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type scriptedTransport struct {
	answers []int // HTTP status per call; 403 carries rateLimitExceeded
	reason  string
	calls   int
}

func (s *scriptedTransport) RoundTrip(*http.Request) (*http.Response, error) {
	status := http.StatusOK
	if s.calls < len(s.answers) {
		status = s.answers[s.calls]
	}
	s.calls++
	body := `{"id":"m1","threadId":"t1","internalDate":"1700000000000","payload":{"mimeType":"text/plain"}}`
	if status != http.StatusOK {
		body = `{"error":{"errors":[{"reason":"` + s.reason + `"}]}}`
	}
	return &http.Response{StatusCode: status, Status: http.StatusText(status), Body: io.NopCloser(strings.NewReader(body))}, nil
}

func readerWith(tr *scriptedTransport, slept *[]time.Duration) *apiReader {
	r := NewAPIReader(&http.Client{Transport: tr}, nil).(*apiReader)
	r.sleep = func(_ context.Context, d time.Duration) error { *slept = append(*slept, d); return nil }
	return r
}

func TestGetMessageWaitsOutARateLimit(t *testing.T) {
	tr := &scriptedTransport{answers: []int{403, 403}, reason: "rateLimitExceeded"}
	var slept []time.Duration
	m, err := readerWith(tr, &slept).GetMessage(context.Background(), "m1")
	if err != nil {
		t.Fatalf("GetMessage: %v", err)
	}
	if m.ID != "m1" || tr.calls != 3 {
		t.Errorf("message %q after %d calls, want m1 after 3", m.ID, tr.calls)
	}
	if len(slept) != 2 || slept[0] != limitFirstWait || slept[1] != 2*limitFirstWait {
		t.Errorf("slept %v, want %v then double", slept, limitFirstWait)
	}
}

func TestRateLimitWaitsStopAtTheBudget(t *testing.T) {
	answers := make([]int, 100)
	for i := range answers {
		answers[i] = 403
	}
	tr := &scriptedTransport{answers: answers, reason: "rateLimitExceeded"}
	var slept []time.Duration
	r := readerWith(tr, &slept)
	_, err := r.GetMessage(context.Background(), "m1")
	if !LimitHit(err) {
		t.Fatalf("err = %v, want the rate-limit refusal once the budget is spent", err)
	}
	var total time.Duration
	for _, d := range slept {
		total += d
		if d > limitMaxWait {
			t.Errorf("a pause of %v exceeds %v", d, limitMaxWait)
		}
	}
	if total > limitWaitBudget || r.waited != total {
		t.Errorf("waited %v (recorded %v), want at most %v", total, r.waited, limitWaitBudget)
	}
	// The budget is the mailbox's for the whole run: a later call spends only what is left.
	if _, err := r.GetMessage(context.Background(), "m2"); !LimitHit(err) || r.waited > limitWaitBudget {
		t.Errorf("second call err = %v, waited %v in all; want a refusal within %v", err, r.waited, limitWaitBudget)
	}
}

func TestNoWaitForADailyQuotaOrARefusedGrant(t *testing.T) {
	for _, tc := range []struct {
		status int
		reason string
	}{{403, "dailyLimitExceeded"}, {403, "insufficientPermissions"}, {401, ""}} {
		tr := &scriptedTransport{answers: []int{tc.status}, reason: tc.reason}
		var slept []time.Duration
		if _, err := readerWith(tr, &slept).GetMessage(context.Background(), "m1"); err == nil || len(slept) != 0 || tr.calls != 1 {
			t.Errorf("%d %s: err=%v slept=%v calls=%d, want one call and no wait", tc.status, tc.reason, err, slept, tr.calls)
		}
	}
}
