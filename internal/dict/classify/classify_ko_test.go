package classify

import "testing"

// TestParse_Korean pins the Korean tables (korean.go, korean_general.go): live headlines from
// Saramin and JobKorea (2026-11-22), each filed where its English twin is. Measured on 2,419
// distinct headlines the English dictionary resolved a category for 1.9% of them; the tables
// resolve 59%, and what stays unresolved is mostly headlines that name no role at all.
func TestParse_Korean(t *testing.T) {
	cases := []struct{ title, category, seniority string }{
		// Technical disciplines.
		{"[알라딘] AI 데이터 엔지니어 (경력직)", "data_engineering", ""},
		{"[비알피커넥트] 데이터 사이언티스트", "data_science", ""},
		{"SK플래닛 AI 데이터 과학자 2기", "data_science", ""},
		{"[유닛블랙] 백엔드 개발자", "backend", ""},
		{"백엔드 개발자 채용(10년이상)", "backend", ""},
		{"서버 / 네트워크 운영 엔지니어 경력직 채용(주간상주-판교)", "network_engineering", ""},
		{"[NC 자회사] ㈜NC ITS 네트워크 엔지니어 채용", "network_engineering", ""},
		{"클라우드 엔지니어 신입 모집", "devops", "junior"},
		{"[알바천국본사] 정보보안 기술 엔지니어 채용", "security", ""},
		{"교보DTS 경력 채용공고 (보안운영, 보안관제, 모의해킹)", "security", ""},
		{"롯데알미늄㈜ 정보보호담당자 경력직 모집", "security", ""},
		{"태솔이엔지(주)에서 전자회로·임베디드 제어 엔지니어를 모집해요", "embedded", ""},
		{"[MDS테크] 임베디드 SW 기술지원 채용(경력)", "embedded", ""},
		{"HW개발자 채용(10년이상)", "hardware", ""},
		{"로봇/로보틱스 S/W 엔지니어 신입 모집", "software_engineering", "junior"},
		{"S/W 엔지니어 채용(주임,선임연구원)", "software_engineering", "senior"},
		{"[본사]응용SW개발(C++, C#) 경력무관", "software_engineering", ""},
		{"Wi-Fi S/W 개발(3년 이상)", "software_engineering", ""},
		{"[동아참메드] 연구기획팀장·의료기기 SW개발 경력채용", "software_engineering", "lead"},
		{"[중견 계열사] 의료영상 3D 어플리케이션 개발자", "software_engineering", ""},
		{"교보DTS 웹 개발자 채용", "software_engineering", ""},
		{"한국건강관리협회 정보화혁신본부 전산직(정보보호/경력) 채용", "security", ""},
		{"[화장품 ODM상장사] IT인프라 담당 (서버/네트워크/전산장비 등)", "system_administration", ""},
		{"[채용대행] 서울전선 본사 전산 개발 및 IT 경력사원 채용", "software_engineering", ""},
		// Beyond IT.
		{"스마트팩토리 기계설계", "engineering_design", ""},
		{"마트 직원 모집", "retail", ""},
		{"[뷰티브랜드] 국내 영업 담당", "sales", ""},
		{"(주)대현단자 납품영업직원 채용", "sales", ""},
		{"[CHAI CRE.W] 광고기획 AE 모집(경력 13년 이상)", "marketing", ""},
		{"컨텐츠 마케터 경력 채용", "marketing", ""},
		{"[나맛엠텍] 경리 경력직 채용", "finance", ""},
		{"[GST] 재무회계팀 경력직 인재 채용", "finance", ""},
		{"[아이디병원] 인사팀 HR 담당자 채용", "hr", ""},
		{"총무 담당 [경력]", "administration", ""},
		{"(주)와이케이컴퍼니 경영지원팀 채용", "administration", ""},
		{"뉴데일리미디어그룹 사무·총무보조 채용", "administration", ""},
		{"간호조무사 정규직 채용", "healthcare", ""},
		{"건강검진센터 간호사(고압산소테라피 및 수액) 채용 공고", "healthcare", ""},
		{"해피실버타운 요양보호사 모집", "healthcare", ""},
		{"사회복지사 계약직 채용 - 경력", "social_services", ""},
		{"잉글리시업투학원 영어강사 채용 공고", "education", ""},
		{"고등부 주말 수학선생님 모십니다.", "education", ""},
		{"[양주덕정] 서래임금님들밥 함께할 가족 모집 (주방/홀직원)", "hospitality", ""},
		{"[대한항공 C&D] 기내식 조리사 채용전환형 인턴 채용", "hospitality", "intern"},
		{"[부산] 미쉐린 1스타 르도헤 홀 채용", "", ""},
		{"트루동[TRVDON] 니치향수 현대목동 정규 스탭 채용(매장관리)", "retail", ""},
		{"[로사노페레티] 프리미엄 헤어 케어 브랜드 판매직 정규 스탭", "retail", ""},
		{"[셀리맥스] 글로벌 아마존 MD 인턴 채용", "", "intern"},
		{"[오전 부업 가능] 학교급식 배송기사 모집 / 1종 보통 필수", "logistics", ""},
		{"(주5일,경기도 광주) 지게차 및 창고 관리", "logistics", ""},
		{"[헤이미니/Salté Library] 물류팀 직원 채용", "logistics", ""},
		{"[글로벌기업/상여금180%]음료용 빨대 생산직 대규모채용", "manufacturing", ""},
		{"cnc,mct 오퍼레이터 오래하실분", "manufacturing", ""},
		{"헬러코리아 연구소 기구설계 신입/경력 엔지니어 충원 채용", "engineering_design", ""},
		{"지열공사현장 안전관리자 모집", "occupational_safety", ""},
		{"[잠실 롯데 어드벤처] 주간근무 남.여 보안사원 채용", "public_safety", ""},
		{"평택시흥고속도로 재난안전상황실 관제요원 채용", "public_safety", ""},
		{"[오까네]온라인 플랫폼 웹디자이너 채용", "design", ""},
		{"[진료실] 치과위생사 채용합니다 (구미 인동부부치과)", "healthcare", ""},
		{"[삼성/하이닉스 1차 협력사] 법무 담당 채용", "legal", ""},
		{"[배달의민족] 콜센터 상담원 모집", "support", ""},
		{"[부산/메리츠화재 본사TM센터] TM상담직 모집", "support", ""},
	}
	for _, c := range cases {
		got := Parse(c.title)
		if got.Category != c.category || got.Seniority != c.seniority {
			t.Errorf("Parse(%q) = {category %q, seniority %q}, want {%q, %q}", c.title, got.Category, got.Seniority, c.category, c.seniority)
		}
	}
}

