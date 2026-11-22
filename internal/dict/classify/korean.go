package classify

import (
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/dict/wordmatch"
)

// Korean titles. Saramin and JobKorea headline a posting the way the employer wrote the advertisement: "[알라딘] AI 데이터 엔지니어 (경력직)", "서버 / 네트워크 운영 엔지니어 경력직 채용". Measured 2026-11-22 on 2,419 distinct titles from both sites, the English dictionary resolved a category for 1.9% of them: it reads no Hangul.
//
// Hangul, like kana and kanji, writes particles and compounds without a separator ("소프트웨어개발자", "백엔드개발자"), so these aliases match as substrings, glued to whatever surrounds them (titleBoundary, cjk.go). That makes the choice of alias the whole safety margin, as it is for Japanese and Chinese:
//
//   - An alias names the ROLE or the discipline, never a head every title ends in: 담당자 ("person in charge"), 사원 ("employee"), 직원, 채용 ("hiring") and 모집 ("recruitment") resolve nothing alone.
//   - A bare 개발자 ("developer") resolves nothing, as the English "Developer" does: it is as often an optics or a product developer as a software one. It needs the software word beside it (웹 개발자, 소프트웨어 개발자).
//   - Saramin's headlines carry the company name, so an industry word that company names are made of (시스템, 건설, 식품, 제약, 바이오, 테크) is never an alias. A word that is also a place of work (사무소, 병원, 의원, 학원) is not one either.
//   - Every alias carries a Hangul character, so it cannot reach an English title: the core table reads those.
//
// A headline that lists several roles ("제조, 기구설계, 촉매개발, 해외영업") is filed by the first alias in table order, not by position in the title. A headline that names no role ("각 부문별 신입/경력 채용", "2026년 하반기 공개채용") stays unresolved, as it should: nothing in it says what the work is.
//
// Table order is the precedence: vetoes, then the technical disciplines, then the heads that outrank a field word (a seller of software is a seller), then the generic software words. koreanGeneralCategoryTable (korean_general.go) carries every other profession.

// koreanCategoryTable lists the Korean category aliases in precedence order: the technical block, then the other professions.
var koreanCategoryTable = append(append([]aliasEntry{}, koreanTechCategoryTable...), koreanGeneralCategoryTable...)

// Contract employee contains the pharmacist alias, but describes the contract only.
var koreanCategoryNoise = strings.NewReplacer("계약사원", " ")

