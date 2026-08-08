package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
)

type memoryThrottler struct {
	mu     sync.Mutex
	counts map[string]int
}

func newMemoryThrottler() *memoryThrottler {
	return &memoryThrottler{counts: make(map[string]int)}
}

func (m *memoryThrottler) Allow(_ context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c := m.counts[key]
	if c >= limit {
		return false, window, nil
	}
	m.counts[key]++
	return true, 0, nil
}

func TestThrottleMiddleware_AllowsUnderLimitAndBlocksOverLimit(t *testing.T) {
	th := newMemoryThrottler()
	app := fiber.New()
	app.Post("/login", ThrottleMiddleware(th, func(c *fiber.Ctx) string { return "login:" + c.IP() }, 3, time.Minute), func(c *fiber.Ctx) error {
		return c.SendStatus(fiber.StatusOK)
	})

	for i := 0; i < 3; i++ {
		req := httptest.NewRequest(fiber.MethodPost, "/login", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("Test request %d: %v", i, err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("request %d status = %d, want 200", i, resp.StatusCode)
		}
	}

	// 4th request must be blocked with 429 Too Many Requests
	req := httptest.NewRequest(fiber.MethodPost, "/login", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("Test request 4: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("request 4 status = %d, want 429 Too Many Requests", resp.StatusCode)
	}
	if resp.Header.Get("Retry-After") == "" {
		t.Error("expected Retry-After header on 429 response")
	}
}

func TestPGThrottler_NilPoolFailsOpen(t *testing.T) {
	th := NewPGThrottler(nil)
	allowed, retryAfter, err := th.Allow(context.Background(), "test-key", 5, time.Minute)
	if err != nil {
		t.Fatalf("Allow: %v", err)
	}
	if !allowed {
		t.Error("expected nil pool to fail open (allowed = true)")
	}
	if retryAfter != 0 {
		t.Errorf("retryAfter = %v, want 0", retryAfter)
	}
}

type timeMemoryThrottler struct {
	mu         sync.Mutex
	counts     map[string]int
	lastWindow map[string]time.Time
	now        func() time.Time
}

func newTimeMemoryThrottler(now func() time.Time) *timeMemoryThrottler {
	return &timeMemoryThrottler{
		counts:     make(map[string]int),
		lastWindow: make(map[string]time.Time),
		now:        now,
	}
}

func (m *timeMemoryThrottler) Allow(_ context.Context, key string, limit int, window time.Duration) (bool, time.Duration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	curr := m.now()
	if last, ok := m.lastWindow[key]; !ok || curr.Sub(last) >= window {
		m.counts[key] = 0
		m.lastWindow[key] = curr
	}
	if m.counts[key] >= limit {
		rem := window - curr.Sub(m.lastWindow[key])
		if rem < 0 {
			rem = time.Second
		}
		return false, rem, nil
	}
	m.counts[key]++
	return true, 0, nil
}

func TestThrottle_WindowExpiry(t *testing.T) {
	nowTime := time.Now()
	th := newTimeMemoryThrottler(func() time.Time { return nowTime })

	// Consume 2 out of 2 limit
	allowed, _, _ := th.Allow(context.Background(), "user1", 2, 5*time.Minute)
	if !allowed {
		t.Fatal("req 1 should be allowed")
	}
	allowed, _, _ = th.Allow(context.Background(), "user1", 2, 5*time.Minute)
	if !allowed {
		t.Fatal("req 2 should be allowed")
	}

	// 3rd attempt is blocked
	allowed, _, _ = th.Allow(context.Background(), "user1", 2, 5*time.Minute)
	if allowed {
		t.Fatal("req 3 should be blocked")
	}

	// Advance clock past 5 minutes
	nowTime = nowTime.Add(6 * time.Minute)

	// Should be allowed again after window expiry
	allowed, _, _ = th.Allow(context.Background(), "user1", 2, 5*time.Minute)
	if !allowed {
		t.Fatal("req after window expiry should be allowed")
	}
}

func TestThrottle_SeparateKeys(t *testing.T) {
	th := newMemoryThrottler()

	// IP-1 exceeds budget
	th.Allow(context.Background(), "ip:1.1.1.1", 1, time.Minute)
	allowed1, _, _ := th.Allow(context.Background(), "ip:1.1.1.1", 1, time.Minute)
	if allowed1 {
		t.Fatal("IP-1 should be blocked after 1 attempt")
	}

	// IP-2 is independent and allowed
	allowed2, _, _ := th.Allow(context.Background(), "ip:2.2.2.2", 1, time.Minute)
	if !allowed2 {
		t.Fatal("IP-2 should be allowed independently")
	}
}

func TestThrottle_SharedBudget(t *testing.T) {
	// Simulate two throttler instances accessing the same shared storage
	sharedTh := newMemoryThrottler()

	instA := sharedTh
	instB := sharedTh

	// Instance A consumes 3 of 5 limit
	for i := 0; i < 3; i++ {
		allowed, _, _ := instA.Allow(context.Background(), "shared-key", 5, time.Minute)
		if !allowed {
			t.Fatalf("instA request %d should be allowed", i)
		}
	}

	// Instance B consumes remaining 2
	for i := 0; i < 2; i++ {
		allowed, _, _ := instB.Allow(context.Background(), "shared-key", 5, time.Minute)
		if !allowed {
			t.Fatalf("instB request %d should be allowed", i)
		}
	}

	// 6th attempt on Instance B must be blocked because shared limit (5) was reached
	allowed, _, _ := instB.Allow(context.Background(), "shared-key", 5, time.Minute)
	if allowed {
		t.Fatal("6th request on instB should be blocked by shared limit")
	}
}
