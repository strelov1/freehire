package viewlog

import (
	"fmt"
	"strings"
	"testing"
)

const human = "Mozilla/5.0 (human)"

// lineAt builds one combined-format access-log line with an explicit time_local.
func lineAt(ip, method, path string, status int, ua, ts string) string {
	return fmt.Sprintf(`%s - - [%s] "%s %s HTTP/2.0" %d 0 "-" "%s"`,
		ip, ts, method, path, status, ua)
}

// line builds a line on the default day (2026-07-21).
func line(ip, method, path string, status int, ua string) string {
	return lineAt(ip, method, path, status, ua, "21/Jul/2026:12:00:00 +0000")
}

func TestAggregate(t *testing.T) {
	t.Run("repeat visitor collapses to one", func(t *testing.T) {
		log := strings.Join([]string{
			line("1.1.1.1", "GET", "/jobs/acme", 200, human),
			line("1.1.1.1", "GET", "/jobs/acme", 200, human),
			line("1.1.1.1", "GET", "/jobs/acme", 200, human),
		}, "\n")
		got := aggregate(t, log)
		if got["2026-07-21"]["acme"] != 1 {
			t.Errorf("acme = %d, want 1", got["2026-07-21"]["acme"])
		}
	})

	t.Run("distinct visitors count separately", func(t *testing.T) {
		log := strings.Join([]string{
			line("1.1.1.1", "GET", "/jobs/acme", 200, human),
			line("2.2.2.2", "GET", "/jobs/acme", 200, human),
		}, "\n")
		got := aggregate(t, log)
		if got["2026-07-21"]["acme"] != 2 {
			t.Errorf("acme = %d, want 2", got["2026-07-21"]["acme"])
		}
	})

	t.Run("same visitor on two days counts once per day", func(t *testing.T) {
		log := strings.Join([]string{
			lineAt("1.1.1.1", "GET", "/jobs/acme", 200, human, "21/Jul/2026:23:00:00 +0000"),
			lineAt("1.1.1.1", "GET", "/jobs/acme", 200, human, "22/Jul/2026:01:00:00 +0000"),
		}, "\n")
		got := aggregate(t, log)
		if got["2026-07-21"]["acme"] != 1 || got["2026-07-22"]["acme"] != 1 {
			t.Errorf("got 21=%d 22=%d, want 1 and 1", got["2026-07-21"]["acme"], got["2026-07-22"]["acme"])
		}
	})

	t.Run("page and api visitors both count for the same slug", func(t *testing.T) {
		log := strings.Join([]string{
			line("1.1.1.1", "GET", "/jobs/acme", 200, human),
			line("2.2.2.2", "GET", "/api/v1/jobs/acme", 200, "curl/8"),
		}, "\n")
		got := aggregate(t, log)
		if got["2026-07-21"]["acme"] != 2 {
			t.Errorf("acme = %d, want 2", got["2026-07-21"]["acme"])
		}
	})

	t.Run("bot skipped on page but counted on api", func(t *testing.T) {
		log := strings.Join([]string{
			line("1.1.1.1", "GET", "/jobs/acme", 200, "Googlebot/2.1"),
			line("2.2.2.2", "GET", "/api/v1/jobs/acme", 200, "Googlebot/2.1"),
		}, "\n")
		got := aggregate(t, log)
		if got["2026-07-21"]["acme"] != 1 {
			t.Errorf("acme = %d, want 1 (bot page skipped, bot api counted)", got["2026-07-21"]["acme"])
		}
	})

	t.Run("unrelated and malformed lines ignored", func(t *testing.T) {
		log := strings.Join([]string{
			line("1.1.1.1", "GET", "/companies/acme", 200, human),
			"garbage line",
			line("1.1.1.1", "POST", "/jobs/acme", 200, human),
		}, "\n")
		got := aggregate(t, log)
		if len(got) != 0 {
			t.Errorf("got %v, want empty", got)
		}
	})
}

// aggregate runs Aggregate over a log string and fails the test on error.
func aggregate(t *testing.T, log string) map[string]map[string]int {
	t.Helper()
	got, err := Aggregate(strings.NewReader(log))
	if err != nil {
		t.Fatal(err)
	}
	return got
}
