package sources

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestHigherMePacesRetriesAcrossBoards(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	gate := &recordingWaiter{}
	original := &Client{httpClient: srv.Client(), maxRetries: 1}
	getter := highermeAttemptGetter(original, gate)
	for range 2 {
		if err := getter.GetJSON(context.Background(), srv.URL, &struct{}{}); err != nil {
			t.Fatal(err)
		}
	}
	if attempts != 3 || gate.calls != 3 {
		t.Fatalf("attempts=%d admissions=%d, want 3 each", attempts, gate.calls)
	}
	if original.httpClient.Transport != srv.Client().Transport {
		t.Fatal("modified shared client")
	}
}

func TestHigherMeCancelledAdmissionSendsNoRequest(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { attempts++ }))
	defer srv.Close()
	gate := &recordingWaiter{err: context.Canceled}
	getter := highermeAttemptGetter(&Client{httpClient: srv.Client()}, gate)
	if err := getter.GetJSON(context.Background(), srv.URL, &struct{}{}); err == nil {
		t.Fatal("cancelled admission succeeded")
	}
	if attempts != 0 || gate.calls != 1 {
		t.Fatalf("attempts=%d admissions=%d, want 0 and 1", attempts, gate.calls)
	}
}

type highermeDelayedAdmission struct{}

func (highermeDelayedAdmission) Wait(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(600 * time.Millisecond):
		return nil
	}
}

func TestHigherMeAdmissionDoesNotConsumeAttemptTimeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(`{}`)) }))
	defer srv.Close()
	original := srv.Client()
	original.Timeout = 500 * time.Millisecond
	getter := highermeAttemptGetter(&Client{httpClient: original}, highermeDelayedAdmission{})
	if err := getter.GetJSON(context.Background(), srv.URL, &struct{}{}); err != nil {
		t.Fatalf("queue wait consumed attempt budget: %v", err)
	}
}

type highermeAdmissionHook func(context.Context) error

func (f highermeAdmissionHook) Wait(ctx context.Context) error { return f(ctx) }

func TestHigherMeRechecksHoldRecordedDuringAdmission(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { attempts++ }))
	defer srv.Close()
	host := strings.TrimPrefix(srv.URL, "http://")
	backoff := newHostBackoff(time.Hour, time.Hour)
	gate := highermeAdmissionHook(func(context.Context) error { backoff.refused(host, time.Hour); return nil })
	getter := highermeAttemptGetter(&Client{httpClient: srv.Client(), backoff: backoff}, gate)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := getter.GetJSON(ctx, srv.URL, &struct{}{}); err == nil {
		t.Fatal("request ignored a new host hold")
	}
	if attempts != 0 {
		t.Fatalf("sent %d requests during host hold", attempts)
	}
}

func TestHigherMeSpacesRequestsAfterHoldExpires(t *testing.T) {
	arrivals := make(chan time.Time, 3)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { arrivals <- time.Now(); _, _ = w.Write([]byte(`{}`)) }))
	defer srv.Close()
	backoff := newHostBackoff(150*time.Millisecond, time.Second)
	host := strings.TrimPrefix(srv.URL, "http://")
	var once sync.Once
	limiter := rate.NewLimiter(rate.Every(40*time.Millisecond), 1)
	gate := highermeAdmissionHook(func(ctx context.Context) error {
		if err := limiter.Wait(ctx); err != nil {
			return err
		}
		once.Do(func() { backoff.refused(host, 150*time.Millisecond) })
		return nil
	})
	client := highermePacedHTTP(srv.Client(), gate, backoff)
	errs := make(chan error, 3)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	for range 3 {
		go func() {
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
			resp, err := client.Do(req)
			if err == nil {
				resp.Body.Close()
			}
			errs <- err
		}()
	}
	for range 3 {
		if err := <-errs; err != nil {
			t.Fatal(err)
		}
	}
	previous := <-arrivals
	for range 2 {
		next := <-arrivals
		if next.Sub(previous) < 25*time.Millisecond {
			t.Fatalf("requests burst after hold: %v", next.Sub(previous))
		}
		previous = next
	}
}

func TestHigherMeRegistryClientUsesAttemptAdmission(t *testing.T) {
	if _, ok := pacedHigherMeGetter(NewClient()).(*Client); !ok {
		t.Fatal("production getter did not install attempt admission")
	}
}
