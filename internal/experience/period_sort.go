package experience

import (
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/google/uuid"
)

// periodSortKey is a comparable YYYYMM (or 999999 for "current/present"). Zero means
// unparseable — those rows sort after dated ones when ordering reverse-chronologically.
type periodSortKey int

const (
	periodKeyUnknown periodSortKey = 0
	periodKeyCurrent periodSortKey = 999999
)

// sortKeyForEmployment derives a reverse-chrono sort key from free-form period labels.
// Prefer a concrete end date; treat Present/current as "now"; fall back to start.
func sortKeyForEmployment(start, end string, current bool) periodSortKey {
	if current || isPresentLabel(end) {
		return periodKeyCurrent
	}
	if k, ok := parsePeriodLabel(end); ok {
		return k
	}
	if k, ok := parsePeriodLabel(start); ok {
		return k
	}
	return periodKeyUnknown
}

// isPresentLabel reports whether s is a CV's way of spelling "this hasn't ended" — the
// only place this vocabulary is defined, so import_resume.go's Current derivation and this
// file's own period parsing can never drift apart on which spellings count.
func isPresentLabel(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "present", "current", "now", "ongoing", "today":
		return true
	default:
		return false
	}
}

// parsePeriodLabel best-effort parses CV date labels into YYYYMM.
// Supported: "2024", "2023-09", "2023/09", "Jan 2018", "January 2018", "Oct 2018".
func parsePeriodLabel(raw string) (periodSortKey, bool) {
	s := strings.TrimSpace(raw)
	if s == "" || isPresentLabel(s) {
		return 0, false
	}
	// Normalize separators and collapse spaces.
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.Join(strings.Fields(s), " ")

	if k, ok := parseYearMonth(s); ok {
		return k, true
	}
	if k, ok := parseMonthYear(s); ok {
		return k, true
	}
	if y, err := strconv.Atoi(s); err == nil && y >= 1900 && y <= 2100 {
		return periodSortKey(y*100 + 1), true // January of that year as a floor
	}
	return 0, false
}

func parseYearMonth(s string) (periodSortKey, bool) {
	// YYYY-MM or YYYY-M
	parts := strings.Split(s, "-")
	if len(parts) != 2 {
		return 0, false
	}
	y, errY := strconv.Atoi(parts[0])
	m, errM := strconv.Atoi(parts[1])
	if errY != nil || errM != nil || y < 1900 || y > 2100 || m < 1 || m > 12 {
		return 0, false
	}
	return periodSortKey(y*100 + m), true
}

func parseMonthYear(s string) (periodSortKey, bool) {
	parts := strings.Split(s, " ")
	if len(parts) != 2 {
		return 0, false
	}
	m, ok := monthNumber(parts[0])
	if !ok {
		return 0, false
	}
	y, err := strconv.Atoi(parts[1])
	if err != nil || y < 1900 || y > 2100 {
		return 0, false
	}
	return periodSortKey(y*100 + m), true
}

func monthNumber(name string) (int, bool) {
	n := strings.ToLower(strings.TrimFunc(name, func(r rune) bool {
		return !unicode.IsLetter(r)
	}))
	months := map[string]int{
		"january": 1, "jan": 1,
		"february": 2, "feb": 2,
		"march": 3, "mar": 3,
		"april": 4, "apr": 4,
		"may":  5,
		"june": 6, "jun": 6,
		"july": 7, "jul": 7,
		"august": 8, "aug": 8,
		"september": 9, "sep": 9, "sept": 9,
		"october": 10, "oct": 10,
		"november": 11, "nov": 11,
		"december": 12, "dec": 12,
	}
	m, ok := months[n]
	return m, ok
}

// sortEmploymentsChronological orders current roles first, then reverse-chronological by
// derived period keys, then by id for stability. Display labels are not rewritten.
func sortEmploymentsChronological(list []Employment) {
	if len(list) < 2 {
		return
	}
	type keyed struct {
		e   Employment
		key periodSortKey
		id  uuid.UUID
	}
	items := make([]keyed, len(list))
	for i, e := range list {
		items[i] = keyed{e: e, key: sortKeyForEmployment(e.Start, e.End, e.Current), id: e.ID}
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if a.e.Current != b.e.Current {
			return a.e.Current // current first
		}
		if a.key != b.key {
			return a.key > b.key // more recent first; unknown (0) last among non-current
		}
		// uuid string compare is enough for a stable deterministic tie-break in tests.
		return a.id.String() < b.id.String()
	})
	for i := range items {
		list[i] = items[i].e
	}
}