var koreanTechCategoryTable = []aliasEntry{
	// Vetoes. A training course is not a job ("[국비지원] AI·SW 교육생모집", a bootcamp for 데이터 분석), and a bare 보안 is as often the guard of a mall as information security.
	{"교육생", categoryNone},
	{"부트캠프", categoryNone},
	{"캠퍼스", categoryNone},
	{"과정", categoryNone},
	{"공동구매", categoryNone},
	{"보안사원", "public_safety"},
	{"보안요원", "public_safety"},
	// Security, machine learning and AI before the fields they sit in.
	{"정보보호", "security"},
	{"정보보안", "security"},
	{"사이버보안", "security"},
	{"보안관제", "security"},
	{"보안 관제", "security"},
	{"모의해킹", "security"},
	{"침해사고", "security"},
	{"보안 엔지니어", "security"},
	{"보안엔지니어", "security"},
	{"머신러닝", "ml_ai"},
	{"딥러닝", "ml_ai"},
	{"ai 연구", "ml_ai"},
	{"ai연구", "ml_ai"},
	{"ai 엔지니어", "ai_engineering"},
	{"ai엔지니어", "ai_engineering"},
	{"ai 개발", "ai_engineering"},
	{"ai개발", "ai_engineering"},
	// Data.
	{"데이터 사이언티스트", "data_science"},
	{"데이터사이언티스트", "data_science"},
	{"데이터 과학자", "data_science"},
	{"데이터 엔지니어", "data_engineering"},
	{"데이터엔지니어", "data_engineering"},
	{"데이터 분석", "data_analytics"},
	{"데이터분석", "data_analytics"},
	{"데이터 애널리스트", "data_analytics"},
	{"bi 개발", "data_analytics"},
	// The software disciplines, ahead of the generic software words.
	{"프론트엔드", "frontend"},
	{"프런트엔드", "frontend"},
	{"프론트 엔드", "frontend"},
	{"백엔드", "backend"},
	{"백 엔드", "backend"},
	{"서버 개발", "backend"},
	{"서버개발", "backend"},
	{"풀스택", "fullstack"},
	{"풀 스택", "fullstack"},
	{"안드로이드", "mobile"},
	{"ios 개발", "mobile"},
	{"모바일 앱", "mobile"},
	{"앱 개발", "mobile"},
	{"앱개발", "mobile"},
	{"임베디드", "embedded"},
	{"펌웨어", "embedded"},
	{"회로 설계", "hardware"},
	{"회로설계", "hardware"},
	{"전자회로", "hardware"},
	{"hw 개발", "hardware"},
	{"hw개발", "hardware"},
	{"h/w 개발", "hardware"},
	{"하드웨어", "hardware"},
	{"블록체인", "blockchain"},
	{"테스터", "qa"},
	{"소프트웨어 테스트", "qa"},
	{"sw 테스트", "qa"},
	{"qa 담당", "qa"},
	{"qa 엔지니어", "qa"},
	{"qa엔지니어", "qa"},
	// Infrastructure: the network ahead of the server desk, as "서버 / 네트워크 운영" is read as the network's.
	{"네트워크 엔지니어", "network_engineering"},
	{"네트워크 운영", "network_engineering"},
	{"네트워크 관리", "network_engineering"},
	{"클라우드 엔지니어", "devops"},
	{"클라우드 운영", "devops"},
	{"클라우드 아키텍트", "architecture"},
	{"데브옵스", "devops"},
	{"인프라 엔지니어", "devops"},
	{"서버 운영", "system_administration"},
	{"시스템 운영", "system_administration"},
	{"시스템 관리", "system_administration"},
	{"서버 관리", "system_administration"},
	{"it 인프라", "system_administration"},
	{"it인프라", "system_administration"},
	{"인프라 운영", "system_administration"},
	// The IT department's desk: 전산 is the company's computing.
	{"전산직", "it_support"},
	{"전산 담당", "it_support"},
	{"전산담당", "it_support"},
	{"전산 운영", "it_support"},
	{"전산실", "it_support"},
	{"it 담당", "it_support"},
	{"it 지원", "it_support"},
	{"헬프데스크", "it_support"},
	// The heads that outrank a software word: 소프트웨어 영업 is a seller, a UX 기획 lead plans a product.
	{"영업", "sales"},
	{"프로덕트 매니저", "product"},
	{"프로덕트 오너", "product"},
	{"서비스 기획", "product"},
	{"웹 기획", "product"},
	{"앱 기획", "product"},
	{"프로젝트 매니저", "project_management"},
	{"프로젝트매니저", "project_management"},
	{"프로젝트 관리", "project_management"},
	{"솔루션 컨설턴트", "solutions_engineering"},
	// CNC programmers work on the production floor.
	{"cnc 프로그래머", "manufacturing"},
	{"cnc프로그래머", "manufacturing"},
	// The generic software words: development, the software engineer, the programmer.
	{"소프트웨어 개발", "software_engineering"},
	{"소프트웨어개발", "software_engineering"},
	{"소프트웨어 엔지니어", "software_engineering"},
	{"소프트웨어 설계", "software_engineering"},
	{"웹 개발", "software_engineering"},
	{"웹개발", "software_engineering"},
	{"웹 프로그래머", "software_engineering"},
	{"웹프로그래머", "software_engineering"},
	{"소프트웨어 프로그래머", "software_engineering"},
	{"소프트웨어프로그래머", "software_engineering"},
	{"전산 개발", "software_engineering"},
	{"전산개발", "software_engineering"},
	{"어플리케이션 개발", "software_engineering"},
	{"애플리케이션 개발", "software_engineering"},
	{"게임 개발", "software_engineering"},
	{"게임개발", "software_engineering"},
	{"sw 개발", "software_engineering"},
	{"sw개발", "software_engineering"},
	{"s/w 개발", "software_engineering"},
	{"s/w 엔지니어", "software_engineering"},
	{"s/w 설계", "software_engineering"},
	{"sw 엔지니어", "software_engineering"},
	{"응용sw", "software_engineering"},
	{"웹 개발자", "software_engineering"},
	{"소프트웨어 개발자", "software_engineering"},
	{"sw 개발자", "software_engineering"},
	{"앱 개발자", "mobile"},
	{"서버 개발자", "backend"},
	{"시스템 개발자", "software_engineering"},
}

