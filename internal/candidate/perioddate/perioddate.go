// Package perioddate is the one shared representation of a CV/work-history period
// boundary — a candidate's start or end date on a role, project, or education entry.
//
// Real CVs give this at one of two precisions: a bare year ("2024") or a month and
// year ("October 2018", "2021-03"). Neither a SQL date column nor a Go time.Time can
// represent "year-only" without lying about a precision nobody stated, so PeriodDate is a
// plain (Year, Month) pair with Month == 0 meaning "year-only" — never a day.
//
// experience.Employment, resumeextract.Experience, and cv.ExperienceItem all embed this type.
// Parse lives here, not in any one of them, because the backfill, the jsonb self-healing
// decode, and any other consumer need the same best-effort reading of old free text.
package perioddate

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"unicode"
)

// PeriodDate is a candidate-facing period boundary: a year, and optionally a month (1-12;
// zero means the CV gave no month). A *PeriodDate of nil means the boundary is unset —
// distinct from a PeriodDate whose Year is zero, which Sanitize never produces.
//
// Tagged lowercase because this shape also reaches an LLM: resumeextract's request schema is
// derived from Structured by reflection (internal/platform/llmschema), which reads these tags;
// untagged fields would ask the model for {"Year":...} instead of {"year":...}.
type PeriodDate struct {
	Year  int `json:"year"`
	Month int `json:"month,omitempty"`
}

// minYear/maxYear bound what Sanitize accepts.
const (
	minYear = 1900
	maxYear = 2100
)

// Sanitize enforces persistence-safe bounds on d, coercing rather than erroring: it runs on
// untrusted LLM output and form values, which must degrade to something storable. A year
// outside [1900, 2100] discards the date (nil); a month outside [1, 12] is dropped, leaving
// a year-only date.
func Sanitize(d *PeriodDate) *PeriodDate {
	if d == nil {
		return nil
	}
	if d.Year < minYear || d.Year > maxYear {
		return nil
	}
	month := d.Month
	if month < 0 || month > 12 {
		month = 0
	}
	return &PeriodDate{Year: d.Year, Month: month}
}

// monthNames indexes 1-12 to Format's three-letter abbreviation; index 0 is unused.
var monthNames = [...]string{"", "Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

// Format renders d for display, e.g. "Mar 2021" or "2018" for a year-only date. A nil
// PeriodDate, or one with no valid year, formats as "". The Year <= 0 case matters beyond nil:
// UnmarshalJSON cannot make the *PeriodDate field nil for an unparseable legacy string
// (encoding/json has already allocated the pointee), so it leaves the zero PeriodDate{}.
func (d *PeriodDate) Format() string {
	if d == nil || d.Year <= 0 {
		return ""
	}
	if d.Month == 0 {
		return strconv.Itoa(d.Year)
	}
	return monthNames[d.Month] + " " + strconv.Itoa(d.Year)
}

// FormatEnd formats a period's end boundary, substituting "Present" when current is true; the
// caller passes end=nil for an ongoing entry. Exported for consumers that render start and end
// separately.
func FormatEnd(end *PeriodDate, current bool) string {
	if current {
		return "Present"
	}
	return end.Format()
}

// FormatRange joins a start/end pair for display: " – " between two present sides, just the one
// side when only it is present, "" when neither is.
func FormatRange(start, end *PeriodDate, current bool) string {
	a := start.Format()
	b := FormatEnd(end, current)
	if a != "" && b != "" {
		return a + " – " + b
	}
	return a + b
}

// IsPresentLabel reports whether s is a CV's way of spelling "this hasn't ended". The one
// definition of that vocabulary, so every caller inferring Current from a raw label agrees.
func IsPresentLabel(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "present", "current", "now", "ongoing", "today":
		return true
	default:
		return false
	}
}

// Parse best-effort reads a free-text CV date label into a PeriodDate. Supported shapes:
// "2024", "2023-09", "2023/09", "Jan 2018", "January 2018". Present/current labels and
// anything else unrecognised report ok=false — Parse never guesses a date for those.
func Parse(raw string) (d *PeriodDate, ok bool) {
	s := strings.TrimSpace(raw)
	if s == "" || IsPresentLabel(s) {
		return nil, false
	}
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.Join(strings.Fields(s), " ")

	if d, ok := parseYearMonth(s); ok {
		return d, true
	}
	if d, ok := parseMonthYear(s); ok {
		return d, true
	}
	if y, err := strconv.Atoi(s); err == nil && y >= minYear && y <= maxYear {
		return &PeriodDate{Year: y}, true
	}
	return nil, false
}

func parseYearMonth(s string) (*PeriodDate, bool) {
	parts := strings.Split(s, "-")
	if len(parts) != 2 {
		return nil, false
	}
	y, errY := strconv.Atoi(parts[0])
	m, errM := strconv.Atoi(parts[1])
	if errY != nil || errM != nil || y < minYear || y > maxYear || m < 1 || m > 12 {
		return nil, false
	}
	return &PeriodDate{Year: y, Month: m}, true
}

func parseMonthYear(s string) (*PeriodDate, bool) {
	parts := strings.Split(s, " ")
	if len(parts) != 2 {
		return nil, false
	}
	m, ok := monthNumber(parts[0])
	if !ok {
		return nil, false
	}
	y, err := strconv.Atoi(parts[1])
	if err != nil || y < minYear || y > maxYear {
		return nil, false
	}
	return &PeriodDate{Year: y, Month: m}, true
}

// monthByName maps every recognised English month spelling (full and abbreviated) to its
// number, for parseMonthYear.
var monthByName = map[string]int{
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

func monthNumber(name string) (int, bool) {
	n := strings.ToLower(strings.TrimFunc(name, func(r rune) bool {
		return !unicode.IsLetter(r)
	}))
	m, ok := monthByName[n]
	return m, ok
}

// dateAlias breaks the json.Marshal/Unmarshal recursion while keeping the struct-tag encoding.
type dateAlias PeriodDate

// MarshalJSON always emits the object shape — a value read from the legacy string
// shape (see UnmarshalJSON) is upgraded the moment it is next saved.
func (d PeriodDate) MarshalJSON() ([]byte, error) {
	return json.Marshal(dateAlias(d))
}

// UnmarshalJSON accepts three shapes: the {"year":..,"month":..} object; a legacy free-text
// JSON string (older jsonb rows, or a gateway ignoring the request schema); or a bare JSON
// number, since a model asked for {"year": N} routinely emits a bare N. A string or number that
// fails to parse decodes to the zero PeriodDate with no error: the jsonb stores are
// read-and-regenerated snapshots, so an unreadable value is dropped like an absent field.
func (d *PeriodDate) UnmarshalJSON(b []byte) error {
	if trimmed := bytes.TrimSpace(b); len(trimmed) > 0 && trimmed[0] != '"' && trimmed[0] != '{' {
		var y int
		if err := json.Unmarshal(b, &y); err == nil {
			if sanitized := Sanitize(&PeriodDate{Year: y}); sanitized != nil {
				*d = *sanitized
			}
			return nil
		}
	}
	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		if parsed, ok := Parse(s); ok {
			*d = *parsed
		}
		return nil
	}
	var a dateAlias
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	*d = PeriodDate(a)
	return nil
}
