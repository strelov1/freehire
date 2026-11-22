package classify

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Vietnamese titles. VietnamWorks titles its postings in Vietnamese, in English, or in both at once ("Kỹ sư Dữ liệu (Data Engineer)", "Chuyên Viên Quản Trị Hệ Thống"). Measured 2026-11-22 on 7,551 distinct titles, the English dictionary resolved a category for 20% of the 4,473 that carry Vietnamese letters, almost always through the English half: "Chuyên Viên Quản Trị Hệ Thống" and "Nhân Viên Kế Toán" named it nothing.
//
// Vietnamese is Latin script with spaces between its syllables, so the table matches PHRASES of words on the folded title, exactly as the Turkish table does (turkish.go): matchStems and splitWords are shared. foldVietnamese drops every tone mark and diacritic and writes đ as d, so every alias is spelled plain ("quan tri he thong", "ke toan"), and a title typed without its marks reads the same. The fold merges words that differ only by a mark (ban sell, ban board, ban table), so an alias is a PHRASE whenever the single word would be ambiguous: "ban hang" is selling, a bare "ban" is nothing.
//
// The table reads only a title that looksVietnamese: a letter no other Latin-script language in the dictionary writes (ơ, ư, and the tone-marked vowels ạ to ỹ), or a Vietnamese role phrase ("nhan vien", "chuyen vien", "ky su"). An English title carries neither, so an alias like "it" or "cntt" cannot reach it.
//
// Each answer follows the English twin of the same title. Where the twin is doubtful the title stays unresolved: "Giám Đốc" and "Trưởng Phòng" (Director, Manager) state no grade and no craft, "Kỹ Sư Hệ Thống" (System Engineer) and "Kiến Trúc Sư" (the building and the software architect) resolve nothing, nor does a bare "Chuyên Viên" or "Nhân Viên" (specialist, staff). A language requirement ("tiếng Nhật", "tiếng Trung") is never a role. A title listing several roles is filed by the first alias in table order.
//
// Table order is the precedence: the vetoes and the lecturer, the technical disciplines, the heads that outrank a field word (a seller of software is a seller), then the generic software words. vietnameseGeneralCategoryTable (vietnamese_general.go) carries every other profession.

// vietnameseCategoryTable lists the Vietnamese category aliases on the folded title, in precedence order.
var vietnameseCategoryTable = append(append([]aliasEntry{}, vietnameseTechCategoryTable...), vietnameseGeneralCategoryTable...)

var vietnameseTechCategoryTable = []aliasEntry{
	// The lecturer teaches the subject: "Giảng Viên Công Nghệ Thông Tin" is a teacher.
	{"giang vien", "education"},
	{"giao vien", "education"},
	// Security, machine learning and AI before the fields they sit in.
	{"an ninh thong tin", "security"},
	{"an ninh mang", "security"},
	{"an ninh cong nghe thong tin", "security"},
	{"an toan thong tin", "security"},
	{"attt", "security"},
	{"bao mat", "security"},
	{"kiem thu xam nhap", "security"},
	{"hoc may", "ml_ai"},
	{"ky su tri tue nhan tao", "ai_engineering"},
	// Data and business analysis.
	{"khoa hoc du lieu", "data_science"},
	{"ky su du lieu", "data_engineering"},
	{"kho du lieu", "data_engineering"},
	{"phan tich du lieu", "data_analytics"},
	{"quan tri co so du lieu", "devops"},
	{"phan tich nghiep vu", "business_analysis"},
	{"phan tich kinh doanh", "business_analysis"},
	// The software disciplines and the testing desk.
	{"ung dung di dong", "mobile"},
	{"lap trinh di dong", "mobile"},
	{"phan mem nhung", "embedded"},
	{"lap trinh nhung", "embedded"},
	{"vi dieu khien", "embedded"},
	{"kiem thu phan mem", "qa"},
	{"kiem thu", "qa"},
	{"kien truc su phan mem", "architecture"},
	{"thiet ke vi mach", "hardware"},
	{"thiet ke mach", "hardware"},
	{"ky su dien tu dong", "industrial_engineering"}, // automation, not electronics
	{"ky su dien tu", "hardware"},
	{"ky su vien thong", "telecommunications_engineering"},
	{"ha tang vien thong", "telecommunications_engineering"},
	// Infrastructure: the network ahead of the system desk.
	{"quan tri mang", "network_engineering"},
	{"quan tri he thong", "system_administration"},
	{"quan tri vien he thong", "system_administration"},
	{"quan tri ha tang", "system_administration"},
	{"quan ly he thong", "system_administration"},
	{"he thong may chu", "system_administration"},
	{"van hanh he thong", "system_administration"},
	{"quan ly du an ha tang", "project_management"},
	{"ha tang cntt", "system_administration"},
	{"ha tang cong nghe thong tin", "system_administration"},
	{"dien toan dam may", "devops"},
	{"ho tro ky thuat cntt", "it_support"},
	{"ho tro ky thuat it", "it_support"},
	{"ho tro cntt", "it_support"},
	{"presale", "solutions_engineering"},
	// The IT department's desk.
	{"nhan vien it", "it_support"},
	{"chuyen vien it", "it_support"},
	{"nhan vien cntt", "it_support"},
	{"chuyen vien cntt", "it_support"},
	{"nhan vien cong nghe thong tin", "it_support"},
	{"chuyen vien cong nghe thong tin", "it_support"},
	// The IT manager's English twin names no craft, and neither does this.
	{"truong phong cntt", categoryNone},
	{"truong phong cong nghe thong tin", categoryNone},
	{"truong nhom cong nghe thong tin", categoryNone},
	{"ky su cong nghe thong tin", "software_engineering"},
	{"ky su cntt", "software_engineering"},
	{"ky su it", "software_engineering"},
	// The heads that outrank a software word: selling software is selling, running a software project is project management.
	{"chien luoc kinh doanh", categoryNone},
	{"kinh doanh", "sales"},
	{"ban hang", "sales"},
	{"quan ly du an", "project_management"},
	{"quan tri du an", "project_management"},
	{"quan ly san pham", "product"},
	// The generic software words: programming, the software engineer.
	{"lap trinh", "software_engineering"},
	{"ky su phan mem", "software_engineering"},
	{"phat trien phan mem", "software_engineering"},
	{"chuyen vien phan mem", "software_engineering"},
	{"nhan vien phan mem", "software_engineering"},
	{"phat trien ung dung", "software_engineering"},
}

