package linksource

import (
	"fmt"
	"strings"
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
