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
		assertCounts(t, aggregate(t, log), "2026-07-21", "acme", Counts{Total: 1, Page: 1})
	})

	t.Run("distinct visitors count separately", func(t *testing.T) {
		log := strings.Join([]string{
			line("1.1.1.1", "GET", "/jobs/acme", 200, human),
			line("2.2.2.2", "GET", "/jobs/acme", 200, human),
		}, "\n")
		assertCounts(t, aggregate(t, log), "2026-07-21", "acme", Counts{Total: 2, Page: 2})
	})

	t.Run("same visitor on two days counts once per day", func(t *testing.T) {
		log := strings.Join([]string{
			lineAt("1.1.1.1", "GET", "/jobs/acme", 200, human, "21/Jul/2026:23:00:00 +0000"),
			lineAt("1.1.1.1", "GET", "/jobs/acme", 200, human, "22/Jul/2026:01:00:00 +0000"),
		}, "\n")
		got := aggregate(t, log)
		assertCounts(t, got, "2026-07-21", "acme", Counts{Total: 1, Page: 1})
		assertCounts(t, got, "2026-07-22", "acme", Counts{Total: 1, Page: 1})
	})

	t.Run("page and api visitors both count for the same slug", func(t *testing.T) {
		log := strings.Join([]string{
			line("1.1.1.1", "GET", "/jobs/acme", 200, human),
			line("2.2.2.2", "GET", "/api/v1/jobs/acme", 200, "curl/8"),
		}, "\n")
		assertCounts(t, aggregate(t, log), "2026-07-21", "acme", Counts{Total: 2, Page: 1})
	})

	t.Run("bot skipped on page but counted on api", func(t *testing.T) {
		log := strings.Join([]string{
			line("1.1.1.1", "GET", "/jobs/acme", 200, "Googlebot/2.1"),
			line("2.2.2.2", "GET", "/api/v1/jobs/acme", 200, "Googlebot/2.1"),
		}, "\n")
		assertCounts(t, aggregate(t, log), "2026-07-21", "acme", Counts{Total: 1, Page: 0})
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

	// The two counters are deduplicated independently over the same visitor key, so a
	// visitor who does both is one visitor in each — NOT two in Total. Adding the signal
	// kind to a single shared key would inflate Total, and Total is what feeds
	// job_daily_views.uniques, a figure GET /api/v1/stats/catalog already publishes.
	t.Run("one visitor doing both counts once in each", func(t *testing.T) {
		log := strings.Join([]string{
			line("1.1.1.1", "GET", "/jobs/acme", 200, human),
			line("1.1.1.1", "GET", "/api/v1/jobs/acme", 200, human),
		}, "\n")
		assertCounts(t, aggregate(t, log), "2026-07-21", "acme", Counts{Total: 1, Page: 1})
	})

	// Order must not decide attribution. The same visitor, the same two signals, the
	// other way round: a dedup that let the first line seen win would report Page: 0.
	t.Run("api read before page open still counts as a page visitor", func(t *testing.T) {
		log := strings.Join([]string{
			line("1.1.1.1", "GET", "/api/v1/jobs/acme", 200, human),
			line("1.1.1.1", "GET", "/jobs/acme", 200, human),
		}, "\n")
		assertCounts(t, aggregate(t, log), "2026-07-21", "acme", Counts{Total: 1, Page: 1})
	})

	t.Run("api-only job has no page uniques", func(t *testing.T) {
		log := strings.Join([]string{
			line("1.1.1.1", "GET", "/api/v1/jobs/acme", 200, "curl/8"),
			line("2.2.2.2", "GET", "/api/v1/jobs/acme", 200, "curl/8"),
		}, "\n")
		assertCounts(t, aggregate(t, log), "2026-07-21", "acme", Counts{Total: 2, Page: 0})
	})

	// The SvelteKit client-side navigation is the same page view as a full SSR load,
	// and must land in Page as well as Total.
	t.Run("spa data request counts as a page open", func(t *testing.T) {
		log := strings.Join([]string{
			line("1.1.1.1", "GET", "/jobs/acme/__data.json", 200, human),
		}, "\n")
		assertCounts(t, aggregate(t, log), "2026-07-21", "acme", Counts{Total: 1, Page: 1})
	})

	// Page can never exceed Total: both dedup over the same visitor key, and every
	// page visitor is by definition a visitor.
	t.Run("page never exceeds total", func(t *testing.T) {
		log := strings.Join([]string{
			line("1.1.1.1", "GET", "/jobs/acme", 200, human),
			line("2.2.2.2", "GET", "/jobs/acme", 200, human),
			line("2.2.2.2", "GET", "/api/v1/jobs/acme", 200, human),
			line("3.3.3.3", "GET", "/api/v1/jobs/acme", 200, "curl/8"),
		}, "\n")
		got := aggregate(t, log)["2026-07-21"]["acme"]
		if got.Page > got.Total {
			t.Errorf("page %d exceeds total %d", got.Page, got.Total)
		}
		assertCounts(t, aggregate(t, log), "2026-07-21", "acme", Counts{Total: 3, Page: 2})
	})
}

// aggregate runs Aggregate over a log string and fails the test on error.
func aggregate(t *testing.T, log string) map[string]map[string]Counts {
	t.Helper()
	got, err := Aggregate(strings.NewReader(log))
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func assertCounts(t *testing.T, got map[string]map[string]Counts, day, slug string, want Counts) {
	t.Helper()
	if g := got[day][slug]; g != want {
		t.Errorf("%s/%s = %+v, want %+v", day, slug, g, want)
	}
}
