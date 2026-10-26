package classify

import (
	"strings"
	"testing"
	"unicode"
)

// Chinese titles, Simplified (mokahr, feishujobs) and Traditional (job104, cake and the
// Taiwanese employer boards). Every case is a real open posting on prod, 2026-10-26.
func TestChineseTitlesResolve(t *testing.T) {
	cases := []struct {
		title string
		want  string
	}{
		// Technical, both scripts.
		{"Java开发工程师", "software_engineering"},
		{"軟體工程師", "software_engineering"},
		{"资深后端开发工程师", "backend"},
		{"高雄軟體園區-PHP後端軟體工程師", "backend"},
		{"前端開發工程師", "frontend"},
		{"高级/资深Android开发工程师", "mobile"},
		{"全端工程師", "fullstack"},
		{"测试开发工程师", "qa"},
		{"算法工程师", "ml_ai"},
		{"演算法工程師【彰化】", "ml_ai"},
		{"AI应用工程师", "ai_engineering"},
		{"数据分析师", "data_analytics"},
		{"资深数据仓库工程师", "data_engineering"},
		{"資安工程師", "security"},
		{"網管工程師", "network_engineering"},
		{"高级系统运维工程师（容器方向）", "devops"},
		{"韌體工程師", "embedded"},
		{"BIOS工程師", "embedded"},
		{"數位IC設計工程師(台南) 8201", "hardware"},
		{"数字后端设计工程师", "hardware"},
		{"硬體研發工程師", "hardware"},
		{"產品經理", "product"},
		{"AI产品经理", "product"},
		{"專案經理", "project_management"},
		{"UI设计师", "design"},
		{"数值策划", "design"},
		{"资深3D角色", "creative"},

		// Engineering seats outside software.
		{"機械設計工程師", "engineering_design"},
		{"繪圖人員", "engineering_design"},
		{"機構工程師", "engineering_design"},
		{"品保工程師", "industrial_engineering"},
		{"工艺工程师", "industrial_engineering"},
		{"電控工程師", "industrial_engineering"},
		{"PLC程式設計工程師", "industrial_engineering"},
		{"工地主任", "skilled_trades"},
		{"職業安全衛生管理員", "occupational_safety"},
		{"安全员", "occupational_safety"},

		// The broader-than-IT catalogue.
		{"業務助理", "sales"},
		{"國外業務人員", "sales"},
		{"大模型销售经理", "sales"},
		{"医药代表 - 血液肿瘤 - 上海", "sales"},
		{"行銷企劃專員", "marketing"},
		{"客服專員", "support"},
		{"會計人員", "finance"},
		{"主辦會計", "finance"},
		{"法務專員", "legal"},
		{"人資專員", "hr"},
		{"招聘专员", "recruiting"},
		{"行政助理", "administration"},
		{"總經理特助", "administration"},
		{"採購專員", "operations"},
		{"生管人員", "operations"},
		{"運營類 | 倉管員-日班、中班、晚班 (鶯歌/八德)", "logistics"},
		{"倉管人員", "logistics"},
		{"大貨車司機", "logistics"},
		{"護理師", "healthcare"},
		{"照顧服務員", "healthcare"},
		{"临床监查员", "healthcare"},
		{"蛋白纯化研究员", "science"},
		{"外場服務人員", "hospitality"},
		{"房務員", "hospitality"},
		{"Guest Services Agent 宾客服务接待员", "hospitality"},
		{"門市人員", "retail"},
		{"日班保全員-台中二處〈梧棲區-萬海〉", "public_safety"},
		{"清潔人員", "personal_services"},
		{"安親課輔老師", "education"},
		{"教保員", "education"},
		{"社工員", "social_services"},
		{"水電學徒", "skilled_trades"},
		{"保全工（电梯）", "skilled_trades"},
		{"作業員", "manufacturing"},
		{"CNC銑床人員", "manufacturing"},
		{"品管人員", "manufacturing"},
		{"越南雙語翻譯人員(嘉義)", "technical_writing"},
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.want {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.want)
		}
	}
}

