package sponsored

import (
	"fmt"
	"testing"
	"time"
)

func TestVisitorHash(t *testing.T) {
	k := []byte("k")
	d1 := time.Date(2026, 11, 24, 23, 59, 0, 0, time.UTC)
	a := VisitorHash(k, "1.2.3.4", "ua", d1)
	if a != VisitorHash(k, "1.2.3.4", "ua", d1.Add(time.Minute-time.Second)) {
		t.Error("same day, same visitor must match")
	}
	for name, other := range map[string]string{
		"next day": VisitorHash(k, "1.2.3.4", "ua", d1.Add(time.Minute)),
		"ip":       VisitorHash(k, "1.2.3.5", "ua", d1),
		"ua":       VisitorHash(k, "1.2.3.4", "ub", d1),
		"key":      VisitorHash([]byte("j"), "1.2.3.4", "ua", d1),
	} {
		if other == a {
			t.Errorf("%s must change the hash", name)
		}
	}
}

func TestIPLimiter(t *testing.T) {
	l := NewIPLimiter(20, time.Minute)
	now := time.Now()
	for i := 1; i <= 20; i++ {
		if !l.Allow("a", now) {
			t.Fatalf("hit %d refused", i)
		}
	}
	if l.Allow("a", now) {
		t.Error("21st hit admitted")
	}
	if !l.Allow("b", now) {
		t.Error("other IP refused")
	}
	if !l.Allow("a", now.Add(time.Minute)) {
		t.Error("new window refused")
	}
}

func TestIPLimiterBoundsMemory(t *testing.T) {
	l := NewIPLimiter(1, time.Minute)
	now := time.Now()
	for i := range maxTrackedIPs {
		l.Allow(string(rune(i+1)), now)
	}
	l.Allow("fresh", now.Add(2*time.Minute))
	if len(l.hits) > 1 {
		t.Errorf("idle IPs kept: %d", len(l.hits))
	}
}

func TestIPLimiterRefusesNewIPsWhenFullOfLiveWindows(t *testing.T) {
	l := NewIPLimiter(5, time.Minute)
	now := time.Now()
	for i := range maxTrackedIPs {
		l.Allow(fmt.Sprint(i), now)
	}
	if l.Allow("new", now.Add(time.Second)) || len(l.hits) > maxTrackedIPs {
		t.Errorf("an untracked IP passed a full limiter: %d tracked", len(l.hits))
	}
	if !l.Allow("0", now.Add(time.Second)) {
		t.Error("a tracked IP within its limit was refused")
	}
}
