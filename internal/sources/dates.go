package sources

import "time"

// maxPostedAtSkew is how far past "now" a posted_at may sit before NotFuture treats it as
// bogus rather than clock skew or a timezone-ahead date-only value (a UTC+14 "today" parses
// to a UTC instant a few hours ahead). Comfortably larger than any real skew, far smaller
// than the month-scale errors a misread deadline/validThrough or wrong year produces.
const maxPostedAtSkew = 48 * time.Hour

// NotFuture drops a posted_at that lies meaningfully in the future: a posting cannot be
// published after now, so such a value is a source or parse artifact (a deadline misread as
// the publish date, a wrong year) that would otherwise sort the job to the very top of the
// "freshest first" browse. It returns nil — the job reads as undated rather than freshest —
// instead of clamping to now, so a bogus date can't masquerade as fresh. A nil or in-range
// time passes through unchanged. Every adapter's date parser funnels through it.
func NotFuture(t *time.Time) *time.Time {
	if t != nil && t.After(time.Now().Add(maxPostedAtSkew)) {
		return nil
	}
	return t
}

// parseLayout parses a platform timestamp with the given layout into a posted_at,
// returning nil on an empty or unparseable value (posted_at is nullable — a missing or
// malformed date is not an error).
func parseLayout(layout, s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(layout, s)
	if err != nil {
		return nil
	}
	return NotFuture(&t)
}

// parseRFC3339 parses an RFC3339 timestamp (the common ATS format). RFC3339Nano
// accepts both fractional and plain-second forms (Z and colon offsets), a strict
// superset of RFC3339. As a fallback it accepts the RFC822-style numeric offset
// WITHOUT a colon (e.g. iCIMS careers-home's "2026-06-30T15:42:00+0000"), which
// RFC3339 rejects — the fallback layout only matches numeric offsets, so a Z or
// colon-offset string that reached here (parse or NotFuture failure) still yields nil.
func parseRFC3339(s string) *time.Time {
	if t := parseLayout(time.RFC3339Nano, s); t != nil {
		return t
	}
	return parseLayout("2006-01-02T15:04:05.999999999-0700", s)
}

// parseRFC3339OrDate parses a JobPosting datePosted that a board may emit as either a full
// RFC3339 timestamp or a bare "2006-01-02" date, trying the timestamp first. Shared by the
// ld+json adapters whose datePosted format varies by tenant (briefhq, northstone).
func parseRFC3339OrDate(s string) *time.Time {
	if t := parseRFC3339(s); t != nil {
		return t
	}
	return parseDate(s)
}

// parseDate parses a date-only timestamp ("2006-01-02", as Workable emits).
func parseDate(s string) *time.Time { return parseLayout("2006-01-02", s) }

// ParseDate, ParseEpochMillis, and ParseRFC3339 are the exported forms used by
// sibling packages (internal/linksource) that resolve a single posting and need
// the same posted_at funnel — date-only, epoch-millis, and RFC3339 inputs all
// pass through NotFuture.
func ParseDate(s string) *time.Time        { return parseDate(s) }
func ParseEpochMillis(ms int64) *time.Time { return parseEpochMillis(ms) }
func ParseRFC3339(s string) *time.Time     { return parseRFC3339(s) }

// parseSpaceTime parses a space-separated, zone-named timestamp ("2006-01-02 15:04:05
// MST", as Recruitee emits). Recruitee emits UTC; an unrecognized zone abbreviation
// would be read as offset 0, acceptable for an approximate posted_at.
func parseSpaceTime(s string) *time.Time { return parseLayout("2006-01-02 15:04:05 MST", s) }

// parsePubDate parses an RSS <pubDate>, an RFC1123 timestamp with or without a numeric zone
// offset. Shared by the RSS-feed adapters (trakstar, earcu, likeit).
func parsePubDate(s string) *time.Time {
	if t := parseLayout(time.RFC1123Z, s); t != nil {
		return t
	}
	return parseLayout(time.RFC1123, s)
}

// parseEpochMillis converts a Unix-millisecond timestamp into a posted_at, returning
// nil for a zero value (treated as "no date").
func parseEpochMillis(ms int64) *time.Time {
	if ms == 0 {
		return nil
	}
	t := time.UnixMilli(ms).UTC()
	return NotFuture(&t)
}

// parseEpochSeconds converts a Unix-second timestamp into a posted_at, returning nil for
// a zero value (treated as "no date"). Gem dates postings with firstPublishedTsSec.
func parseEpochSeconds(s int64) *time.Time {
	if s == 0 {
		return nil
	}
	t := time.Unix(s, 0).UTC()
	return NotFuture(&t)
}
