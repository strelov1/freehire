package config

import "math"

// Suggest holds the two floors that bound the search box's suggestion dictionary
// (cmd/build-suggestions). Both are frequency cut-offs: they decide how much of the
// catalogue's long tail becomes offerable vocabulary.
type Suggest struct {
	// TitleFloor is the merged occurrences a normalised posting title must reach.
	// Applied AFTER normalisation, so the three spellings of "Product Owner" count
	// once — see internal/search/suggest.Build.
	TitleFloor int
	// CompanyFloor is the open postings an employer must carry to be offered. It is
	// what keeps the tail of one-posting slugs out of a dictionary meant to name real
	// employers; many of those slugs are job titles that landed in an employer column.
	CompanyFloor int
}

// LoadSuggest reads the dictionary's floors, both optional.
//
// The defaults come from measuring the live catalogue rather than from taste. Over a
// 2,500-title sample, 562 normalised titles are distinct and the 72 occurring three
// times or more cover 78.7% of the postings — so a floor in the low tens keeps the
// vocabulary people actually write and drops the one-off spellings, without the
// dictionary growing to a size the per-keystroke query would feel.
//
// They are deliberately env-tunable and deliberately NOT precise: the first real build
// is what sets them, and re-running the builder with a different floor is free — it
// rewrites the dictionary wholesale and swaps it in.
func LoadSuggest() Suggest {
	s := Suggest{
		TitleFloor:   envInt("SUGGEST_TITLE_FLOOR", 25),
		CompanyFloor: envInt("SUGGEST_COMPANY_FLOOR", 5),
	}
	// A floor of zero would admit every one-off spelling in the catalogue, which is a
	// dictionary of hundreds of thousands of rows that answer nothing. Floor both at 1
	// so a misconfiguration degrades to "everything measured at least once" rather
	// than to a division of the catalogue by nothing.
	//
	// The CEILING is not cosmetic. The company floor is handed to Postgres as an int32,
	// and on a 64-bit host `envInt` can return a value that does not fit — narrowing it
	// wraps, and a wrapped floor can come out NEGATIVE, which admits every company
	// slug in the catalogue. That is the opposite of what the operator asked for, and
	// it would look like the floor simply not working.
	//
	// A floor at the cap yields an empty dictionary, which cmd/build-suggestions
	// refuses to swap in — so an absurd value fails loudly instead of quietly
	// rewriting the box's vocabulary.
	s.TitleFloor = clamp(s.TitleFloor)
	s.CompanyFloor = clamp(s.CompanyFloor)
	return s
}

// maxSuggestFloor is the largest floor either knob may hold: the widest value that
// survives narrowing to the int32 Postgres takes.
const maxSuggestFloor = math.MaxInt32

func clamp(n int) int {
	if n < 1 {
		return 1
	}
	return min(n, maxSuggestFloor)
}
