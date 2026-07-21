// Package viewlog aggregates nginx access-log lines into per-job view counts. It
// runs off the request path: a scheduled worker (cmd/rollup-views) feeds it a
// day's log and it returns per-job unique views, deduplicated by hashed IP+UA so
// a visitor counts at most once per job per day. Two request shapes are counted —
// the SSR detail page GET /jobs/<slug> (bot-filtered) and the API read
// GET /api/v1/jobs/<slug> (not bot-filtered) — every other line is ignored.
package viewlog

import (
	"regexp"
	"strconv"
	"time"
)

// Record is the subset of an access-log line the aggregator needs.
type Record struct {
	IP        string
	Time      time.Time
	UserAgent string
	Method    string
	Path      string
	Status    int
}

// combinedLine matches the nginx `combined` log format:
//
//	$remote_addr - $remote_user [$time_local] "$request" $status $body_bytes_sent "$referer" "$user_agent"
//
// The request group requires METHOD PATH PROTO, so bad requests logged as "-"
// (or otherwise malformed) fail to match and are skipped by the caller.
var combinedLine = regexp.MustCompile(`^(\S+) \S+ \S+ \[([^\]]*)\] "([A-Z]+) (\S+) [^"]*" (\d{3}) \S+ "[^"]*" "([^"]*)"`)

// timeLocalLayout is nginx's $time_local, e.g. 21/Jul/2026:12:00:00 +0000.
const timeLocalLayout = "02/Jan/2006:15:04:05 -0700"

// ParseLine parses one nginx combined-format access-log line. It returns ok=false
// for any line that does not match the format (including bad requests or an
// unparseable timestamp/status), so the caller can skip it and continue.
func ParseLine(line string) (Record, bool) {
	m := combinedLine.FindStringSubmatch(line)
	if m == nil {
		return Record{}, false
	}
	ts, err := time.Parse(timeLocalLayout, m[2])
	if err != nil {
		return Record{}, false
	}
	status, err := strconv.Atoi(m[5])
	if err != nil {
		return Record{}, false
	}
	return Record{
		IP:        m[1],
		Time:      ts,
		Method:    m[3],
		Path:      m[4],
		Status:    status,
		UserAgent: m[6],
	}, true
}
