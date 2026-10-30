package location

import "strings"

// japanesePrefectures are Japan's 47 prefectures in Japanese script, each with its
// 都/道/府/県 suffix. HRMOS writes every location as "<ward or city>, <prefecture>" ("港区,
// 東京都", "大阪市北区, 大阪府") and HERP writes a postal address, which in Japanese opens
// with the prefecture and runs on with no separator ("東京都渋谷区渋谷2-24-12 …"), often
// behind the postal mark (see japanesePostalMark). Neither placed a posting anywhere:
// measured 2026-10-30, all ~43.8k open HRMOS postings and ~20k of HERP's had no country.
//
// The suffixed form is what makes the name unambiguous: 県 is the Japanese character
// (Chinese writes 县 or 縣), and 東京都/京都府/大阪府/北海道 name nothing else. The bare forms
// ("東京", "大阪") are absent. Wards and cities ("渋谷区", "大阪市北区") are not read — the
// city tables carry Latin names only, and a ward without its prefecture states nothing.
var japanesePrefectures = []string{
	"北海道",
	"青森県", "岩手県", "宮城県", "秋田県", "山形県", "福島県",
	"茨城県", "栃木県", "群馬県", "埼玉県", "千葉県", "東京都", "神奈川県",
	"新潟県", "富山県", "石川県", "福井県", "山梨県", "長野県", "岐阜県", "静岡県", "愛知県",
	"三重県", "滋賀県", "京都府", "大阪府", "兵庫県", "奈良県", "和歌山県",
	"鳥取県", "島根県", "岡山県", "広島県", "山口県",
	"徳島県", "香川県", "愛媛県", "高知県",
	"福岡県", "佐賀県", "長崎県", "熊本県", "大分県", "宮崎県", "鹿児島県", "沖縄県",
}

// japanesePostalMark (〒) marks a Japanese postal code and nothing else, so a token
// carrying it is a Japanese address wherever it sits ("本社 〒151-0053 東京都渋谷区…").
const japanesePostalMark = "〒"

// isJapaneseAddress reports whether tok is a prefecture, an address that opens with one,
// or an address behind the postal mark. A prefecture named later in a token with no
// postal mark ("■SHIRO 銀座三越店 東京都中央区") is not read: what precedes it is a shop or
// office name, and the line is left to say no more than it plainly states.
func isJapaneseAddress(tok string) bool {
	if strings.Contains(tok, japanesePostalMark) {
		return true
	}
	for _, p := range japanesePrefectures {
		if strings.HasPrefix(tok, p) {
			return true
		}
	}
	return false
}
