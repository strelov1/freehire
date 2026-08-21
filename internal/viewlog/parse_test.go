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

	// The log format gained a trailing "$http_sec_purpose" so a speculative fetch
	// can be told apart from a real visit. Both formats have to parse: rotated
	// files written before the nginx change are still fed to the aggregator, and a
	// deploy where the parser lands first must not drop the day's counts.
	t.Run("reads sec-purpose from the extended format", func(t *testing.T) {
		line := `203.0.113.5 - - [21/Jul/2026:12:00:00 +0000] "GET /jobs/acme-engineer-123/__data.json HTTP/2.0" 200 1234 "-" "Mozilla/5.0" "prefetch"`
		rec, ok := ParseLine(line)
		if !ok {
			t.Fatalf("ParseLine ok = false, want true")
		}
		if rec.Purpose != "prefetch" {
			t.Errorf("Purpose = %q, want prefetch", rec.Purpose)
		}
		if rec.UserAgent != "Mozilla/5.0" {
			t.Errorf("UserAgent = %q, want Mozilla/5.0", rec.UserAgent)
		}
	})

	t.Run("a line in the old format still parses, with no purpose", func(t *testing.T) {
		line := `203.0.113.5 - - [21/Jul/2026:12:00:00 +0000] "GET /jobs/acme-engineer-123 HTTP/2.0" 200 1234 "-" "Mozilla/5.0"`
		rec, ok := ParseLine(line)
		if !ok {
			t.Fatalf("ParseLine ok = false, want true")
		}
		if rec.Purpose != "" {
			t.Errorf("Purpose = %q, want empty", rec.Purpose)
		}
	})

	// nginx writes "-" for a header the request did not carry, which means the
	// same thing as absent and must not read as a purpose.
	t.Run("treats nginx's dash placeholder as no purpose", func(t *testing.T) {
		line := `203.0.113.5 - - [21/Jul/2026:12:00:00 +0000] "GET /jobs/acme-engineer-123 HTTP/2.0" 200 1234 "-" "Mozilla/5.0" "-"`
		rec, ok := ParseLine(line)
		if !ok {
			t.Fatalf("ParseLine ok = false, want true")
		}
		if rec.Purpose != "" {
			t.Errorf("Purpose = %q, want empty", rec.Purpose)
		}
	})

	// This one guards a decision made OUTSIDE this repository. web-metrics.sh on host2
	// exports nginx response rates but no latency, and its comment gave the reason:
	// latency needs $request_time in log_format, and "internal/viewlog parses this log
	// — not worth breaking that for". The objection was reasonable and never checked.
	// It does not hold: combinedLine is anchored at ^ and not at $, so anything nginx
	// appends after the user-agent (and the optional Sec-Purpose) is simply not looked
	// at.
	//
	// Every field is asserted rather than just ok, because the failure this exists to
	// catch is not "the line was rejected" — it is a future edit anchoring the pattern
	// or reordering a group, which would leave ParseLine returning true while the job
	// slug came from the wrong capture and the view counts quietly moved.
	t.Run("ignores the timing fields appended for the latency metrics", func(t *testing.T) {
		const (
			ip   = "203.0.113.5"
			ua   = "Mozilla/5.0 (Macintosh)"
			path = "/jobs/acme-engineer-123"
		)
		base := `203.0.113.5 - - [21/Jul/2026:12:00:00 +0000] "GET ` + path + ` HTTP/2.0" 200 1234 "https://ref" "` + ua + `"`

		for name, line := range map[string]string{
			"with purpose":    base + ` "prefetch" 0.412 0.398`,
			"purpose as dash": base + ` "-" 0.412 0.398`,
			// nginx writes "-" for upstream_response_time on a request it served itself
			// (a 403 from the deny list, a file off disk), so the field is not always a
			// number and the parser must not care either way.
			"no upstream time": base + ` "-" 0.004 -`,
		} {
			rec, ok := ParseLine(line)
			if !ok {
				t.Fatalf("%s: ParseLine ok = false, want true", name)
			}
			if rec.IP != ip {
				t.Errorf("%s: IP = %q, want %q", name, rec.IP, ip)
			}
			if want := time.Date(2026, time.July, 21, 12, 0, 0, 0, time.UTC); !rec.Time.Equal(want) {
				t.Errorf("%s: Time = %v, want %v", name, rec.Time.UTC(), want)
			}
			if rec.Method != "GET" {
				t.Errorf("%s: Method = %q, want GET", name, rec.Method)
			}
			if rec.Path != path {
				t.Errorf("%s: Path = %q, want %q — a view would be credited to the wrong job", name, rec.Path, path)
			}
			if rec.Status != 200 {
				t.Errorf("%s: Status = %d, want 200", name, rec.Status)
			}
			if rec.UserAgent != ua {
				t.Errorf("%s: UserAgent = %q, want %q — bot filtering reads this", name, rec.UserAgent, ua)
			}
		}
	})
}
