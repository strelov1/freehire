package viewlog

import (
	"testing"
	"time"
)

func TestParseLine(t *testing.T) {
	t.Run("page open line", func(t *testing.T) {
		line := `203.0.113.5 - - [21/Jul/2026:12:00:00 +0000] "GET /jobs/acme-engineer-123 HTTP/2.0" 200 1234 "https://ref" "Mozilla/5.0 (Macintosh)"`
		rec, ok := ParseLine(line)
		if !ok {
			t.Fatalf("ParseLine ok = false, want true")
		}
		if rec.IP != "203.0.113.5" {
			t.Errorf("IP = %q, want 203.0.113.5", rec.IP)
		}
		if want := time.Date(2026, time.July, 21, 12, 0, 0, 0, time.UTC); !rec.Time.Equal(want) {
			t.Errorf("Time = %v, want %v", rec.Time.UTC(), want)
		}
		if rec.Method != "GET" {
			t.Errorf("Method = %q, want GET", rec.Method)
		}
		if rec.Path != "/jobs/acme-engineer-123" {
			t.Errorf("Path = %q, want /jobs/acme-engineer-123", rec.Path)
		}
		if rec.Status != 200 {
			t.Errorf("Status = %d, want 200", rec.Status)
		}
		if rec.UserAgent != "Mozilla/5.0 (Macintosh)" {
			t.Errorf("UserAgent = %q, want Mozilla/5.0 (Macintosh)", rec.UserAgent)
		}
	})

	t.Run("api read line", func(t *testing.T) {
		line := `198.51.100.9 - - [21/Jul/2026:12:00:01 +0000] "GET /api/v1/jobs/acme-engineer-123 HTTP/1.1" 200 4096 "-" "curl/8.4.0"`
		rec, ok := ParseLine(line)
		if !ok {
			t.Fatalf("ParseLine ok = false, want true")
		}
		if rec.Path != "/api/v1/jobs/acme-engineer-123" {
			t.Errorf("Path = %q, want /api/v1/jobs/acme-engineer-123", rec.Path)
		}
		if rec.UserAgent != "curl/8.4.0" {
			t.Errorf("UserAgent = %q, want curl/8.4.0", rec.UserAgent)
		}
	})

	t.Run("malformed line is rejected", func(t *testing.T) {
		if _, ok := ParseLine(`not an access log line at all`); ok {
			t.Errorf("ParseLine ok = true for malformed line, want false")
		}
	})

	t.Run("bad request with dash request is rejected", func(t *testing.T) {
		line := `203.0.113.5 - - [21/Jul/2026:12:00:00 +0000] "-" 400 0 "-" "-"`
		if _, ok := ParseLine(line); ok {
			t.Errorf("ParseLine ok = true for dash request, want false")
		}
	})
}
