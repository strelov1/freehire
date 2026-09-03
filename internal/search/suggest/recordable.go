package suggest

import "strings"

// The bounds on what may be RECORDED as demand. They are deliberately generous — the
// longest thing anybody genuinely searches for here is a graded title with a
// qualifier, and `Title` has already cut the qualifier off by this point.
//
// "Senior Technical Program Manager" is 32 characters and four words; "Senior Software
// Engineer, Infrastructure, Infra Spanner" normalises to 24 and three. Sixty and eight
// leave room for a longer phrase than any of those without leaving room for a pasted
// paragraph.
const (
	maxRecordedRunes = 60
	maxRecordedWords = 8
)

// Recordable reports whether a normalised query is worth storing as demand.
//
// The demand table is public input written on every search, and one rule answers both
// hazards that creates, because both come from the same fact — what is worth recording
// is a search PHRASE, and a phrase is short:
//
//   - Privacy. `Title` normalises text; it does not judge it. Without this, a visitor
//     who pastes an email address, a phone number or a whole job description into the
//     box has it stored indefinitely, in a table whose only purpose is ranking
//     vocabulary. Nothing downstream needs that text, and the honest place to refuse
//     it is before the write.
//   - Growth. The table is keyed BY the phrase, so every distinct string is a row, and
//     the public rate limit is per caller — it bounds one client, not the aggregate,
//     and it fails open when the limiter itself errors. Unbounded length times
//     unbounded distinct values is the whole exposure.
//
// It judges the NORMALISED phrase, because that is what gets stored. Rejecting is
// silent and lossless: the search still runs, and a phrase nobody could type is a
// phrase no suggestion needed to rank by.
func Recordable(normalised string) bool {
	if normalised == "" {
		return false
	}
	if len([]rune(normalised)) > maxRecordedRunes {
		return false
	}
	if len(strings.Fields(normalised)) > maxRecordedWords {
		return false
	}
	// An `@` is the marker that separates a contact detail from a job title. No posting
	// title carries one, and `Title` keeps it (it is not a separator), so without this
	// an address typed into the box survives every bound above — it is short and it is
	// one word.
	return !strings.ContainsRune(normalised, '@')
}
