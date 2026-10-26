package classify

import (
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/dict/vocab"
)

// Japanese titles the dictionary could not read at all until now. hrmos alone carried 35,458
// open postings with 731 in search, because a title the dictionary cannot categorise is
// dropped from the index by search.CategoryUnresolved and never enriched — is_tech stays
// NULL, EnqueuePendingJobs skips it, and nothing ever revisits the decision.
//
// Every title below is a real open posting. The glued ones (シニア+データサイエンティスト,
// 向け+Webアプリケーションエンジニア) resolve only because titleBoundary lets a CJK alias edge
// match without a separator.
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
		{"【シニアデータサイエンティスト】 （製薬業界向け）", "data_science"},
		{"GNW_WEBエンジニア", "backend"},
		{"フロントエンドから設計・AWSまで！金融機関向けWebアプリケーションエンジニア", "backend"},
		{"新規エンタメサービスのUI/UXデザイナー", "design"},
		{"【急募】大手法人向けルータ移行（EOL対応）ネットワーク設計構築エンジニア【11月～1月入社歓迎】", "network_engineering"},
		{"システムエンジニア", "software_engineering"},
		{"ITエンジニア／プログラマー（WEB・オープン・モバイル系）", "software_engineering"},
		{"社内SE／社内システム開発・運用", "software_engineering"},
		{"情報システム課社内SE候補", "software_engineering"},
		{"[スカラコミュニケーションズ]Javaエンジニア（国産ERP導入・開発）", "software_engineering"},
		{"組み込み系開発エンジニア", "embedded"},
		{"ネットワーク運用監視", "network_engineering"},
		{"SOCアナリスト｜セキュリティ運用（ログ・イベント分析、検知／評価等）★CTCグループ", "security"},
		{"プロダクトマネージャー（PdM）", "product"},
		{"【Salesforce関連】プロジェクトマネージャ―、コンサルタント（東京）【在宅勤務可】＜0048CX＞", "project_management"},
		{"プリセールスSE‐戦略アカウント担当（商社・流通・小売・製薬業界)", "solutions_engineering"},
		{"ソリューションアーキテクト（Solution Architect）", "architecture"},
		{"ヘルプデスク", "support"},
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.want {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.want)
		}
	}
}

// The catalogue is broader than IT, so a Japanese title outside it gets its own category
// rather than staying unfiled. Several of these were once this file's NEGATIVE cases —
// asserted to resolve nothing, because an alias that resolved them would have had to be
// a technical one. With a non-technical home for each, they are filed where they belong.
func TestJapaneseNonITTitlesResolveTheirOwnCategory(t *testing.T) {
	cases := []struct {
		title string
		want  string
	}{
		{"EC運営アシスタント（データ入力・事務）｜ELLE SHOP（アルバイト）", "administration"},
		{"営業事務（データ集計・業務自動化）【E40】", "administration"},
		{"《大阪》《調達》航空機向け機内エンターテインメントシステム 調達部門 電気電子部品購買担当者", "operations"},
		{"採用担当（京都府/京都市）※エンジニアリング事業本部", "recruiting"},
		{"イメージングデバイス・ディスプレイデバイスの半導体プロセス生産エンジニア", "industrial_engineering"},
		{"法人営業（エンタープライズ）", "sales"},
		{"インサイドセールス", "sales"},
		{"カスタマーサクセス", "customer_success"},
		{"【名古屋】治験コーディネーター（CRC）/経験者/CHI", "healthcare"},
		{"【正社員】薬剤師（チームリーダー・出荷判定・品質保証）", "healthcare"},
		{"介護（正社員）はぴね別府亀川（有料老人ホーム）", "healthcare"},
		{"保育士_パート", "education"},
		{"建築施工管理（将来の役員候補）", "skilled_trades"},
		{"ルート配送ドライバー", "logistics"},
		{"Anker Store ららぽーと富士見 販売スタッフ", "retail"},
		{"客室清掃【ふふ 京都／京都府】", "personal_services"},
		{"税務コンサルタント（法人）", "finance"},
		{"【業務委託】労務（コーポレート本部 人事部）", "hr"},
		// A domain word yields to the role a title names: a cook in a care home, a
		// salesperson selling to hotels.
		{"ガーデンテラス赤羽　介護施設調理師（正社員）", "hospitality"},
		{"【USEN-ALMEX】ホテル・医療機関・ゴルフ場 法人営業（静岡）", "sales"},
		// 営業所 is a branch office; the role is the service engineer.
		{"サービスエンジニア（厚木営業所）", "industrial_engineering"},
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.want {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.want)
		}
	}
}

