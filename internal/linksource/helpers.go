package linksource

import (
	"fmt"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/sources"
)

// monetaryAmount is the schema.org MonetaryAmount shape JobPosting ld+json uses for
// baseSalary, shared by the adapters that fold a structured salary into the description.
type monetaryAmount struct {
	Currency string `json:"currency"`
	Value    struct {
		MinValue float64 `json:"minValue"`
		MaxValue float64 `json:"maxValue"`
		UnitText string  `json:"unitText"`
	} `json:"value"`
}

// salaryParagraph renders a structured salary range as a leading <p>, or "" when no amount
// is stated. sources.Job has no dedicated salary field, so adapters fold it into the
// description (sanitize the result — currency is third-party text) to keep it visible and
// available to enrichment.
func salaryParagraph(s monetaryAmount) string {
	min, max := s.Value.MinValue, s.Value.MaxValue
	if min <= 0 && max <= 0 {
		return ""
	}
	cur, unit := s.Currency, salaryUnit(s.Value.UnitText)
	switch {
	case min > 0 && max > 0:
		return fmt.Sprintf("<p>Salary: %.0f–%.0f %s%s</p>", min, max, cur, unit)
	case min > 0:
		return fmt.Sprintf("<p>Salary: from %.0f %s%s</p>", min, cur, unit)
	default:
		return fmt.Sprintf("<p>Salary: up to %.0f %s%s</p>", max, cur, unit)
	}
}

// isTelecommute reports whether a schema.org jobLocationType marks a fully-remote role.
func isTelecommute(jobLocationType string) bool {
	return strings.EqualFold(jobLocationType, "TELECOMMUTE")
}

// salaryUnit maps a schema.org UnitText to a "/period" suffix, or "" when absent/unknown.
func salaryUnit(u string) string {
	switch strings.ToUpper(u) {
	case "HOUR":
		return "/hour"
	case "DAY":
		return "/day"
	case "WEEK":
		return "/week"
	case "MONTH":
		return "/month"
	case "YEAR":
		return "/year"
	default:
		return ""
	}
}

// parseDate parses a date-only timestamp ("2006-01-02", as Habr's datePosted emits),
// returning nil for an empty or unparseable value (posted_at is nullable).
func parseDate(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return nil
	}
	return sources.NotFuture(&t)
}

// parseEpochMillis converts a Unix-millis timestamp into a posted_at, returning nil for a
// zero value. Lever dates postings with createdAt in epoch millis.
func parseEpochMillis(ms int64) *time.Time {
	if ms == 0 {
		return nil
	}
	t := time.UnixMilli(ms).UTC()
	return sources.NotFuture(&t)
}

// parseRFC3339 parses an RFC3339 timestamp in UTC, returning nil for an empty or
// unparseable value. RFC3339Nano accepts both fractional (Ashby's publishedAt) and plain
// (RemoteYeah's datePosted) second forms.
func parseRFC3339(s string) *time.Time {
	if s == "" {
		return nil
	}
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return nil
	}
	t = t.UTC()
	return sources.NotFuture(&t)
}

// humanizeBoard turns an ATS board slug into a display company name ("ruby-labs" → "Ruby
// Labs"), used when the platform's API carries no company name. Its slug matches a curated
// board-file company name's slug for the common case, so the companies table aligns.
func humanizeBoard(slug string) string {
	words := strings.FieldsFunc(slug, func(r rune) bool { return r == '-' || r == '_' })
	for i, w := range words {
		if w != "" {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}