// The negative half: real postings whose words look like an alias and are not one, or
// that name no category at all. The bare heads every title ends in — 工程師, 人員, 助理,
// 開發 — must stay unresolved, or a Taiwanese R&D engineer of any discipline and a
// carmaker's trim developer would be filed as software.
func TestChineseTitlesStayUnresolved(t *testing.T) {
	for _, title := range []string{
		"研發工程師",
		"工程師",
		"助理工程師",
		"廠務助理",
		"儲備幹部",
		"对外院-软内饰开发岗",
		"紧固件开发岗",   // 固件 (firmware) inside 紧固件 (fastener)
		"食品衛生管理人員", // 生管 (production control) across 衛生 / 管理
		"育嬰留停代理人",  // parental-leave cover, not childcare
	} {
		if got := Parse(title).Category; got != "" {
			t.Errorf("Parse(%q).Category = %q, want unresolved", title, got)
		}
	}
}

// Every Chinese alias carries Han, so a Latin title never reaches the block, and a Latin
// edge of an alias still needs a word boundary (titleBoundary).
func TestChineseLatinEdges(t *testing.T) {
	cases := []struct {
		title string
		want  string
	}{
		{"鳳凰食號-中餐二級廚師/西廚(COMMIS II)", "hospitality"}, // not "mis"
		{"倉管高工(墨西哥) - PHPF24341", "logistics"},       // a requisition code, not PHP
		{"Programador CNC", ""},
		{"CAD Engineer - AI Trainer", ""},
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.want {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.want)
		}
	}
}

// Japanese shares the characters, and a Japanese title the Japanese blocks leave alone
// falls through to the Chinese one. These are real open Japanese postings (hrmos, herp,
// engage) whose kanji a Chinese reading gets wrong; each pairs the title with the
// misreading it must never come back as. What they SHOULD resolve to is the Japanese
// tables' business, so only the misreading is asserted.
func TestChineseTableLeavesJapaneseAlone(t *testing.T) {
	cases := []struct {
		title string
		never string
	}{
		{"EVeM Management Trainer（業務委託）", "sales"}, // 業務 is "duties" here
		{"航務業務", "sales"},
		{"設備保全（交替勤務）[姫路]", "public_safety"}, // plant maintenance, not a guard
		{"25007：プラント保全", "public_safety"},
		{"2G-103_契約社員｜【レジル電気保安(株)】電気主任技術者（関東）", "public_safety"},
		{"【栃木工場】製造（産機製造）", "skilled_trades"}, // 木工 across 栃木 / 工場
		{"07．ＮＣ自動機の加工品段取り替え・刃具交換スタッフ（本社工場）", "social_services"},
		{"【芝生管理スタッフ】エンジニアリングオープン求人", "operations"},
		{"プライベートバンカー（富裕層・法人資産の総合的な資産管理）", "hr"},
		{"【Biz】BizDev（行政事業 責任者候補）", "administration"}, // 行政 is government
		{"【BitStar Agent】美容メディアSuki-na｜メディア運用担当", "personal_services"},
		{"製品保守業務", "manufacturing"},
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got == c.never {
			t.Errorf("Parse(%q).Category = %q, the Chinese misreading", c.title, got)
		}
	}
}

func TestChineseSeniority(t *testing.T) {
	cases := []struct {
		title string
		want  string
	}{
		{"资深数据分析师", "senior"},
		{"資深韌體工程師", "senior"},
		{"高级前端开发工程师（数字化）", "senior"},
		{"首席科学家", "principal"},
		{"初/中级临床监查员（CRA/CRAII）-河南", "middle"},
		{"琢玉计划 - 数据开发实习生", "intern"},
		{"財务管培生", "junior"},
		{"高級飯店房務員", ""}, // Traditional 高級 is luxury as often as grade
	}
	for _, c := range cases {
		if got := Parse(c.title).Seniority; got != c.want {
			t.Errorf("Parse(%q).Seniority = %q, want %q", c.title, got, c.want)
		}
	}
}

// The rule that keeps the Chinese block away from every other language's titles: an
// alias with no Han in it would match a Latin title ("cnc" in "Programador CNC").
func TestChineseAliasesCarryHan(t *testing.T) {
	for _, table := range [][]aliasEntry{chineseCategoryTable, chineseSeniorityTable} {
		for _, e := range table {
			if !strings.ContainsFunc(e.alias, func(r rune) bool { return unicode.Is(unicode.Han, r) }) {
				t.Errorf("alias %q carries no Han character", e.alias)
			}
		}
	}
}
