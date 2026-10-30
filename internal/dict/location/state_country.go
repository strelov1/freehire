package location

import "strings"

// StateCountry resolves the full name of a US state or Canadian province ("Ohio",
// "Saskatchewan") to its country code, or "" when the name is neither. It is for a caller that
// reads the name from a field naming a state and nothing else — a platform's state picklist —
// where "Georgia" is the US state: a free-text line cannot assume that, which is why
// subdivisionToCountry leaves the name out. Postal codes are refused, since a two-letter value
// collides with a country code and a picklist that spells its states out has no use for them.
func StateCountry(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "georgia" {
		return "us"
	}
	if len(n) <= 2 {
		return ""
	}
	return subdivisionToCountry[n]
}
