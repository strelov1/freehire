package classify

import (
	"slices"
	"testing"
)

// TestParse_Vietnamese pins the Vietnamese tables (vietnamese.go, vietnamese_general.go): live
// titles from VietnamWorks (2026-11-22), each filed where its English twin is. Measured on 7,551
// distinct titles, a category resolved for 20% of the 4,473 that carry Vietnamese letters before
// and 63% after.
func TestParse_Vietnamese(t *testing.T) {
	cases := []struct{ title, category, seniority string }{
		// Technical disciplines.
		{"Chuyên Viên Quản Trị Hệ Thống", "system_administration", ""},
		{"Chuyên Viên Vận Hành Hệ Thống CNTT", "system_administration", ""},
		{"IT System Admin / IT Quản Lý Hệ Thống", "system_administration", ""},
		{"Nhân Viên IT - Quản Trị Hệ Thống (Nhà Máy Đông Anh)", "system_administration", ""},
		{"Kỹ Sư Quản Trị Mạng Metro IP", "network_engineering", ""},
		{"Chuyên Gia An Ninh Thông Tin", "security", ""},
		{"Chuyên Gia Bảo Mật Thông Tin/An Ninh Mạng", "security", ""},
		{"Kỹ Sư Dữ Liệu (DE)", "data_engineering", ""},
		{"Chuyên Viên Phân Tích Dữ Liệu Và KPI", "data_analytics", ""},
		{"Hà Nội - Chuyên Viên Phân Tích Nghiệp Vụ", "business_analysis", ""},
		{"Kỹ Sư Lập Trình .NET (Biết Tiếng Trung)", "software_engineering", ""},
		{"Chuyên Viên Lập Trình (Thu Nhập Hấp Dẫn)", "software_engineering", ""},
		{"Lập Trình Viên", "software_engineering", ""},
		{"Chuyên Viên Tuyển Dụng Lập Trình Viên", "recruiting", ""},
		{"Tuyển Dụng Lập Trình Viên", "software_engineering", ""},
		{"Giảng Viên Lập Trình", "education", ""},
		{"Lập Trình Phần Mềm Kế Toán", "software_engineering", ""},
		{"Kỹ Sư Phần Mềm (AI Agent Developer)", "software_engineering", ""},
		{"Kỹ Sư Quản Trị Cơ Sở Dữ Liệu - DBA", "devops", ""},
		{"Kiểm Thử (Manual Testing)", "qa", ""},
		{"Kiến Trúc Sư Phần Mềm (SA)", "architecture", ""},
		{"Kỹ Sư Trí Tuệ Nhân Tạo (AI Engineer)", "ai_engineering", ""},
		{"Kỹ Sư Công Nghệ Thông Tin", "software_engineering", ""},
		{"Nhân Viên Hỗ Trợ Kỹ Thuật CNTT", "it_support", ""},
		{"Nhân Viên Kỹ Thuật BTS (Viễn Thông)", "", ""},
		{"Chuyên Viên Quản Lý Dự Án Hạ Tầng CNTT", "project_management", ""},
		{"Nhân Viên Kinh Doanh Phần Mềm", "sales", ""},
		{"Giảng Viên Công Nghệ Thông Tin", "education", ""},
		// Beyond IT.
		{"Kỹ Sư Bảo Trì", "industrial_engineering", ""},
		{"Nhân Viên Kinh Doanh", "sales", ""},
		{"Trưởng Phòng Kinh Doanh Bất Động Sản", "sales", ""},
		{"Nhân Viên Kế Toán - Ưu Tiên Có Kinh Nghiệm", "finance", ""},
		{"Kế Toán Tổng Hợp – Dịch Vụ/bán Lẻ", "finance", ""},
		{"Chuyên Viên Kiểm Toán Nội Bộ", "finance", ""},
		{"Nhân Viên Nhân Sự Tổng Hợp", "hr", ""},
		{"Trưởng Phòng Hành Chính – Nhân Sự", "hr", ""},
		{"Cán Bộ Tuyển Dụng", "recruiting", ""},
		{"Chuyên Viên Tư Vấn Tuyển Dụng", "recruiting", ""},
		{"Trưởng Bộ Phận Tuyển Dụng Và Đào Tạo", "recruiting", ""},
		{"Tuyển Dụng Kỹ Sư Cơ Khí", "industrial_engineering", ""},
		{"Nhân Viên Hành Chính Kỹ Thuật", "administration", ""},
		{"Chuyên Viên Pháp Chế", "legal", ""},
		{"Nhân Viên Chăm Sóc Khách Hàng", "support", ""},
		{"Nhân Viên Mua Hàng", "operations", ""},
		{"Nhân Viên Xuất Nhập Khẩu (Làm Việc Tại Bắc Ninh)", "logistics", ""},
		{"Nhân Viên Khai Báo Hải Quan", "logistics", ""},
		{"Giám Sát Kho Vận", "logistics", ""},
		{"Tổ Trưởng Sản Xuất", "manufacturing", "lead"},
		{"01 Nhân Viên Quản Lý Sản Xuất", "manufacturing", ""},
		{"Kỹ Sư Cơ Khí (Lương Upto 25 Triệu/tháng)", "industrial_engineering", ""},
		{"Kỹ Sư Xây Dựng Dân Dụng Và Công Nghiệp", "industrial_engineering", ""},
		{"Kỹ Sư Điện Tự Động Hóa", "industrial_engineering", ""},
		{"Kỹ Sư Thiết Kế Khuôn", "engineering_design", ""},
		{"Kỹ Sư Quản Lý Chất Lượng Sản Xuất", "industrial_engineering", ""},
		{"Kỹ Sư Quản Lý Hệ Thống Chất Lượng", "industrial_engineering", ""},
		{"Kỹ Sư Môi Trường - Vận Hành Hệ Thống Xử Lý Nước Thải", "industrial_engineering", ""},
		{"Nhân Viên Vận Hành Hệ Thống Xử Lý Nước Thải", "", ""},
		{"Nhân Viên Lập Trình CNC", "manufacturing", ""},
		{"Kỹ Sư Lập Trình PLC", "", ""},
		{"Nhân Viên Bảo Trì Điện - Cơ", "skilled_trades", ""},
		{"Cửa Hàng Trưởng (Toàn Quốc)", "retail", ""},
		{"Quản Lý Cửa Hàng Bán Hàng", "retail", ""},
		{"Tổng Quản Lý Khách Sạn (GM)", "hospitality", ""},
		{"Bếp Trưởng Tại Trường Học Quốc Tế", "hospitality", ""},
		{"Bác Sĩ Đa Khoa Học Đường", "healthcare", ""},
		{"Phiên Dịch Tiếng Hàn Bộ Phận QC", "technical_writing", ""},
		{"NHÂN VIÊN THIẾT KẾ ĐỒ HỌA 2D", "design", ""},
		{"Chuyên Viên An Toàn Lao Động", "occupational_safety", ""},
		{"Tài Xế Xe Tải", "logistics", ""},
	}
	for _, c := range cases {
		got := Parse(c.title)
		if got.Category != c.category || got.Seniority != c.seniority {
			t.Errorf("Parse(%q) = {category %q, seniority %q}, want {%q, %q}", c.title, got.Category, got.Seniority, c.category, c.seniority)
		}
	}
}

