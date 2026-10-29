package classify

import (
	"strings"
	"testing"
)

// TestParse_BCSHeadsAndFields pins the Croatian and Serbian fallback (bcs.go): a generic
// head takes its category from the inflected field after it, the role nouns are read in
// their inflected and feminine forms, and both standards and both scripts resolve. Each
// case is a live title from an open posting placed in Croatia, Serbia, Bosnia or
// Montenegro that resolved no category before.
func TestParse_BCSHeadsAndFields(t *testing.T) {
	cases := []struct{ title, want string }{
		// The field in the genitive, the locative and the instrumental.
		{"Voditelj trgovine (m/ž)", "retail"},
		{"VODITELJ/ICA TRGOVINE", "retail"},
		{"Zamjenik voditelja trgovine (m/ž), Zagreb", "retail"},
		{" Radnik u trgovini (m/ž)", "retail"},
		{"Specijalista za podršku klijentima na engleskom jeziku", "support"},
		{"Operater u korisničkom servisu", "support"},
		{"Agent (m/ž) u Službi Kontakt centar - Zagreb", "support"},
		{"REFERENT ZA RAČUNOVODSTVO (M/Ž)", "finance"},
		{"Suradnik za naplatu (m/ž)", "finance"},
		{"Stručni saradnik za procenu šteta na imovini", "finance"},
		{"Specijalist za zapošljavanje (m/ž)", "recruiting"},
		{"Student u odjelu za ljudske potencijale (f/m)", "hr"},
		{"Stručni suradnik za pravne poslove (m/ž)", "legal"},
		{"Radnik na komisioniranju (m/ž)", "logistics"},
		{"Referent špedicije", "logistics"},
		{"Radnik u tvornici stočne hrane (m/ž)", "manufacturing"},
		{"OPERATER/OPERATERKA NA PROIZVODNIM STROJEVIMA", "manufacturing"},
		{"OPERATER / OPERATERKA NA CNC GLODALICI", "manufacturing"},
		{"Tehničar za održavanje", "skilled_trades"},
		{"Radnik fizičkog obezbeđenja", "public_safety"},
		{"Agent za video nadzor", "public_safety"},
		{"KUHINJA [Dalmacija]", "hospitality"},
		{"Pomoćni kuhinjski radnik (m/ž)", "hospitality"},
		{"Agent u rezervacijskom centru Zadar (m/ž/d)", "hospitality"},
		{"DOMAĆINSTVO [Istra, Kvarner, Karlovac]", "personal_services"},
		{"Specijalist dermatovenerologije (m/ž) - Poliklinika Bagatin", "healthcare"},
		{"Doktor/ica dentalne medicine (protetika)", "healthcare"},
		{"Voditelj ljekarne (m/ž)", "healthcare"},
		{"Voditelj ključnih kupaca (m/ž)", "sales"},
		{"Ključni kupac - menadžer", "sales"},
		{"Studenti u teleprodaji Zagreb, Split i Osijek  (m/ž)", "sales"},
		{"Specijalist za internu komunikaciju (m/ž)", "marketing"},
		{"Voditelj robne marke", "marketing"},
		{"Asistent menadžera kategorije", "operations"},
		// The IT desks, the network, security and the engineers by discipline.
		{"INFORMATIČKI/A SPECIJALIST/ICA", "it_support"},
		{"Predstavnik tehničke podrške - engleski jezik", "it_support"},
		{"TEHNIČAR/TEHNIČARKA ZA RAČUNALNE SUSTAVE", "it_support"},
		{"ADMINISTRATOR/ICA BAZE PODATAKA", "devops"},
		{"MREŽNI ADMINISTRATOR / ADMINISTRATORICA", "network_engineering"},
		{"Specijalist cyber i informacijske sigurnosti (m/ž)", "security"},
		{"MLAĐI/A INŽENJER/KA INFORMACIJSKIH TEHNOLOGIJA", "software_engineering"},
		{"INŽENJER/INŽENJERKA CJELOVITOG RAZVOJA", "fullstack"},
		{"Developer baze podataka – Mid / Senior (m/ž)", "software_engineering"},
		{"Poslovni analitičar (m/ž)", "business_analysis"},
		{"SQL ANALITIČAR/KA", "data_analytics"},
		{"Inženjer na gradilištu (m/ž)", "industrial_engineering"},
		{"Dizajner kartonske ambalaže", "design"},
		{"VIDEOGRAF/VIDEOGRAFKINJA", "creative"},
		// The role nouns in their feminine and inflected forms.
		{"Brijači za nordijske zemlje", "personal_services"},
		{"HIDRAULIČAR/HIDRAULIČARKA", "skilled_trades"},
		// A business's development is sales, an employee's is HR.
		{"Menadžer za razvoj korporativnog poslovanja", "sales"},
		{"Specijalist za zapošljavanje i razvoj zaposlenika ", "hr"},
		// The shop floor is retail, not the sales team.
		{"Voditelj prodajnog mjesta (ž/m)", "retail"},
		{"Voditelj prodajnog područja", "sales"},
		// The executive's assistant is administration.
		{"Pomoćnik izvršnog direktora", "administration"},
		// Bosnian lines (mojposao), measured 2026-10-29: the engineer spelled "inžinjer",
		// the loan officer, customs, transport planning and the customer adviser.
		{"Diplomirani inžinjer mašinstva", "industrial_engineering"},
		{"Inženjer kvaliteta (m/ž)", "industrial_engineering"},
		{"Kreditni službenik/službenik za klijente", "finance"},
		{"Carinski referent (m/ž)", "logistics"},
		{"Organizator međunarodnog transporta (m/ž)", "logistics"},
		{"Savjetnik za kupce (m/ž)", "support"},
		{"Referent za reklamacije (m/ž)", "support"},
		{"Agent u kontaktnom centru (m/ž)", "support"},
		{"Operater na uplatno-isplatnom mjestu", "retail"},
		{"Radnik/ca na mašinskom pranju suđa", "hospitality"},
		{"Radnik na stroju za kantiranje", "manufacturing"},
		{"Rukovalac građevinskih mašinama", "skilled_trades"},
		{"Interni revizor", "finance"},
		{"Finansijski referent", "finance"},
		{"Specijalista za usklađenost poslovanja (m/ž)", "legal"},
		{"Tehničar za videonadzor i sigurnosne sisteme", "public_safety"},
		// Serbian Cyrillic is read through its Latin spelling.
		{"Програмер", "software_engineering"},
		{"Возач Б категорије", "logistics"},
		{"Продавац у малопродајном објекту", "retail"},
		{"Конобар/ица", "hospitality"},
		{"Медицинска сестра", "healthcare"},
		{"Референт за рачуноводство", "finance"},
		// Without the diacritics, as some boards print them.
		{"Vozac kamiona", "logistics"},
		{"Racunovodja (m/z)", "finance"},
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.want {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.want)
		}
	}
}

