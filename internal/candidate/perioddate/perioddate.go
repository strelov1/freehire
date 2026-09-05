// Package perioddate is the one shared representation of a CV/work-history period
// boundary — a candidate's start or end date on a role, project, or education entry.
//
// Real CVs give this at one of two precisions: a bare year ("2024") or a month and
// year ("October 2018", "2021-03"). Neither a SQL date column nor a Go time.Time can
// represent "year-only" without lying about a precision nobody stated, so PeriodDate is a
// plain (Year, Month) pair with Month == 0 meaning "year-only" — never a day.
//
// experience.Employment, resumeextract.Experience, and cv.ExperienceItem all embed
// this type: it used to be three independent free-form strings sharing one documented
// convention (migration 0047_experience_bank.sql), which is also why Parse exists here
// rather than in any one of them — the backfill, the jsonb self-healing decode, and any
// future consumer all need the same best-effort reading of old free text.
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
// Tagged lowercase (matching every other field on the structs that embed it) because
// this shape also reaches an LLM: resumeextract's request schema is derived from
// Structured by reflection (internal/platform/llmschema), which reads these same
// struct tags — untagged fields would ask the model for {"Year":...,"Month":...}
// instead of {"year":...,"month":...}.
type PeriodDate struct {
	Year  int `json:"year"`
	Month int `json:"month,omitempty"`
}

// minYear/maxYear bound what Sanitize accepts, matching the range the free-text
// parser this replaces already enforced.
const (
	minYear = 1900
	maxYear = 2100
)

// Sanitize enforces persistence-safe bounds on d, coercing rather than erroring — this
// runs on untrusted LLM output as well as candidate-entered form values, and both must
// degrade to something storable instead of failing the whole write. A year outside
// [1900, 2100] discards the date entirely (nil): an implausible year is not usable
// evidence of when something happened, and there is nothing safe to fall back to
// within just the date value. A month outside [1, 12] is dropped, coercing to
// year-only instead of discarding the (valid) year alongside it.
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
// PeriodDate, or one with no valid year, formats as "" — callers joining a start/end pair
// (see cv/renderer.go's daterange-equivalent) already treat an empty side as absent.
// The Year <= 0 case matters beyond nil: UnmarshalJSON cannot make the *PeriodDate field
// itself nil when handed an unparseable legacy string (encoding/json has already
// allocated the pointee before calling it), so it leaves the zero PeriodDate{} instead —
// Format treats that the same as absent rather than printing "0".
func (d *PeriodDate) Format() string {
	if d == nil || d.Year <= 0 {
		return ""
	}
	if d.Month == 0 {
		return strconv.Itoa(d.Year)
	}
	return monthNames[d.Month] + " " + strconv.Itoa(d.Year)
}

// FormatEnd formats a period's end boundary, substituting "Present" when current is
// true instead of leaving it blank — the caller passes end=nil for an ongoing entry,
// matching Employment/ExperienceItem's own Current-means-no-end convention. Exported so
// a consumer that renders start and end separately (cv/renderer.go's per-field render
// projection) doesn't hand-roll the same substitution FormatRange applies internally.
func FormatEnd(end *PeriodDate, current bool) string {
	if current {
		return "Present"
	}
	return end.Format()
}

// FormatRange joins a start/end pair for display the same way every consumer that used
// to hold pre-formatted strings already joined them (cv's typst templates' daterange,
// the assistant tools' period line): " – " between two present sides, just the one side
// when only it is present, "" when neither is.
func FormatRange(start, end *PeriodDate, current bool) string {
	a := start.Format()
	b := FormatEnd(end, current)
	if a != "" && b != "" {
		return a + " – " + b
	}
	return a + b
}

// IsPresentLabel reports whether s is a CV's way of spelling "this hasn't ended" — the
// only place this vocabulary is defined, so every caller inferring Current from a raw
// label (import_resume.go, the jsonb legacy decode below) agrees on which spellings
// count.
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

// dateAlias breaks the recursion a PeriodDate method calling json.Marshal/Unmarshal(d) on
// itself would cause, while still going through the ordinary struct-tag-driven
// encoding (Year/Month's own `json:` tags) rather than a second, hand-written shape.
type dateAlias PeriodDate

// MarshalJSON always emits the object shape — a value read from the legacy string
// shape (see UnmarshalJSON) is upgraded the moment it is next saved.
func (d PeriodDate) MarshalJSON() ([]byte, error) {
	return json.Marshal(dateAlias(d))
}

// UnmarshalJSON accepts three shapes: the new {"year":..,"month":..} object; a legacy
// free-text JSON string (resume_structured/cv_documents rows written before this type
// existed, or an LLM gateway that stops honouring the request schema — see
// perioddate's package doc); or a bare JSON number, the same defensive tolerance
// resumeextract's old verbatimString shim gave free-text dates, kept here because a
// model asked for {"year": N} routinely emits a bare N instead. A string or number that
// fails to parse as a date decodes to the zero PeriodDate with no error: the jsonb stores
// this feeds are read-and-regenerated snapshots, not accumulating records, so a value
// nobody can make sense of is dropped the same way an absent field would be, not
// treated as a decode failure.
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
