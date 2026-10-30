package location

import (
	"reflect"
	"testing"
)

// TestParseJapaneseAddresses covers HRMOS, whose every location is "<ward or city>,
// <prefecture>" in Japanese script and placed nowhere, and HERP's addresses, written
// prefecture first with no separator, often behind the 〒 postal mark.
func TestParseJapaneseAddresses(t *testing.T) {
	tests := []struct {
		location string
		want     []string
	}{
		{"港区, 東京都", []string{"jp"}},
		{"大阪市北区, 大阪府", []string{"jp"}},
		{"札幌市中央区, 北海道", []string{"jp"}},
		{"京都市下京区, 京都府", []string{"jp"}},
		{"横浜市西区, 神奈川県", []string{"jp"}},
		{"那覇市, 沖縄県", []string{"jp"}},
		{"東京都", []string{"jp"}},
		// An address opens with its prefecture.
		{"東京都渋谷区渋谷2-24-12　渋谷スクランブルスクエア 28F～36F", []string{"jp"}},
		{"北海道砂川市", []string{"jp"}},
		{"東京都あきる野市", []string{"jp"}},
		{"熊本県阿蘇郡西原村", []string{"jp"}},
		// A prefecture opening a business name is no address: a Hong Kong restaurant.
		{"北海道牧場餐廳, Hong Kong", []string{"hk"}},
		// A marker with no name before it names no municipality: 市集 is a market.
		{"北海道市集香港有限公司, Hong Kong", []string{"hk"}},
		// A space may part the prefecture from the municipality.
		{"東京都 渋谷区", []string{"jp"}},
		{"東京都　渋谷区", []string{"jp"}},
		// The postal mark is Japan's alone, wherever the address it opens sits in the token.
		{"本社 〒151-0053 東京都渋谷区代々木1-36-4 全理連ビル", []string{"jp"}},
		{"〒108-0023 東京都港区芝浦3丁目1－1", []string{"jp"}},
		// A ward or city alone names no prefecture, and the dictionary does not guess.
		{"大阪市北区", nil},
		{"渋谷区", nil},
		// A prefecture named mid-token is not an address that opens with it.
		{"■SHIRO 銀座三越店 東京都中央区", nil},
	}
	for _, tt := range tests {
		t.Run(tt.location, func(t *testing.T) {
			g := Parse(tt.location)
			got := g.Countries
			if len(got) == 0 {
				got = nil
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Parse(%q).Countries = %v, want %v", tt.location, got, tt.want)
			}
			if tt.want != nil && !reflect.DeepEqual(g.Regions, []string{"apac"}) {
				t.Errorf("Parse(%q).Regions = %v, want [apac]", tt.location, g.Regions)
			}
		})
	}
}

// TestJapanesePrefecturesAreComplete holds the list to Japan's 47 prefectures, each
// carrying its 都/道/府/県 suffix — the suffixed form is what makes a name unambiguous.
func TestJapanesePrefecturesAreComplete(t *testing.T) {
	if len(japanesePrefectures) != 47 {
		t.Fatalf("len(japanesePrefectures) = %d, want 47", len(japanesePrefectures))
	}
	seen := map[string]bool{}
	counts := map[string]int{}
	for _, p := range japanesePrefectures {
		if seen[p] {
			t.Errorf("duplicate prefecture %q", p)
		}
		seen[p] = true
		r := []rune(p)
		counts[string(r[len(r)-1])]++
	}
	want := map[string]int{"都": 1, "道": 1, "府": 2, "県": 43}
	if !reflect.DeepEqual(counts, want) {
		t.Errorf("suffix counts = %v, want %v", counts, want)
	}
}