// TestParse_BCSUnresolved pins what the BCS fallback leaves alone: an open application
// names no role whatever department it mentions, nor does a scholarship or a mystery
// shopper; the generic heads with no field resolve nothing, as in the main table; and a
// quality-assurance desk is the tester or the inspector, which the title rarely says.
func TestParse_BCSUnresolved(t *testing.T) {
	for _, title := range []string{
		"Otvorena molba",
		"*Otvorena molba u Sektoru informatike",
		"Otvorena prijava - Logističko  distributivni centri",
		"Otvorene zamolbe za posao",
		"Opšta prijava - Knauf Zemun, Beograd",
		"Atlantbh Stipendija 2026. Kategorija 1: Uspješni studenti",
		"Tajni kupac (m/ž)",
		"Tajni kupac - maloprodaja",
		"TAJNI KUPAC U AUTOMOBILSKOJ INDUSTRIJI",
		"Zamjenik voditelja osiguranja kvalitete (m/ž)",
		"Voditelj smjene (m/ž)",
		"Specijalist tehničkih poslova (m/ž)",
		"Operater",
		"Inženjer",
		"Razvojni inženjer",
		"Developer",
	} {
		if got := Parse(title).Category; got != "" {
			t.Errorf("Parse(%q).Category = %q, want unresolved", title, got)
		}
	}
}

