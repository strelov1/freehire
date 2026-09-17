package classify

import "testing"

// Japanese titles the dictionary could not read at all until now. hrmos alone carried 35,458
// open postings with 731 in search, because a title the dictionary cannot categorise is
// dropped from the index by search.CategoryUnresolved and never enriched — is_tech stays
// NULL, EnqueuePendingJobs skips it, and nothing ever revisits the decision.
//
// Every alias here is a COMPOUND, and that is the whole design. A bare エンジニア ("engineer"),
// データ ("data"), 開発 ("development") or デザイナー ("designer") appears throughout Japanese
// manufacturing, procurement and recruiting titles — the negative cases below are all real
// hrmos postings — so matching a single token would declare a tractor-cabin designer and a
// parts buyer technical. That is the trap a bare "analyst" already sprang once, on 77k
// postings.
func TestJapaneseITTitlesResolve(t *testing.T) {
	cases := []struct {
		title string
		want  string
	}{
		{"機械学習エンジニア (ML/NLP)", "ml_ai"},
		{"【CL】SREエンジニア（オブザーバビリティ）", "sre"},
		{"インフラエンジニア（ハードウェア担当）", "devops"},
		{"インフラエンジニア（①サーバー・クラウドエンジニア②ネットワークエンジニア）新卒採用", "devops"},
		{"【PKSHA Infinity】プラットフォームエンジニア", "devops"},
		{"データサイエンティスト", "data_science"},
		{"GNW_WEBエンジニア", "backend"},
		{"新規エンタメサービスのUI/UXデザイナー", "design"},
		{"【急募】大手法人向けルータ移行（EOL対応）ネットワーク設計構築エンジニア【11月～1月入社歓迎】", "devops"},
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.want {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.want)
		}
	}
}

// The negative half, and the reason the aliases are compounds. Every one of these is a real
// open hrmos posting that contains エンジニア, データ, 開発 or デザイナー and is not a technology
// job. A single-token alias would have claimed all of them.
func TestJapaneseNonITTitlesStayUnclassified(t *testing.T) {
	for _, title := range []string{
		"EC運営アシスタント（データ入力・事務）｜ELLE SHOP（アルバイト）",          // data ENTRY clerk
		"営業事務（データ集計・業務自動化）【E40】",                         // sales admin
		"《大阪》《調達》航空機向け機内エンターテインメントシステム 調達部門 電気電子部品購買担当者", // procurement
		"クボタ／トラクタのキャビンと内外装、操作系の設計開発／グローバル技術研究所",          // tractor cabin design
		"＜クボタケミックス＞【大阪】インフラを支える樹脂管製品の製品開発",               // resin pipe products
		"■新商品開発（服飾雑貨・ライフスタイル雑貨・インテリア小物）",                 // fashion accessories
		"採用担当（京都府/京都市）※エンジニアリング事業本部",                     // RECRUITER
		"イメージングデバイス・ディスプレイデバイスの半導体プロセス生産エンジニア",           // semiconductor production
		"リードデザイナー/デザインセンター",                              // industrial design
	} {
		if got := Parse(title).Category; got != "" {
			t.Errorf("Parse(%q).Category = %q, want no category — this is not a technology job", title, got)
		}
	}
}

// The limit these aliases run into, named rather than hidden.
//
// wordmatch requires a token boundary on each side, and isWordRune counts kana and kanji as
// letters — correct in general, but Japanese writes no separators, so a compound GLUED to its
// qualifier has no boundary to find. シニア+データサイエンティスト and 向け+Webアプリケーションエンジニア
// are both real hrmos titles that stay unclassified for that reason alone.
//
// Measured on a real sample, the aliases still resolve 9 of 11 IT titles, because Japanese
// postings lean on 【】（）／・ and those delimiters do provide boundaries. The remaining case
// needs a boundary rule that treats a script change as a break, or a substring match for CJK
// terms specifically — safe for a ten-character katakana compound in a way it is not for a
// one-letter Latin one, which is exactly what the boundary check exists to stop.
//
// This test asserts the CURRENT behaviour so the gap is visible and measurable. When the
// boundary rule learns about CJK, this test fails and moves back into the positive set.
func TestJapaneseGluedCompoundsAreAKnownGap(t *testing.T) {
	for _, title := range []string{
		"【シニアデータサイエンティスト】 （製薬業界向け）",
		"フロントエンドから設計・AWSまで！金融機関向けWebアプリケーションエンジニア",
	} {
		if got := Parse(title).Category; got != "" {
			t.Logf("Parse(%q).Category = %q — the CJK boundary gap has been closed; "+
				"move this title into TestJapaneseITTitlesResolve", title, got)
			t.Fail()
		}
	}
}
