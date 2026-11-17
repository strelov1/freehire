package classify

import "testing"

// turkishITTitles are live titles from Kariyer.net's IT work areas (437 postings, 2026-11-17),
// each filed where its English twin is. The English dictionary placed 30 of the 437 as technical
// and found a category for 60; the Turkish tables place 191 and 285.
var turkishITTitles = []struct {
	title, category, seniority string
	tech                       bool
}{
	// Software development, however the head is spelled.
	{"Yazılım Geliştirme Uzmanı", "software_engineering", "", true},
	{"Kıdemli Yazılım Geliştirme Uzmanı", "software_engineering", "senior", true},
	{"Yazılım Mühendisi", "software_engineering", "", true},
	{"Yazılım Stajyeri", "software_engineering", "intern", true},
	{"Yazılım Şefi", "software_engineering", "", true},
	{"SAP ABAP Yazılım Geliştirme Uzmanı", "software_engineering", "", true},
	{"ABAP Geliştirme Uzmanı", "software_engineering", "", false},
	// The software roles that are not plain development.
	{"Yazılım Test Stajyeri (Web ve Mobil Uygulamalar)", "qa", "intern", true},
	{"Yazılım Projeleri Müdürü", "project_management", "", false},
	{"Yazılım Destek Uzmanı / Hbys - Adana", "it_support", "", true},
	{"Kurumsal Yazılım Mimarı ( Enterprise Architect It/Software)", "architecture", "", true},
	{"Çözüm Mimarı", "architecture", "", false},
	{"Yazılım Satış Uzmanı", "sales", "", false},
	// The IT department and the service desk.
	{"Bilgi İşlem Uzmanı", "it_support", "", true},
	{"Mersin-Bilgi İşlem Uzmanı", "it_support", "", true},
	{"Bilgi İşlem Uzman Yardımcısı", "it_support", "junior", true},
	{"Bilgi İşlem Stajyeri", "it_support", "intern", true},
	{"Bilgi Teknolojileri Uzmanı", "it_support", "", true},
	{"Bilgi Teknolojileri / IT Uzmanı", "it_support", "", true},
	{"Bilişim Teknolojileri Uzmanı / Uzman Yardımcısı", "it_support", "junior", true},
	{"BT Teknik Destek Uzmanı", "it_support", "", true},
	{"IT Destek Uzmanı", "it_support", "", true},
	{"Son Kullanıcı Destek Uzmanı", "it_support", "", false},
	{"Sistem Destek Uzmanı", "it_support", "", false},
	// An IT manager resolves nothing, as "IT Director" does; the posting is still technical.
	{"Bilgi Teknolojileri Müdürü", "", "", true},
	{"Bilgi İşlem Müdürü", "", "", true},
	// Systems, network, security.
	{"Sistem Uzmanı", "system_administration", "", false},
	{"Sistem ve Yedekleme Uzmanı", "system_administration", "", false},
	{"Ağ ve Sistem Yönetimi Ekip Lideri", "system_administration", "", false},
	{"Network Uzmanı", "network_engineering", "", false},
	{"Sistem ve Network Lideri", "network_engineering", "", false},
	{"Kıdemli Network Mühendisi", "network_engineering", "senior", false},
	{"Siber Güvenlik Uzmanı", "security", "", true},
	{"Bilgi Güvenliği Uzmanı", "security", "", true},
	{"Ağ ve Güvenlik Uzmanı", "security", "", false},
	{"Bulut Altyapı Yöneticisi", "devops", "", false},
	// ERP and SAP.
	{"ERP Uzmanı", "solutions_engineering", "", false},
	{"SAP FI Danışmanı", "solutions_engineering", "", false},
	{"Netsis Uzmanı", "solutions_engineering", "", false},
	{"SAP BW - BI Yazılım Uzmanı", "software_engineering", "", true},
	// Data, AI, analysis.
	{"İş Zekası Uzmanı", "data_analytics", "", false},
	{"Veri Ambarı Analisti", "data_engineering", "", false},
	{"Yapay Zeka ve Makine Öğrenmesi Mühendisi", "ml_ai", "", true},
	{"İş Analisti", "business_analysis", "", false},
	{"Proje Yöneticisi / Yönetmeni", "project_management", "", false},
	{"Veri Tabanı Yönetimi Uzman Yardımcısı", "devops", "junior", true},
}

func TestTurkishITTitles(t *testing.T) {
	for _, c := range turkishITTitles {
		got := Parse(c.title)
		if got.Category != c.category || got.Seniority != c.seniority {
			t.Errorf("Parse(%q) = {category %q, seniority %q}, want {%q, %q}", c.title, got.Category, got.Seniority, c.category, c.seniority)
		}
		if IsTech(c.title) != c.tech {
			t.Errorf("IsTech(%q) = %v, want %v", c.title, !c.tech, c.tech)
		}
	}
}

// Dotted and dotless i survive upper-casing: lowercasing "İ" leaves a combining dot behind, and
// the fold has to drop it for "BİLGİ İŞLEM" to read as "bilgi islem".
func TestTurkishFoldsTheDottedCapital(t *testing.T) {
	for _, title := range []string{"BİLGİ İŞLEM UZMANI", "Bilgi İşlem Uzmanı", "bilgi işlem uzmanı", "Bilgi Islem Uzmani"} {
		if got := Parse(title).Category; got != "it_support" {
			t.Errorf("Parse(%q).Category = %q, want it_support", title, got)
		}
		if !IsTech(title) {
			t.Errorf("IsTech(%q) = false", title)
		}
	}
}

// The Turkish words that name a role are never read as a grade or a craft on their own, and a
// Turkish title outside IT keeps resolving nothing technical.
func TestTurkishTablesNeverGuess(t *testing.T) {
	for _, title := range []string{
		"Teknik Destek Uzmanı",            // technical support of anything
		"Kalite Sistem Yönetim Sorumlusu", // a quality system, not a sysadmin
		"Kalite Sistem Uzmanı",
		"Yapay Zeka Uzmanı", // the English twin "AI Specialist" resolves nothing either
		"Sistem Mühendisi",  // the English "System Engineer" is software_engineering
		"Veri Tabanı Uzmanı",
		"Saha Uzmanı",
		"Satış Uzmanı",
		"Resepsiyon Sorumlusu",
		"Şirket Şoförü",
	} {
		if got := Parse(title).Category; got != "" {
			t.Errorf("Parse(%q).Category = %q, want none", title, got)
		}
		if IsTech(title) {
			t.Errorf("IsTech(%q) = true, want false", title)
		}
	}
	if got := Parse("Uzman").Seniority; got != "" {
		t.Errorf("a bare Uzman states no grade, got %q", got)
	}
}

// The Turkish fallback runs only after every other table resolved nothing and only on a title
// carrying a Turkish role word, so no title in another language changes.
func TestTurkishTablesLeaveOtherLanguagesAlone(t *testing.T) {
	cases := []struct{ title, category string }{
		{"SAP Basis Administrator", "devops"},
		{"ERP Implementation Lead", ""},
		{"SAP Operations Lead", ""},
		{"Software Engineer", "software_engineering"},
		{"IT Specialist", "it_support"},
		{"Network Specialist", "network_engineering"},
		{"Programista Java (K/M)", "software_engineering"},
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.category {
			t.Errorf("Parse(%q).Category = %q, want %q: a title with no Turkish word must not reach the Turkish table", c.title, got, c.category)
		}
	}
}
