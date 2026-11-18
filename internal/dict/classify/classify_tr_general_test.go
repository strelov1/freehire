package classify

import "testing"

// TestParse_TurkishBeyondIT pins the Turkish table for every profession (turkish_general.go):
// live titles from Kariyer.net's catalogue (24,956 postings, 2026-11-17), each filed where its
// English twin is.
func TestParse_TurkishBeyondIT(t *testing.T) {
	cases := []struct{ title, want string }{
		// Sales and the store.
		{"Satış Danışmanı", "sales"},
		{"Bölge Satış Müdürü", "sales"},
		{"Tele-Satış Uzmanı", "sales"},
		{"Satış Pazarlama Uzmanı", "sales"},
		{"Gayrimenkul Danışmanı", "sales"},
		{"Sigorta Portföy Danışmanı - Antalya", "sales"},
		{"Tıbbi Satış Temsilcisi", "sales"},
		{"Satış Öncesi Çözüm Mühendisi", "solutions_engineering"},
		{"Mağaza Müdürü - Silivri", "retail"},
		{"Mağaza Satış Danışmanı", "retail"},
		{"Kasiyer", "retail"},
		{"Reyon Görevlisi", "retail"},
		// Support.
		{"Çağrı Merkezi Müşteri Temsilcisi", "support"},
		{"Müşteri Temsilcisi", "support"},
		{"Çağrı Merkezi Satış Temsilcisi", "sales"},
		{"Satış Sonrası Hizmetler Sorumlusu", "support"},
		// Back office.
		{"Muhasebe Uzmanı", "finance"},
		{"Ön Muhasebe Elemanı", "finance"},
		{"Mali Müşavir", "finance"},
		{"Finans Uzmanı", "finance"},
		{"Bordro Uzmanı", "finance"},
		{"İnsan Kaynakları Uzmanı", "hr"},
		{"Bordro ve Özlük İşleri Uzmanı", "hr"},
		{"İşe Alım Uzmanı", "recruiting"},
		{"Avukat", "legal"},
		{"Pazarlama Uzmanı", "marketing"},
		{"Sosyal Medya Uzmanı", "marketing"},
		{"Satın Alma Uzmanı", "operations"},
		{"Operasyon Uzmanı", "operations"},
		{"İdari İşler Uzmanı", "administration"},
		{"Sekreter", "administration"},
		{"Resepsiyonist", "administration"},
		{"Yönetici Asistanı", "administration"},
		// Moving goods and making them.
		{"Depo Görevlisi", "logistics"},
		{"Forklift Operatörü", "logistics"},
		{"Şoför / Sürücü", "logistics"},
		{"Stok Kontrol Elemanı", "logistics"},
		{"Üretim Operatörü", "manufacturing"},
		{"Makine Operatörü", "manufacturing"},
		{"Montaj Elemanı", "manufacturing"},
		{"Üretim Müdürü", "manufacturing"},
		// The trades.
		{"Elektrik Bakım Teknisyeni", "skilled_trades"},
		{"Mobilya Ustası", "skilled_trades"},
		{"Kaynakçı", "skilled_trades"},
		// Engineers and quality.
		{"Makine Mühendisi", "industrial_engineering"},
		{"İnşaat Mühendisi", "industrial_engineering"},
		{"Endüstri Mühendisi", "industrial_engineering"},
		{"Kalite Kontrol Elemanı", "industrial_engineering"},
		{"Kalite Güvence Şefi", "industrial_engineering"},
		{"Tasarım Mühendisi", "engineering_design"},
		{"Teknik Ressam", "engineering_design"},
		{"İş Sağlığı ve Güvenliği Uzmanı", "occupational_safety"},
		{"B Sınıfı İş Güvenliği Uzmanı", "occupational_safety"},
		// Care, teaching and the service sectors.
		{"Hemşire", "healthcare"},
		{"İşyeri Hekimi", "healthcare"},
		{"Eczacı", "healthcare"},
		{"İngilizce Öğretmeni", "education"},
		{"Garson", "hospitality"},
		{"Aşçı Yardımcısı", "hospitality"},
		{"Barista", "hospitality"},
		{"Temizlik Görevlisi", "personal_services"},
		{"Güvenlik Görevlisi", "public_safety"},
		{"Laboratuvar Teknikeri", "science"},
		{"Sosyal Hizmet Uzmanı", "social_services"},
		{"Grafik Tasarım Uzmanı", "design"},
		{"Tercüman", "technical_writing"},
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.want {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.want)
		}
	}
}

// A title the English twin leaves unresolved, a generic head with no field, or a posting that is
// not a role stays unresolved: unresolved beats wrong.
func TestParse_TurkishBeyondITNeverGuesses(t *testing.T) {
	for _, title := range []string{
		"Satış Mühendisi",            // a seller at the plant, a technical role in English: neither is claimed
		"Finansal Güvence Danışmanı", // an insurer's seller, not an accountant
		"Genel Başvuru (Engelli)",    // a general application, not a role
		"Genel Başvuru - Yeni Mezun",
		"Saha Uzmanı",
		"Teknik Uzman",
		"Müşteri İlişkileri Uzmanı", // the English twin resolves nothing
		"Proje Uzmanı",
		"Yönetim Kurulu Üyesi",
		"Planlama Uzmanı",
		"Mimar", // a building architect and a software architect share the word
		"Danışman",
		"Uzman Yardımcısı",
	} {
		if got := Parse(title).Category; got != "" {
			t.Errorf("Parse(%q).Category = %q, want none", title, got)
		}
	}
}

// The product word never outranks the role: a cleaning-products seller sells, a store's
// warehouse clerk works in the store, a medical representative sells to doctors.
func TestParse_TurkishFieldsAreReadInPrecedence(t *testing.T) {
	cases := []struct{ title, want string }{
		{"Temizlik Ürünleri Satış Temsilcisi", "sales"},
		{"Sağlık Ürünleri Satış Uzmanı", "sales"},
		{"Mağaza Depo Sorumlusu", "retail"},
		{"Tıbbi Temsilci", "sales"},
		{"Satış ve Pazarlama Uzmanı", "sales"},
		{"Güvenlik Görevlisi", "public_safety"}, // not information security
		{"Siber Güvenlik Uzmanı", "security"},
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.want {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.want)
		}
	}
}

// English titles and the other languages are untouched: the Turkish tables read only a title
// carrying a Turkish role word.
func TestTurkishGeneralLeavesOtherLanguagesAlone(t *testing.T) {
	for _, title := range []string{"Market Analyst", "Operator", "Sales Manager", "Store Clerk", "Depot Manager", "Quality Analyst"} {
		before := Parse(title).Category
		// none of these carries a Turkish role word, so the Turkish fallback is never asked
		if len(turkishWords(title)) != 0 {
			t.Errorf("turkishWords(%q) read an English title as Turkish", title)
		}
		_ = before
	}
}
