package socialdigest

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/dict/vocab"
)

func remotePostings(n int) []Posting {
	out := make([]Posting, 0, n)
	for i := range n {
		p := posting(int64(i+1), fmt.Sprintf("co-%d", i), 50-i)
		p.Remote = true
		out = append(out, p)
	}
	return out
}

func TestThemeForRotatesByWeekday(t *testing.T) {
	want := map[string]string{
		"2026-09-07": ThemeMostViewed.Heading, // Monday
		"2026-09-08": themeRemote.Heading,
		"2026-09-09": themeEntryLevel.Heading,
		"2026-09-10": ThemeMostViewed.Heading,
		"2026-09-11": themeDataAI.Heading,
		"2026-09-12": ThemeMostViewed.Heading,
		"2026-09-13": ThemeMostViewed.Heading,
	}
	for d, heading := range want {
		if got := ThemeFor(day(d)).Heading; got != heading {
			t.Errorf("ThemeFor(%s) = %q, want %q", d, got, heading)
		}
	}
	// The hour must not move a day to another theme.
	late := time.Date(2026, 9, 8, 23, 59, 0, 0, time.UTC)
	if got := ThemeFor(late).Heading; got != themeRemote.Heading {
		t.Errorf("ThemeFor(late Tuesday) = %q", got)
	}
}

func TestSelectThemed(t *testing.T) {
	office := posting(100, "office", 500)

	t.Run("keeps only the theme's postings, still most viewed first", func(t *testing.T) {
		theme, items := SelectThemed(themeRemote, append([]Posting{office}, remotePostings(MinThemeItems)...), nil)
		if theme.Heading != themeRemote.Heading {
			t.Errorf("theme = %q", theme.Heading)
		}
		assertIDs(t, items, []int64{1, 2, 3, 4, 5})
	})

	t.Run("a thin theme publishes the plain list under the plain heading", func(t *testing.T) {
		theme, items := SelectThemed(themeRemote, append([]Posting{office}, remotePostings(MinThemeItems-1)...), nil)
		if theme.Heading != ThemeMostViewed.Heading {
			t.Errorf("theme = %q, want the plain heading on a list that is not all remote", theme.Heading)
		}
		assertIDs(t, items, []int64{100, 1, 2, 3, 4})
	})

	t.Run("the quarantine counts toward thinness", func(t *testing.T) {
		theme, _ := SelectThemed(themeRemote, remotePostings(MinThemeItems), map[int64]bool{1: true})
		if theme.Heading != ThemeMostViewed.Heading {
			t.Errorf("theme = %q, want the fallback once a quarantined posting drops it below the minimum", theme.Heading)
		}
	})

	t.Run("the editorial rules hold inside a theme", func(t *testing.T) {
		sameCompany := remotePostings(MinThemeItems + 2)
		for i := range sameCompany[:3] {
			sameCompany[i].CompanySlug = "acme"
		}
		_, items := SelectThemed(themeRemote, sameCompany, nil)
		acme := 0
		for _, p := range items {
			if p.CompanySlug == "acme" {
				acme++
			}
		}
		if acme != MaxPerCompany {
			t.Errorf("acme holds %d places, want %d", acme, MaxPerCompany)
		}
	})
}

func TestDataAICategoriesAreVocabulary(t *testing.T) {
	for _, c := range dataAICategories {
		if !slices.Contains(vocab.CategoryValues, c) {
			t.Errorf("%q is not in vocab.CategoryValues, so the theme would silently never match it", c)
		}
	}
}

func TestDigestTitle(t *testing.T) {
	d := Digest{Day: day("2026-09-08"), Theme: themeRemote}
	if got, want := d.Title(), "Most viewed remote jobs on freehire — 8 September 2026"; got != want {
		t.Errorf("Title = %q, want %q", got, want)
	}
}
