package sources

import (
	"fmt"
	"strconv"
	"strings"
)

// linkedInSearchOptions reads optional catalogue budgets without widening a bad
// configuration into an unbounded search. Missing values retain the initial crawl.
func linkedInSearchOptions(tenants map[string]string) (int, int, error) {
	pages, err := linkedInSearchOption(tenants, "pages", 10, 100)
	if err != nil {
		return 0, 0, err
	}
	days, err := linkedInSearchOption(tenants, "jobage", 7, 30)
	if err != nil {
		return 0, 0, err
	}
	return pages, days, nil
}

func linkedInSearchOption(tenants map[string]string, key string, fallback, maximum int) (int, error) {
	raw, exists := tenants[key]
	if !exists {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 1 || n > maximum || strings.HasPrefix(raw, "+") {
		return 0, fmt.Errorf("linkedin: %s must be a decimal integer from 1 to %d", key, maximum)
	}
	return n, nil
}
