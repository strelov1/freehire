package socialdigest

import (
	"slices"
	"time"
)

// Theme is one of the tops the daily post rotates through. Every theme ranks on page_uniques
// under the same rules and only narrows which postings compete, so a heading can always say
// "most viewed".
type Theme struct {
	// Heading starts the post's title and must stay true of every item the list can hold.
	Heading string
	// Keep decides whether a posting competes. Nil keeps everything.
	Keep func(Posting) bool
}

// The tops. Each reads one column the candidate query already returns. Salary is absent on
// purpose: it has four producers that only the job view folds together.
var (
	ThemeMostViewed = Theme{Heading: "Most viewed"}
	themeRemote     = Theme{
		Heading: "Most viewed remote jobs",
		Keep:    func(p Posting) bool { return p.Remote },
	}
	themeEntryLevel = Theme{
		Heading: "Most viewed entry-level jobs",
		Keep:    func(p Posting) bool { return p.Seniority == "intern" || p.Seniority == "junior" },
	}
	themeDataAI = Theme{
		Heading: "Most viewed data and AI jobs",
		Keep:    func(p Posting) bool { return slices.Contains(dataAICategories, p.Category) },
	}
)

var dataAICategories = []string{"data_engineering", "data_science", "data_analytics", "ml_ai", "ai_engineering"}

// ThemeFor names the top a digest day runs. It depends on the day alone, so every channel,
// retry and -day replay agrees and the ledger needs no new column.
func ThemeFor(day time.Time) Theme {
	switch truncateDay(day).Weekday() {
	case time.Tuesday:
		return themeRemote
	case time.Wednesday:
		return themeEntryLevel
	case time.Friday:
		return themeDataAI
	default:
		return ThemeMostViewed
	}
}

// SelectThemed builds the day's list for its theme, falling back to the plain list when the
// theme cannot fill MinThemeItems. It returns the theme actually used, since the heading must
// describe the list that goes out.
func SelectThemed(theme Theme, candidates []Posting, quarantined map[int64]bool) (Theme, []Posting) {
	if theme.Keep != nil {
		kept := make([]Posting, 0, len(candidates))
		for _, p := range candidates {
			if theme.Keep(p) {
				kept = append(kept, p)
			}
		}
		if items := Select(kept, quarantined); len(items) >= MinThemeItems {
			return theme, items
		}
	}
	return ThemeMostViewed, Select(candidates, quarantined)
}