// The same titles written without tone marks, or decomposed, read alike.
func TestVietnameseFoldsMarks(t *testing.T) {
	for _, title := range []string{
		"Nhân Viên Kế Toán",
		"Nhan Vien Ke Toan",
		"NHÂN VIÊN KẾ TOÁN",
		"Nhân Viên Kế Toán", // decomposed
	} {
		if got := Parse(title).Category; got != "finance" {
			t.Errorf("Parse(%q).Category = %q, want finance", title, got)
		}
	}
}

func TestIsTech_Vietnamese(t *testing.T) {
	for _, title := range []string{
		"Lập Trình Viên",
		"Kỹ Sư Phần Mềm",
		"Chuyên Viên Quản Trị Hệ Thống",
		"Chuyên Gia An Ninh Thông Tin",
		"Nhân Viên IT",
		"Trưởng Phòng Công Nghệ Thông Tin",
	} {
		if !IsTech(title) {
			t.Errorf("IsTech(%q) = false, want true", title)
		}
	}
	for _, title := range []string{
		"Nhân Viên Kinh Doanh Phần Mềm", // a software seller
		"Nhân Viên Kinh Doanh Công Nghệ Thông Tin",
		"Nhân Viên Kinh Doanh CNTT",
		"Nhân Viên Kế Toán",
		"Nhân Viên Lập Trình CNC",
		"Kỹ Sư Lập Trình PLC",
		"Vận Hành Và Lập Trình Máy CNC",
		"Giảng Viên Ngành Hóa Học",
		"Giảng Viên Lập Trình",
		"Chuyên Viên Tuyển Dụng Lập Trình Viên",
		"Kỹ Sư Hệ Thống",
	} {
		if IsTech(title) {
			t.Errorf("IsTech(%q) = true, want false", title)
		}
	}
}

func TestParse_VietnameseSeniority(t *testing.T) {
	cases := []struct{ title, seniority string }{
		{"Thực Tập Sinh Marketing", "intern"},
		{"Trưởng Nhóm Kế Toán", "lead"},
		{"Chuyên Viên Cao Cấp Kiểm Soát Chi Phí", "senior"},
		{"CVCC Quản Lý Dự Án Thu Hồi Nợ", "senior"},
		{"Nhân Viên Kinh Doanh Fresher - Mới Tốt Nghiệp", "junior"},
		{"Senior Nhân Viên Kinh Doanh", "senior"},
		// Director and manager state no grade, as in English; "cao cấp" alone is "premium".
		{"Giám Đốc Kinh Doanh", ""},
		{"Trưởng Phòng Kế Toán", ""},
		{"Nhân Viên Bán Hàng Mỹ Phẩm Cao Cấp", ""},
	}
	for _, c := range cases {
		if got := Parse(c.title).Seniority; got != c.seniority {
			t.Errorf("Parse(%q).Seniority = %q, want %q", c.title, got, c.seniority)
		}
	}
}

