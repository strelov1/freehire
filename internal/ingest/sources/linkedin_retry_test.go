package sources

import (
	"context"
	"errors"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// throttledDetail answers the first `throttled` requests for a posting detail with 429,
// then delegates — the shape of one throttled proxy exit followed by a served one.
type throttledDetail struct {
	next      HTMLResolvedGetter
	throttled int
	calls     int
}

func (t *throttledDetail) GetHTMLResolved(ctx context.Context, raw string) (*html.Node, string, error) {
	if strings.Contains(raw, "/jobs-guest/jobs/api/jobPosting/") {
		t.calls++
		if t.calls <= t.throttled {
			return nil, "", &StatusError{Method: "GET", URL: raw, Code: 429}
		}
	}
	return t.next.GetHTMLResolved(ctx, raw)
}

func TestLinkedInRetriesATransientDetailThrottle(t *testing.T) {
	listing := `<div data-entity-urn="urn:li:jobPosting:123"><h3 class="base-search-card__title">Engineer</h3><h4 class="base-search-card__subtitle">Acme</h4></div>`
	for _, tc := range []struct {
		name      string
		throttled int
		wantRead  bool
	}{
		{"one throttled exit is retried", 1, true},
		{"retries exhausted stop the search", linkedInThrottleRetries + 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fake := &throttledDetail{throttled: tc.throttled, next: (&routedHTTP{}).route("start=0", listing).route("/jobs-guest/jobs/api/jobPosting/123", linkedInOpenPage)}
			s := NewLinkedIn(fake).(linkedIn)
			s.pace, s.backoff = &recordingWaiter{}, 0
			jobs, err := s.Fetch(context.Background(), CompanyEntry{Board: "Go", Region: "London"})
			if tc.wantRead {
				if err != nil || len(jobs) != 1 || jobs[0].Description == "" || fake.calls != 2 {
					t.Fatalf("transient 429 lost the posting: jobs=%+v calls=%d err=%v", jobs, fake.calls, err)
				}
				return
			}
			var status *StatusError
			if !errors.As(err, &status) || status.Code != 429 || fake.calls != linkedInThrottleRetries+1 {
				t.Fatalf("persistent refusal must stop after bounded retries: calls=%d err=%v", fake.calls, err)
			}
		})
	}
}
