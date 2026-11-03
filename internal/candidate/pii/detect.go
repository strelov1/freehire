// Package pii detects personally-identifiable information in CV text and produces a
// reversible Redactor that masks it out of LLM prompts and restores it in user-facing
// output. Detection unions a high-precision regex floor (email, phone, URL, @handle) with
// name/address spans from a local model detector; see the add-cv-pii-masking change.
package pii

import "regexp"

// Span is a half-open [Start, End) byte range in the source text carrying one PII value.
type Span struct {
	Start int
	End   int
	Kind  string
}

// PII kinds. NAME and ADDRESS come from the model; the rest are regex-detectable.
const (
	KindName    = "NAME"
	KindEmail   = "EMAIL"
	KindPhone   = "PHONE"
	KindLink    = "LINK"
	KindAddress = "ADDRESS"
)

var (
	emailRe = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	// URLs: explicit scheme, known-TLD bare domains with a path, or the common profile hosts.
	urlRe = regexp.MustCompile(`https?://\S+|(?:www\.)?[A-Za-z0-9-]+\.(?:com|dev|app|io|me|net|org)/\S*|linkedin\.com/\S+|github\.com/\S+|t\.me/\S+`)
	// Handle: an @name not preceded by a word char (so it never fires inside an email).
	handleRe = regexp.MustCompile(`(^|[^\w])(@\w{3,})`)
	// Phone: a run of digits/space/parens/dashes, at least a few long.
	phoneRe = regexp.MustCompile(`\+?\d[\d()\- \t]{7,}\d`)
	// A bare YYYY-YYYY year range (with optional spaces around the dash) that the phone
	// regex would otherwise capture — a common CV employment range, not a phone number.
	yearRangeRe = regexp.MustCompile(`^\d{4}\s*-\s*\d{4}$`)
	// The same range written ISO-style, "2016-05 - 2024-06" or "2016-05-01 - 2024-06-30", which
	// phoneRe takes whole.
	isoRangeRe = regexp.MustCompile(`^\d{4}(?:-\d{1,2}){1,2}\s*-\s*\d{4}(?:-\d{1,2}){0,2}$`)
	// The middle of a month-precision range, as phoneRe cuts it out of "05/2016 - 06/2024":
	// the match is "2016 - 06", the "05/" before it and the "/2024" after it are not digits-
	// and-dashes, so they are left outside. dateTailRe is the "year - month" shape and
	// dateHeadRe the "month - year" one ("2016.05 - 2024.06" cuts as "05 - 2024").
	dateTailRe = regexp.MustCompile(`^\d{4}\s*-\s*\d{1,2}$`)
	dateHeadRe = regexp.MustCompile(`^\d{1,2}\s*-\s*\d{4}$`)
)

// isDateRangeFragment reports whether the phone-shaped match text[start:end] is really the
// seam of a "MM/YYYY - MM/YYYY" (or dotted) employment range. A real number is not flanked by
// a date separator and a digit, so the flanking is what tells them apart; the match alone
// ("2016 - 06") is indistinguishable from the tail of a phone number.
//
// Missing this is not a cosmetic leak: the masked year never reaches the model, which then
// has to invent it, and does — every role of one CV came back with the wrong years.
func isDateRangeFragment(text string, start, end int) bool {
	match := text[start:end]
	if dateTailRe.MatchString(match) && end+1 < len(text) &&
		(text[end] == '/' || text[end] == '.') && isDigit(text[end+1]) {
		return true
	}
	if dateHeadRe.MatchString(match) && start >= 2 &&
		(text[start-1] == '/' || text[start-1] == '.') && isDigit(text[start-2]) {
		return true
	}
	return false
}

func isDigit(b byte) bool { return b >= '0' && b <= '9' }

// regexSpans returns the high-precision regex-detectable PII spans (email, URL, @handle,
// phone). Phone spans that are a bare year range, or that overlap an already-detected
// email/URL/handle, are dropped so digits inside a link are never mis-read as a phone.
func regexSpans(text string) []Span {
	var spans []Span
	for _, m := range emailRe.FindAllStringIndex(text, -1) {
		spans = append(spans, Span{m[0], m[1], KindEmail})
	}
	for _, m := range urlRe.FindAllStringIndex(text, -1) {
		spans = append(spans, Span{m[0], m[1], KindLink})
	}
	for _, m := range handleRe.FindAllStringSubmatchIndex(text, -1) {
		spans = append(spans, Span{m[4], m[5], KindLink}) // group 2 = the @handle itself
	}
	for _, m := range phoneRe.FindAllStringIndex(text, -1) {
		if yearRangeRe.MatchString(text[m[0]:m[1]]) || isoRangeRe.MatchString(text[m[0]:m[1]]) || isDateRangeFragment(text, m[0], m[1]) || overlapsAny(spans, m[0], m[1]) {
			continue
		}
		spans = append(spans, Span{m[0], m[1], KindPhone})
	}
	return spans
}

// overlapsAny reports whether [start, end) intersects any existing span.
func overlapsAny(spans []Span, start, end int) bool {
	for _, s := range spans {
		if start < s.End && s.Start < end {
			return true
		}
	}
	return false
}