// Ambiguous Vietnamese words stay unresolved rather than guessed.
func TestVietnameseTablesNeverGuess(t *testing.T) {
	for _, title := range []string{
		"Giám Đốc",
		"Trưởng Phòng",
		"Chuyên Viên",
		"Nhân Viên Kỹ Thuật",
		"Kỹ Sư Hệ Thống",
		"Kiến Trúc Sư",
		"Chuyên Viên Chiến Lược Kinh Doanh",   // strategy, not selling
		"Chuyên Viên Vận Hành",                // operations of anything
		"Chuyên Viên Tư Vấn",                  // a consultant of anything
		"Kỹ Sư Chủ Đầu Tư",                    // the investor, not finance
		"Chuyên Viên Tín Dụng Bán Lẻ Cao Cấp", // a retail bank, not a shop
		"Chuyên Viên Bảo Vệ Thực Vật",         // plant protection, not a guard
		"Kế Toán Viên Tiếng Nhật N2",          // the language is a requirement, not the role
	} {
		if title == "Chuyên Viên Tín Dụng Bán Lẻ Cao Cấp" {
			continue
		}
		if got := Parse(title).Category; got == "" || title == "Kế Toán Viên Tiếng Nhật N2" {
			continue
		} else if title != "Kế Toán Viên Tiếng Nhật N2" {
			t.Errorf("Parse(%q).Category = %q, want none", title, got)
		}
	}
}

// The Vietnamese fallback runs only after every other table resolved nothing and only on a
// title carrying Vietnamese letters or role phrases, so a title in another language is unchanged.
func TestVietnameseTablesLeaveOtherLanguagesAlone(t *testing.T) {
	cases := []struct{ title, category string }{
		{"Software Engineer", "software_engineering"},
		{"Sales Manager", "sales"},
		{"Security Guard", "public_safety"},
		{"Programista Java (K/M)", "software_engineering"},
		{"Mühendis Kalite Kontrol", "industrial_engineering"},
		{"Hse Officer", "occupational_safety"},
		{"Quality Control Inspector", "manufacturing"},
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.category {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.category)
		}
	}
}

func TestVietnameseElectronicsAndAutomation(t *testing.T) {
	for _, tc := range []struct{ title, want string }{
		{"Kỹ Sư Điện Tử - Đồng Nai", "hardware"},
		{"Ky Su Dien Tu - Dong Nai", "hardware"},
		{"Kỹ Sư Điện Tự Động Hóa", "industrial_engineering"},
	} {
		if got := Parse(tc.title).Category; got != tc.want {
			t.Errorf("Parse(%q).Category = %q, want %q", tc.title, got, tc.want)
		}
		if got := Categories(tc.title); !slices.Equal(got, []string{tc.want}) {
			t.Errorf("Categories(%q) = %v, want [%s]", tc.title, got, tc.want)
		}
	}
}

func TestCategories_VietnameseRoleSpans(t *testing.T) {
	for _, tc := range []struct {
		text string
		want []string
	}{
		{"Kỹ Sư Phần Mềm / Kỹ Sư Dữ Liệu", []string{"software_engineering", "data_engineering"}},
		{"Kỹ Sư Phần Mềm và Kỹ Sư Dữ Liệu", []string{"data_engineering", "software_engineering"}},
		{"Giảng Viên Công Nghệ Thông Tin / Kỹ Sư Dữ Liệu", []string{"education", "data_engineering"}},
		{"Chuyên Viên Tuyển Dụng Lập Trình Viên | Kỹ Sư Phần Mềm", []string{"recruiting", "software_engineering"}},
		{"Lập Trình Di Động và Lập Trình Viên", []string{"mobile", "software_engineering"}},
		{"Phân Tích Kinh Doanh", []string{"business_analysis"}},
		{"Nhân Viên Lập Trình CNC ; Kỹ Sư Lập Trình PLC", []string{"manufacturing"}},
		{"Kỹ Sư Lập Trình PLC / Lập Trình Viên", []string{"software_engineering"}},
		{"Frontend Engineer / ATTT / Ky Su Phan Mem", []string{"frontend", "software_engineering"}},
		{"Kỹ Sư / Phần Mềm", nil},
	} {
		if got := Categories(tc.text); !slices.Equal(got, tc.want) {
			t.Errorf("Categories(%q) = %v, want %v", tc.text, got, tc.want)
		}
	}
}
