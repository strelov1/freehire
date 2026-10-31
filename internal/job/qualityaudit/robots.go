package qualityaudit

import (
	"slices"
	"strconv"
	"strings"
	"time"
)

// agentToken is the product token robots.txt groups are matched against — the first word of
// liveness.UserAgent, which every request of the audit carries.
const agentToken = "freehire"

// robots is what one host's robots.txt asks of this agent (RFC 9309): its rules and its
// Crawl-delay, which is not in the RFC but is honoured anyway because it is the one way a host
// can tell an unknown crawler how fast it may go.
type robots struct {
	rules      []robotsRule
	crawlDelay time.Duration
	// denyAll is set when robots.txt could not be read for a reason the RFC treats as "assume
	// complete disallow" — a 5xx, or no answer at all.
	denyAll bool
}

type robotsRule struct {
	allow   bool
	pattern string
}

// robotsFromStatus applies RFC 9309's status rules: a 2xx is parsed, a 4xx means no rules, and
// anything else — a 5xx, a transport failure reported as 0 — means everything is disallowed,
// because the host may be down or shedding load and a crawler should stay away.
func robotsFromStatus(status int, body string) robots {
	switch {
	case status >= 200 && status < 300:
		return parseRobots(body)
	case status >= 400 && status < 500:
		return robots{}
	default:
		return robots{denyAll: true}
	}
}

// parseRobots reads the groups that apply to agentToken, or the "*" groups when none names it.
// Matching groups are merged, as the RFC requires.
func parseRobots(body string) robots {
	type group struct {
		agents []string
		rules  []robotsRule
		delay  time.Duration
	}
	var groups []*group
	var cur *group
	inAgents := false
	for line := range strings.SplitSeq(body, "\n") {
		if i := strings.IndexByte(line, '#'); i >= 0 {
			line = line[:i]
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		key = strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		switch key {
		case "user-agent":
			if !inAgents {
				cur = &group{}
				groups = append(groups, cur)
				inAgents = true
			}
			cur.agents = append(cur.agents, strings.ToLower(value))
		case "allow", "disallow":
			inAgents = false
			if cur == nil || value == "" {
				// An empty Disallow allows everything, which no rule already does.
				continue
			}
			cur.rules = append(cur.rules, robotsRule{allow: key == "allow", pattern: value})
		case "crawl-delay":
			inAgents = false
			if cur == nil {
				continue
			}
			if secs, err := strconv.ParseFloat(value, 64); err == nil && secs > 0 {
				cur.delay = time.Duration(secs * float64(time.Second))
			}
		}
	}

	pick := func(agent string) (robots, bool) {
		var r robots
		found := false
		for _, g := range groups {
			if slices.Contains(g.agents, agent) {
				found = true
				r.rules = append(r.rules, g.rules...)
				r.crawlDelay = max(r.crawlDelay, g.delay)
			}
		}
		return r, found
	}
	if r, ok := pick(agentToken); ok {
		return r
	}
	r, _ := pick("*")
	return r
}

// allowed reports whether path (with its query) may be fetched: the longest matching rule
// decides, and Allow wins a tie, per RFC 9309.
func (r robots) allowed(path string) bool {
	if r.denyAll {
		return false
	}
	best, allow := -1, true
	for _, rule := range r.rules {
		if !matchRobots(rule.pattern, path) {
			continue
		}
		n := len(rule.pattern)
		if n > best || (n == best && rule.allow) {
			best, allow = n, rule.allow
		}
	}
	return allow
}

// matchRobots matches a robots.txt path pattern: a prefix match, where "*" matches any run of
// characters and a trailing "$" anchors the end.
func matchRobots(pattern, path string) bool {
	anchored := strings.HasSuffix(pattern, "$")
	pattern = strings.TrimSuffix(pattern, "$")
	parts := strings.Split(pattern, "*")
	if !strings.HasPrefix(path, parts[0]) {
		return false
	}
	rest := path[len(parts[0]):]
	for i, part := range parts[1:] {
		last := i == len(parts)-2
		if last && anchored {
			return strings.HasSuffix(rest, part)
		}
		at := strings.Index(rest, part)
		if at < 0 {
			return false
		}
		rest = rest[at+len(part):]
	}
	return !anchored || rest == ""
}