// koreanTechTerms are the Korean titles that are confidently technical whatever category they resolve, the counterpart of techTitleTerms and held to the same rule: anchored to the discipline, never a bare 개발자 or 시스템.
var koreanTechTerms = []string{
	"소프트웨어 개발", "소프트웨어개발", "소프트웨어 엔지니어", "웹 개발", "웹개발",
	"웹 프로그래머", "웹프로그래머", "소프트웨어 프로그래머", "소프트웨어프로그래머",
	"게임 개발", "게임개발", "sw 개발", "sw개발", "s/w 개발", "s/w 엔지니어",
	"sw 엔지니어", "응용sw", "웹 개발자", "소프트웨어 개발자", "sw 개발자", "앱 개발자", "서버 개발자",
	"프론트엔드", "프런트엔드", "백엔드", "풀스택", "안드로이드", "앱 개발", "앱개발", "임베디드", "펌웨어",
	"정보보호", "정보보안", "사이버보안", "보안관제", "보안 관제", "모의해킹", "침해사고", "머신러닝",
	"딥러닝", "데이터 사이언티스트", "데이터사이언티스트", "데이터 엔지니어", "데이터엔지니어",
	"클라우드 엔지니어", "데브옵스", "네트워크 엔지니어", "서버 운영", "시스템 운영", "전산직", "전산 담당",
	"블록체인", "소프트웨어 테스트", "qa 엔지니어",
}

// koreanGradeBlind masks eligibility phrases and words that contain a grade without stating one.
// For example, 신입/경력 accepts applicants at different levels and 대리점 means an agency.
var koreanGradeBlind = regexp.MustCompile(`신입\s*[/·,.&+~\s]*(?:및|또는|혹은)?\s*경력|경력\s*[/·,.&+~\s]*(?:및|또는|혹은)?\s*신입|신입\s*(?:도\s*)?(?:가능|환영|무관|경단)|대리점|대리운전|대리인|신입생`)

// koreanSeniorityTable lists the grade words of a Korean title, highest first. The corporate ladder (사원, 주임, 대리, 과장, 차장, 부장) is mapped where a rung says a level: 주임 is the first rung above a clerk, 대리 the mid-level, 과장 and 차장 the senior ones, 팀장 and 부장 the leads. 사원 ("employee") and 책임 ("in charge") state nothing and are not here.
var koreanSeniorityTable = []aliasEntry{
	{"수석", "principal"},
	{"팀장", "lead"},
	{"파트장", "lead"},
	{"그룹장", "lead"},
	{"팀리더", "lead"},
	{"팀 리더", "lead"},
	{"부장", "lead"},
	{"시니어", "senior"},
	{"선임", "senior"},
	{"차장", "senior"},
	{"과장", "senior"},
	{"대리", "middle"},
	{"주니어", "junior"},
	{"주임", "junior"},
	{"인턴", "intern"},
	{"신입", "junior"},
}

// matchKoreanSeniority resolves the grade of a lowercased title the other tables left unresolved.
func matchKoreanSeniority(lower string) string {
	return matchOrdered(koreanGradeBlind.ReplaceAllString(lower, " "), koreanSeniorityTable)
}

// isKoreanTech reports whether a lowercased title states a confidently technical role in Korean.
func isKoreanTech(lower string) bool {
	for _, term := range koreanTechTerms {
		if wordmatch.Contains(lower, term, titleBoundary) {
			return true
		}
	}
	return false
}
