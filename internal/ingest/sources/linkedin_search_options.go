package sources

import (
	"fmt"
	"strconv"
	"strings"
)

// LinkedInAnyKeyword is the Board sentinel of a keyword-less "firehose" search: the adapter
// omits the keywords parameter, so the search lists the whole location.
const LinkedInAnyKeyword = "*"

// linkedInSearchOptions reads optional catalogue budgets without widening a bad
// configuration into an unbounded search. Missing values retain the initial crawl.
// windowSeconds is the f_TPR age: jobage days, or jobage_hours when present.
func linkedInSearchOptions(tenants map[string]string) (pages, windowSeconds, easyApplyPages int, err error) {
	if pages, err = linkedInSearchOption(tenants, "pages", 1, 10, 100); err != nil {
		return 0, 0, 0, err
	}
	jobAgeDays, err := linkedInSearchOption(tenants, "jobage", 1, 7, 30)
	if err != nil {
		return 0, 0, 0, err
	}
	windowSeconds = jobAgeDays * 24 * 60 * 60
	// jobage_hours narrows the window below a day; when present it overrides jobage.
	if _, ok := tenants["jobage_hours"]; ok {
		hours, err := linkedInSearchOption(tenants, "jobage_hours", 1, 0, 720)
		if err != nil {
			return 0, 0, 0, err
		}
		windowSeconds = hours * 60 * 60
	}
	// easyapply_pages adds a second listing narrowed by LinkedIn's Easy Apply filter; 0 is off.
	if easyApplyPages, err = linkedInSearchOption(tenants, "easyapply_pages", 0, 0, 100); err != nil {
		return 0, 0, 0, err
	}
	return pages, windowSeconds, easyApplyPages, nil
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