// The reason the technical aliases are compounds. Every one of these is a real open hrmos
// posting that contains エンジニア, データ, 開発 or デザイナー and is not a technology job; a
// single-token alias would have claimed all of them.
func TestJapaneseNonITTitlesAreNotTechnical(t *testing.T) {
	for _, title := range []string{
		"クボタ／トラクタのキャビンと内外装、操作系の設計開発／グローバル技術研究所",      // tractor cabin design
		"＜クボタケミックス＞【大阪】インフラを支える樹脂管製品の製品開発",           // resin pipe products
		"■新商品開発（服飾雑貨・ライフスタイル雑貨・インテリア小物）",             // fashion accessories
		"リードデザイナー/デザインセンター",                          // industrial design
		"【FY26_10】反射防止フィルム 開発エンジニア(商品開発職：プロセス検討)/栃木", // film R&D
		"EC運営アシスタント（データ入力・事務）｜ELLE SHOP（アルバイト）",
		"採用担当（京都府/京都市）※エンジニアリング事業本部",
	} {
		if got := Parse(title).Category; slices.Contains(vocab.TechCategories, got) {
			t.Errorf("Parse(%q).Category = %q — a technical category for a job that is not one", title, got)
		}
	}
}

// The blind spans: a word that contains a role alias without naming the role.
func TestJapaneseBlindSpans(t *testing.T) {
	for _, title := range []string{
		"（夜間専任）ロボットシステムサポート／累計資金調達額596億円！",   // fundraising, not procurement
		"ラグジュアリーブランド評価者になろう ー 応募受付中 (関東エリア)", // accepting applications
		"《福井県(森田)》抵抗製品の販売管理業務",               // order administration
	} {
		if got := Parse(title).Category; got != "" {
			t.Errorf("Parse(%q).Category = %q, want no category", title, got)
		}
	}
}

func TestJapaneseSeniority(t *testing.T) {
	cases := []struct{ title, want string }{
		{"【シニアデータサイエンティスト】 （製薬業界向け）", "senior"},
		{"［A2D］LLM/VLMエンジニア（テックリード）", "lead"},
		{"セキュリティエンジニア（新卒）", "junior"},
		{"長期インターン", "intern"},
		// A welcome, not a grade.
		{"【相模原×特殊車両、エンジン＆ターボ事業】経理・財務職（第二新卒歓迎）", ""},
	}
	for _, c := range cases {
		if got := Parse(c.title).Seniority; got != c.want {
			t.Errorf("Parse(%q).Seniority = %q, want %q", c.title, got, c.want)
		}
	}
}

// titleBoundary relaxes only the CJK edge of an alias. A Latin edge keeps its word
// boundary, so "sreエンジニア" cannot match inside a longer Latin token.
func TestTitleBoundaryRelaxesOnlyCJKEdges(t *testing.T) {
	cases := []struct {
		title, alias string
		want         bool
	}{
		{"シニアデータサイエンティスト", "データサイエンティスト", true},
		{"データサイエンティスト候補", "データサイエンティスト", true},
		{"presreエンジニア", "sreエンジニア", false},
		{"【sreエンジニア】", "sreエンジニア", true},
		{"開発itサポート", "itサポート", true},
		{"leading role", "lead", false},
	}
	for _, c := range cases {
		if got := len(occurrences(c.title, c.alias)) > 0; got != c.want {
			t.Errorf("match %q in %q = %v, want %v", c.alias, c.title, got, c.want)
		}
	}
}

// A CJK alias matches as a substring, so a single character would match inside any longer
// word that happens to contain it. Every CJK alias must be at least two characters; the
// two-character ones (営業, 経理, 介護…) were each sampled against live titles before entry.
func TestCJKAliasesAreNeverSingleCharacters(t *testing.T) {
	tables := [][]aliasEntry{categoryTable, japaneseSeniorityTable}
	for _, table := range tables {
		for _, e := range table {
			runes := []rune(e.alias)
			if len(runes) < 2 && (isCJK(runes[0])) {
				t.Errorf("CJK alias %q is a single character", e.alias)
			}
		}
	}
}
