package sources

import (
	"fmt"
	"strconv"
	"strings"
)

// linkedInSearchOptions reads optional catalogue budgets without widening a bad
// configuration into an unbounded search. Missing values retain the initial crawl.
func linkedInSearchOptions(tenants map[string]string) (pages, jobAgeDays, easyApplyPages int, err error) {
	if pages, err = linkedInSearchOption(tenants, "pages", 1, 10, 100); err != nil {
		return 0, 0, 0, err
	}
	if jobAgeDays, err = linkedInSearchOption(tenants, "jobage", 1, 7, 30); err != nil {
		return 0, 0, 0, err
	}
	// easyapply_pages adds a second listing narrowed by LinkedIn's Easy Apply filter; 0 is off.
	if easyApplyPages, err = linkedInSearchOption(tenants, "easyapply_pages", 0, 0, 100); err != nil {
		return 0, 0, 0, err
	}
	return pages, jobAgeDays, easyApplyPages, nil
}

func linkedInSearchOption(tenants map[string]string, key string, minimum, fallback, maximum int) (int, error) {
	raw, exists := tenants[key]
	if !exists {
		return fallback, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < minimum || n > maximum || strings.HasPrefix(raw, "+") {
		return 0, fmt.Errorf("linkedin: %s must be a decimal integer from %d to %d", key, minimum, maximum)
	}
	return n, nil
}
