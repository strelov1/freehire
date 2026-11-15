package config

import "math"

// Suggest holds the two frequency floors that bound the search box's suggestion dictionary
// (cmd/build-suggestions).
type Suggest struct {
	// TitleFloor is the merged occurrences a normalised posting title must reach, applied after
	// normalisation (see internal/search/suggest.Build).
	TitleFloor int
	// CompanyFloor is the open postings an employer must carry to be offered; it keeps one-posting
	// slugs (often job titles in the employer column) out.
	CompanyFloor int
}

// LoadSuggest reads the dictionary's floors, both optional. The defaults come from measuring the
// catalogue: a floor in the low tens keeps the vocabulary people write and drops one-off spellings.
// They are env-tunable because re-running the builder rewrites the dictionary wholesale.
func LoadSuggest() Suggest {
	s := Suggest{
		TitleFloor:   envInt("SUGGEST_TITLE_FLOOR", 25),
		CompanyFloor: envInt("SUGGEST_COMPANY_FLOOR", 5),
	}
	// A floor of zero would admit every one-off spelling. Floor both at 1.
	//
	// The ceiling matters: the company floor is passed to Postgres as an int32, and narrowing a larger
	// value can wrap negative, admitting every company slug. A floor at the cap yields an empty
	// dictionary, which cmd/build-suggestions refuses to swap in, so an absurd value fails loudly.
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
