package sources

import (
	"slices"
	"testing"
)

// TestMarketCountries pins which aggregators scope the coverage gate to a national market, and
// to which country: a wrong or missing one lets a foreign employer of the same name discard the
// board's local postings (issue #4412).
func TestMarketCountries(t *testing.T) {
	reg := Taxonomy()
	for _, tc := range []struct {
		provider, board string
		want            []string
	}{
		{"mojedelo", "", []string{"si"}},
		{"optius", "", []string{"si"}},
		{"ess", "", []string{"si"}},
		{"hzz", "", []string{"hr"}},
		{"posaohr", "", []string{"hr"}},
		{"infostud", "", []string{"rs"}},
		{"nsz", "", []string{"rs"}},
		{"mojposao", "mojposao.hr", []string{"hr"}},
		{"mojposao", " MojPosao.BA ", []string{"ba"}},
		{"mojposao", "mojposao.si", nil},
		{"himalayas", "", nil},
		{"greenhouse", "acme", nil},
	} {
		if got := MarketCountries(reg, CompanyEntry{Provider: tc.provider, Board: tc.board}); !slices.Equal(got, tc.want) {
			t.Errorf("MarketCountries(%s, %q) = %v, want %v", tc.provider, tc.board, got, tc.want)
		}
	}
}