func TestIsTech_Korean(t *testing.T) {
	for _, title := range []string{
		"[유닛블랙] 백엔드 개발자",
		"소프트웨어 개발자 채용",
		"정보보호 담당자",
		"[알라딘] AI 데이터 엔지니어 (경력직)",
		"임베디드 SW 기술지원",
		"서버 운영 엔지니어",
	} {
		if !IsTech(title) {
			t.Errorf("IsTech(%q) = false, want true", title)
		}
	}
	// Technical words that sit in a non-technical title are not a confident signal.
	for _, title := range []string{
		"전자광학 개발자 모집",
		"[삼성/하이닉스 1차 협력사] 사무/관리 정규직 채용",
		"[K-뉴딜] 어도비 AI 콘텐츠 마케팅 과정",
		"보안사원 채용",
	} {
		if IsTech(title) {
			t.Errorf("IsTech(%q) = true, want false", title)
		}
	}
}

// The grade words of the Korean corporate ladder, and the advertisement phrases that look like
// a grade and are not.
func TestParse_KoreanSeniority(t *testing.T) {
	cases := []struct{ title, seniority string }{
		{"경영지원 과장급 채용 (경력 5년 이상)", "senior"},
		{"[SB성보] 인사 팀장급 채용", "lead"},
		{"영업관리 대리 모집", "middle"},
		{"총무 주임 모집", "junior"},
		{"시니어 MD 채용", "senior"},
		{"주니어 디자이너 모집", "junior"},
		{"[신입] 반도체 IC FAE 채용", "junior"},
		{"2026년 신입 정기 채용", "junior"},
		{"디자이너 체험형 인턴 채용", "intern"},
		{"S/W 엔지니어 채용 (책임~수석)", "principal"},
		// Open to both, or to a new graduate among others: no grade.
		{"하이록코리아 수시 채용(신입/경력) 공고", ""},
		{"에이앤지테크놀로지 신입 및 경력 채용", ""},
		{"경력/신입 사원 모집", ""},
		{"신입·경력직 모집", ""},
		{"자동차부품 생산직 경력자 우대 · 신입가능", ""},
		// Words that contain a grade word without stating one.
		{"보험대리점 영업 모집", ""},
		{"보험대리점 영업 팀장 모집", "lead"},
		{"대리운전 기사 모집", ""},
		{"2027 하이테크과정 신입생 모집", ""},
	}
	for _, c := range cases {
		if got := Parse(c.title).Seniority; got != c.seniority {
			t.Errorf("Parse(%q).Seniority = %q, want %q", c.title, got, c.seniority)
		}
	}
}

// A headline that names no role resolves nothing, and the ambiguous words stay unresolved
// rather than guessed.
func TestKoreanTablesNeverGuess(t *testing.T) {
	for _, title := range []string{
		"2026년 하반기 각 부문별 신입/경력 채용",
		"개발자 채용(10년이상)",                  // a bare developer: optics, product or software
		"연구원 모집",                         // a researcher of any discipline
		"시스템 엔지니어(SE) 경력직 구인",            // the English "System Engineer" is not read either
		"[국비지원] 2027년 취업연계형 AI·SW 교육생모집", // a course, not a job
		"[K-뉴딜] 인텔 AI for X 실무 프로젝트 과정",
		"[인턴연계] 제조AI부트캠프- 생성형AI/빅데이터 실무데이터분석 4기",
		"공동구매 전문 MD 경력직",
		"신입사원 공개채용",
		"리더십 교육",
	} {
		if got := Parse(title).Category; got != "" {
			t.Errorf("Parse(%q).Category = %q, want none", title, got)
		}
	}
}

// Hangul is a boundary for a Latin alias, as kana and kanji are: Korean glues the two in one
// token, and a title in another language is unchanged.
func TestKoreanLeavesOtherLanguagesAlone(t *testing.T) {
	cases := []struct{ title, category string }{
		{"Java Developer", "software_engineering"},
		{"Back-end Engineer (서울)", "backend"},
		{"Software Engineer", "software_engineering"},
		{"Sales Manager", "sales"},
		{"Security Guard", "public_safety"},
		{"Data Engineer", "data_engineering"},
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.category {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.category)
		}
	}
}