// vietnameseTechTerms are the Vietnamese titles that are confidently technical whatever category they resolve, the counterpart of techTitleTerms and held to the same rule: anchored to the discipline. A bare "phan mem" is not a term: "Kinh Doanh Phần Mềm" is a software seller.
var vietnameseTechTerms = []aliasEntry{
	{"lap trinh", ""},
	{"ky su phan mem", ""},
	{"phat trien phan mem", ""},
	{"kiem thu phan mem", ""},
	{"kien truc su phan mem", ""},
	{"phan mem nhung", ""},
	{"cong nghe thong tin", ""},
	{"cntt", ""},
	{"an ninh mang", ""},
	{"an ninh thong tin", ""},
	{"an toan thong tin", ""},
	{"bao mat thong tin", ""},
	{"quan tri he thong", ""},
	{"quan tri mang", ""},
	{"quan tri co so du lieu", ""},
	{"ky su du lieu", ""},
	{"khoa hoc du lieu", ""},
	{"hoc may", ""},
	{"ky su tri tue nhan tao", ""},
	{"dien toan dam may", ""},
	{"nhan vien it", ""},
	{"chuyen vien it", ""},
}

// vietnameseCues are the role phrases that let a title written without a distinctive letter into the Vietnamese tables ("Nhan Vien Kinh Doanh").
var vietnameseCues = []aliasEntry{
	{"nhan vien", ""},
	{"chuyen vien", ""},
	{"chuyen gia", ""},
	{"ky su", ""},
	{"truong phong", ""},
	{"giam doc", ""},
	{"ke toan", ""},
	{"kinh doanh", ""},
	{"lap trinh", ""},
	{"quan ly", ""},
}

// vietnameseSeniorityTable lists the grade words of a Vietnamese title. "Giám Đốc" and "Trưởng Phòng" (director, department head) state no grade, as "Director" and "Manager" do not; "cao cap" is the senior rung of a bank's ladder ("Chuyên Viên Cao Cấp"), but only as the grade of a specialist or an expert, never "premium".
var vietnameseSeniorityTable = []aliasEntry{
	{"thuc tap sinh", "intern"},
	{"thuc tap", "intern"},
	{"truong nhom", "lead"},
	{"to truong", "lead"},
	{"chuyen vien cao cap", "senior"},
	{"chuyen gia cao cap", "senior"},
	{"cvcc", "senior"},
	{"moi tot nghiep", "junior"},
}

// foldVietnamese prepares a lowercased title for the Vietnamese tables: every tone mark and diacritic is dropped and đ is written d, so "Kế Toán" reads "ke toan" however it was typed or composed.
func foldVietnamese(lower string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(lower) {
		switch {
		case unicode.Is(unicode.Mn, r):
		case r == 'đ':
			b.WriteRune('d')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// hasVietnameseLetter reports whether a lowercased title carries a letter no other Latin-script language in the dictionary writes: ơ, ư, or a tone-marked vowel (ạ to ỹ, the Latin Extended Additional block).
func hasVietnameseLetter(lower string) bool {
	for _, r := range norm.NFC.String(lower) {
		if r == 'ơ' || r == 'ư' || (r >= 'ạ' && r <= 'ỹ') {
			return true
		}
	}
	return false
}

// vietnameseWords folds a lowercased title and splits it into words, reporting nil when the title is not Vietnamese at all.
func vietnameseWords(lower string) []string {
	words := splitWords(foldVietnamese(lower))
	if !hasVietnameseLetter(lower) {
		if _, ok := matchStems(words, vietnameseCues); !ok {
			return nil
		}
	}
	return words
}

// matchVietnameseCategory resolves the category of a lowercased title the other tables left unresolved. A blind alias answers categoryNone.
func matchVietnameseCategory(lower string) string {
	c, _ := matchStems(vietnameseWords(lower), vietnameseCategoryTable)
	return c
}

// matchVietnameseSeniority resolves the grade of a lowercased title the other tables left unresolved.
func matchVietnameseSeniority(lower string) string {
	s, _ := matchStems(vietnameseWords(lower), vietnameseSeniorityTable)
	return s
}

// isVietnameseTech reports whether a lowercased title states a confidently technical role in Vietnamese.
func isVietnameseTech(lower string) bool {
	_, ok := matchStems(vietnameseWords(lower), vietnameseTechTerms)
	return ok
}