// TestParse_BCSSeniority pins the BCS grade words: the trainee, the junior and senior
// rungs of the civil-service titles, the team's and the shift's lead, and only the
// general, executive and financial directors as the executive.
func TestParse_BCSSeniority(t *testing.T) {
	cases := []struct{ title, want string }{
		{"INFORMATIČKI/A SURADNIK/CA (PRIPRAVNIK/CA)", "intern"},
		{"Pripravnik u hotelskim operacijama (m/ž)", "intern"},
		{"Studentska praksa", "intern"},
		{"Otvorena prijava za stručnu praksu", "intern"},
		{"Praktikant u računovodstvu", "intern"},
		{"MLAĐI/A KONZULTANT/ICA", "junior"},
		{"Niži saradnik u odeljenju za podršku", "junior"},
		{"PROGRAMER POČETNIK/PROGRAMERKA POČETNICA", "junior"},
		{"VIŠI/A STRUČNI/A SURADNIK/CA ZA INFORMATIKU", "senior"},
		{"Viši specijalist u računovodstvu (m/ž)", "senior"},
		{"Stariji stručni suradnik u financijskom računovodstvu", "senior"},
		{"Voditelj / Voditeljica smjene - Samobor", "lead"},
		{"Vođa smene (Beograd - Dorćol)", "lead"},
		{"Vođa tima", "lead"},
		{"Tim lider i zastupnik u osiguranju", "lead"},
		{"Šef kuhinje (m/ž)", "lead"},
		{"Vodeći analitičar za portfolio analize i izvještavanja (m/ž)", "lead"},
		{"Generalni direktor", "c_level"},
		{"Izvršni direktor za finansije", "c_level"},
		{"Finansijski direktor (m/ž)", "c_level"},
		{"Predsjednik Uprave", "c_level"},
		{"Пословођа малопродајног објекта - приправник", "intern"},
	}
	for _, c := range cases {
		if got := Parse(c.title).Seniority; got != c.want {
			t.Errorf("Parse(%q).Seniority = %q, want %q", c.title, got, c.want)
		}
	}
	// "Više" is "more" as often as "senior"; a director of a department, the assistant
	// or deputy of a lead, and the Slovenian "vodja" (any manager) state no grade.
	for _, title := range []string{
		"Prodavac (više gradova)",
		"Direktor prodaje",
		"Direktor poslovnice Varaždin (m/ž)",
		"Pomoćnik izvršnog direktora",
		"Zamjenik voditelja smjene",
		"Asistent šefa kuhinje",
		"Vodja prodaje",
		"Voditelj trgovine (m/ž)",
		"Visa Analyst",
	} {
		if got := Parse(title).Seniority; got != "" {
			t.Errorf("Parse(%q).Seniority = %q, want unresolved", title, got)
		}
	}
}

// TestLatinizeSerbian pins the transliteration and its gate. A title is transliterated
// only when it shows it is Serbian: a letter only Serbian uses, or a Serbian role word.
// Russian, Ukrainian and Bulgarian titles are left as they are, including the ones
// spelled only with letters the alphabets share, and so is a Latin title carrying a
// Cyrillic homoglyph.
func TestLatinizeSerbian(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"програмер", "programer"},
		{"возач б категорије", "vozač b kategorije"},
		{"љекар у ђачком дому", "ljekar u đačkom domu"},
		{"медицинска сестра", "medicinska sestra"},
		{"шеф-повар", "шеф-повар"},
		{"контролер качества", "контролер качества"},
		{"аналитик в логистику", "аналитик в логистику"},
		{"кухар", "кухар"},
		{"пентестер", "пентестер"},
		{"водитель погрузчика", "водитель погрузчика"},
		{"оператор кол-центру і продажів", "оператор кол-центру і продажів"},
		{"chief executive officer (сео)", "chief executive officer (сео)"},
		{"programer", "programer"},
	} {
		if got := latinizeSerbian(c.in); got != c.want {
			t.Errorf("latinizeSerbian(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TestParse_BCSLeavesOtherCyrillicAlone pins Russian and Ukrainian titles the main table
// leaves unresolved, which a transliteration would have read as BCS words.
func TestParse_BCSLeavesOtherCyrillicAlone(t *testing.T) {
	for _, title := range []string{
		"Шеф-редактор новинного Telegram-каналу", "Контролер", "Аналитик в логистику",
		"Колорист", "Кухар", "Chief Executive Officer (СЕО)",
	} {
		if c := matchBCSCategory(strings.ToLower(title)); c != "" {
			t.Errorf("matchBCSCategory(%q) = %q, want unresolved", title, c)
		}
		if s := matchBCSSeniority(strings.ToLower(title)); s != "" {
			t.Errorf("matchBCSSeniority(%q) = %q, want unresolved", title, s)
		}
	}
}
