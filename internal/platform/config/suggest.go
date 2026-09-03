package config

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
	if s.TitleFloor < 1 {
		s.TitleFloor = 1
	}
	if s.CompanyFloor < 1 {
		s.CompanyFloor = 1
	}
	return s
}
