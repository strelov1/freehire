// Package viewlog aggregates nginx access-log lines into per-job view counts. It
// runs off the request path: a scheduled worker (cmd/rollup-views) feeds it a
// day's log and it returns per-job unique views, deduplicated by the raw IP+UA
// tuple (NUL-joined, no hashing) so a visitor counts at most once per job per
// day. Two request shapes are counted — the SSR detail page GET /jobs/<slug>
// (bot-filtered) and the API read GET /api/v1/jobs/<slug> (not bot-filtered) —
// every other line is ignored, including any request the browser marked
// speculative with Sec-Purpose (see Classify).
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
	// Purpose is the request's Sec-Purpose header: non-empty when the browser
	// fetched this speculatively (`prefetch`, or `prefetch;prerender`) rather than
	// because someone opened the page. Empty for a real navigation, for a client
	// that does not send the header, and for every line written before the log
	// format carried it.
	Purpose string
	Status  int
}

// combinedLine matches the nginx `combined` log format:
//
//	$remote_addr - $remote_user [$time_local] "$request" $status $body_bytes_sent "$referer" "$user_agent"
//
// plus one OPTIONAL trailing quoted field, "$http_sec_purpose", which the freehire
// site config appends (see freehire-ops, provision/host2/nginx). Optional so rotated
// files written before that change still parse.
//
// The request group requires METHOD PATH PROTO, so bad requests logged as "-" fail to
// match and are skipped by the caller.
//
// NOT anchored at the end, on purpose: the live format appends $request_time and
// $upstream_response_time, which this parser ignores, as it does anything appended
// later. Adding a $ breaks that; TestParseLine/"ignores the timing fields appended
// for the latency metrics" guards it.
var combinedLine = regexp.MustCompile(`^(\S+) \S+ \S+ \[([^\]]*)\] "([A-Z]+) (\S+) [^"]*" (\d{3}) \S+ "[^"]*" "([^"]*)"(?: "([^"]*)")?`)

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
	// nginx writes "-" for a header the request did not carry, which means the same
	// thing as the field being absent — normalize both to empty so Classify has one
	// case to test rather than two.
	purpose := m[7]
	if purpose == "-" {
		purpose = ""
	}
	return Record{
		IP:        m[1],
		Time:      ts,
		Method:    m[3],
		Path:      m[4],
		Purpose:   purpose,
		Status:    status,
		UserAgent: m[6],
	}, true
}
