package classify

import "testing"

// TestParse_CentralEasternEuropeTitles pins the vocabulary added for freehire#3513: the
// open postings of one pharmaceutical employer, written in Slovenian, Polish, Ukrainian,
// Bulgarian, Hungarian, Czech and Croatian, resolved no category at all, so search hid 70
// of its 82. Each case is a live title from that board or from the wider CEE sample the
// vocabulary was measured against.
func TestParse_CentralEasternEuropeTitles(t *testing.T) {
	cases := []struct{ title, want string }{
		// Slovenian. The desk is in the locative ("v Proizvodnji", "v Marketingu").
		{"KUHAR v Družbeni prehrani", "hospitality"},
		{"KUHARSKI POMOČNIK v Družbeni prehrani", "hospitality"},
		{"ASISTENT KUHARJA v Družbeni prehrani", "hospitality"},
		{"FARMACEVTSKI DELAVEC v Proizvodnji zdravil", "manufacturing"},
		{"UPRAVLJALEC STROJEV v Proizvodnji zdravil", "manufacturing"},
		{"PROCESNIČAR v Proizvodnji učinkovin", "manufacturing"},
		{"TEHNIK VZDRŽEVANJA v Tehnični službi v Proizvodnji zdravil", "skilled_trades"},
		{"MEDICINSKI SVETOVALEC v Produktnem marketingu Farmacevtike", "marketing"},
		{"POMOČNIK ZA BLAGOVNE ZNAMKE za zdravila na recept v Marketingu", "marketing"},
		{"SAMOSTOJNI STROKOVNI SODELAVEC v Kontrolingu, Korporativna ekonomika", "finance"},
		{"SAMOSTOJNI STROKOVNI SODELAVEC v Mednarodnem računovodstvu", "finance"},
		{"ZDRAVNIK SPECIALIST FIZIKALNE IN REHABILITACIJSKE MEDICINE v Šmarjeških Toplicah", "healthcare"},
		{"RECEPTOR VARNOSTNIK v Oddelku varovanja", "public_safety"},
		// The water rescuer is the lifeguard, not the ambulance crew.
		{"REŠEVALEC IZ VODE v Šmarjeških Toplicah", "public_safety"},
		{"Prodajalec (m/ž)", "retail"},
		// Polish. Both genders, and the desk in the genitive.
		{"Medical Representatives (m/w/d)", "sales"},
		{"Pharmacy Representatives (m/w/d)", "sales"},
		{"Przedstawicielka farmaceutyczna/ Przedstawiciel farmaceutyczny", "sales"},
		{"Przedstawicielka medyczna/ Przedstawiciel medyczny", "sales"},
		{"Przedstawiciel/Przedstawicielka naukowy/-a w dziale promocji produktów weterynaryjnych", "marketing"},
		{"Młodszy specjalista/ Młodsza Specjalistka ds. sprzedaży", "sales"},
		{"Specjalista ds. personalnych (obszar rekrutacji)", "hr"},
		{"Kierownik/Kierowniczka Działu HR", "hr"},
		{"Młodsza Księgowa/ Młodszy Księgowy", "finance"},
		{"Zastępca/ Zastępczyni Głównej Księgowej", "finance"},
		{"Asystent/Asystentka ds. Obsługi Floty", "logistics"},
		{"Specjalista/-tka ds. gospodarki magazynowej", "logistics"},
		{"Pracownik produkcji", "manufacturing"},
		{"TECHNIK ANALITYK DS. KONTROLI JAKOŚCI", "science"},
		{"Asystent/-tka ds. administracji", "administration"},
		{"Mechanik/ Mechaniczka - konserwator/konserwatorka", "skilled_trades"},
		{"Lekarz Dermatolog / Lekarka Dermatolożka", "healthcare"},
		{"Pielęgniarz / Pielęgniarka (K/M/N)", "healthcare"},
		// The role noun outranks the desk wherever it sits in the title.
		{"Magazynier/Magazynierka - obsługa produkcji", "logistics"},
		{"Specjalista ds. rekrutacji (k/m)", "recruiting"},
		{"Analityk Systemowy", "business_analysis"},
		{"Programista Java (k/m)", "software_engineering"},
		// Czech and Slovak, including the register's plural occupation groups.
		{"MEDICÍNSKÝ REPREZENTANT - RX PORTFOLIO - PRAHA", "sales"},
		{"Regionální zástupce veterinárních léčiv", "sales"},
		{"Vývojáři softwaru, Vývojáři softwaru", "software_engineering"},
		{"Vývojári a analytici softvéru a aplikácií inde neuvedení", "software_engineering"},
		{"Systémoví analytici", "business_analysis"},
		{"Specialisté v oblasti testování softwaru a příbuzní pracovníci", "qa"},
		{"Špecialisti v oblasti databáz a počítačových sietí inde neuvedení", "network_engineering"},
		{"Systémoví administrátoři, správci počítačových sítí", "devops"},
		{"Operátor výroby", "manufacturing"},
		{"Skladník", "logistics"},
		// Croatian.
		{"STRUČNI SURADNIK U MARKETINGU (m/ž)", "marketing"},
		{"RADNIK U FARMACEUTSKOJ PROIZVODNJI (m/ž)", "manufacturing"},
		{"Skladištar (m/ž)", "logistics"},
		{"Konobar (m/ž)", "hospitality"},
		{"CNC OPERATER/OPERATORICA", "manufacturing"},
		// Hungarian.
		{"KLINIKAI KUTATÁSI MUNKATÁRS", "healthcare"},
		{"ORVOSLÁTOGATÓ (REGIONÁLISAN MAX. 2 MEGYE)", "sales"},
		{"PATIKALÁTOGATÓ (FEJÉR/SOMOGY/VESZPRÉM/PEST VÁRMEGYÉK)", "sales"},
		{"Raktáros", "logistics"},
		{"Könyvelő", "finance"},
		// Ukrainian and Bulgarian.
		{"Медичний представник РХ, Львів", "sales"},
		{"Медицински представител - град София и област София", "sales"},
		{"Фахівець IT підтримки", "it_support"},
		{"Фахівець з діджитал маркетингу", "marketing"},
		{"Асистент відділу маркетингу", "marketing"},
		{"Сътрудник Продажби", "sales"},
		{"Тест Інженер", "qa"},
		{"Системний адміністратор", "devops"},
		{"Адміністратор баз даних (Oracle)", "devops"},
		// A bank's cashier is the teller.
		{"Менеджер - касир відділення банку", "finance"},
		// The CNC programmer is a production occupation (SOC 51-9162).
		{"Programista CNC", "manufacturing"},
		{"CNC PROGRAMER/PROGRAMERKA (CAD/CAM)", "manufacturing"},
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.want {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.want)
		}
	}
}

// TestParse_CentralEasternEuropeUnresolved pins what the CEE vocabulary deliberately leaves
// alone: the talent pools and open applications name no role, student work and internships
// stay as unresolved as their English twins, and the generic "specialist"/"associate" nouns
// and the bare quality, compliance and tax desks resolve nothing, as they do in English.
// The PLC programmers are pinned ahead of the bare programmer noun.
func TestParse_CentralEasternEuropeUnresolved(t *testing.T) {
	for _, title := range []string{
		"SPLOŠNA BAZA KANDIDATOV ZA ZAPOSLITEV",
		"SPLOŠNA BAZA KANDIDATOV ZA ŠTUDENTSKO DELO",
		"ŠTUDENTSKO DELO v Razvoju in raziskavah zdravil",
		"ОБЩА БАЗА ДАННИ НА КАНДИДАТИТЕ ЗА РАБОТА",
		"База кандидатів",
		"Initiativbewerbung",
		"Otvorena molba",
		"Zgłoszenia do przyszłych rekrutacji",
		"Specjalista ds. Jakości",
		"Specjalista / Specjalistka ds. Zgodności",
		"SAMOSTOJNI STROKOVNI SODELAVEC v Kakovosti v obratu Ljutomer, Upravljanje kakovosti",
		"Фахівець з цифрової трансформації, Київ",
		"PLC programozó",
		"Programer PLC",
		"Elektro programer",
		// No bare engineer noun, as in English ("Engineer" alone resolves nothing).
		"Inženir",
		"Inżynier",
		"Inženjer",
		"Projektni inženir",
		"Szivattyú tesztelő",
	} {
		if got := Parse(title).Category; got != "" {
			t.Errorf("Parse(%q).Category = %q, want unresolved", title, got)
		}
	}
}

// TestParse_EngineerAndProgrammerShadowing pins freehire#3518: a bare "inženir" filed every
// Slovenian engineer with the plant and a bare "programer" every machine programmer with
// software, because the bare noun was the only word the table knew in those titles. The
// discipline beside the noun decides, and the bare programmer noun is read last.
func TestParse_EngineerAndProgrammerShadowing(t *testing.T) {
	cases := []struct{ title, want string }{
		{"Java programski inženir", "software_engineering"},
		{"Inženir za razvoj programske opreme", "software_engineering"},
		{"Inženir vgrajene programske opreme", "embedded"},
		{"Inženir za kibernetsko varnost", "security"},
		{"Inženir varnosti informacijskih sistemov", "security"},
		{"Sistemski inženir", "software_engineering"},
		{"SISTEMSKI INŽENIR III - M/Ž", "software_engineering"},
		{"Radiološki inženir", "healthcare"},
		{"Prodajni inženir", "solutions_engineering"},
		{"Terenski prodajni inženir", "solutions_engineering"},
		{"Varnostni inženir", "occupational_safety"},
		{"Testni inženir", "qa"},
		{"Procesni inženir", "industrial_engineering"},
		{"Višji strojni inženir (m/ž)", "industrial_engineering"},
		{"INŽENJER/KA ZA RAZVOJ SOFTVERA", "software_engineering"},
		{"INŽENJER PRODAJE (M/Ž)", "solutions_engineering"},
		{"Procesni inženjer/ka", "industrial_engineering"},
		{"Inżynier jakości (K/M)", "industrial_engineering"},
		{"Procesní inženýr/ka", "industrial_engineering"},
		{"CNC operater - programer", "manufacturing"},
		{"CNC operater in programer", "manufacturing"},
		{"Programer strojev in naprav", "manufacturing"},
		{"CNC operátor/programátor (m/ž)", "manufacturing"},
		{"TEHNOLOG/TEHNOLOGINJA - PROGRAMER/PROGRAMERKA U STROJNOJ OBRADI", "industrial_engineering"},
		// The bare programmer noun is still software once nothing else speaks.
		{"PROGRAMER/PROGRAMERKA", "software_engineering"},
		{"Programista Java (k/m)", "software_engineering"},
		{"Python розробник", "software_engineering"},
		// From the same contribution: the curated Slovenian vocabulary.
		{"Vzdrževalec perila", "personal_services"},
		{"Keramičar (m/ž)", "skilled_trades"},
		{"Natakarski pomočnik", "hospitality"},
		{"Diplomirana medicinska sestra", "healthcare"},
		{"Skladiščni delavec", "logistics"},
		{"Komercialist", "sales"},
		{"Receptor", "administration"},
	}
	for _, c := range cases {
		if got := Parse(c.title).Category; got != c.want {
			t.Errorf("Parse(%q).Category = %q, want %q", c.title, got, c.want)
		}
	}
}

// TestParse_SlovenianCatchAllNounsStayUnresolved pins the curation of the #3518 list: the
// generic nouns it filed as a catch-all ("vodja" as operations, "delavec" as manufacturing)
// name no craft, and stay as unresolved as their English twins.
func TestParse_SlovenianCatchAllNounsStayUnresolved(t *testing.T) {
	for _, title := range []string{
		"Vodja", "Specialist", "Sodelavec", "Svetovalec", "Pomočnik", "Delavec", "Operater",
		"Referent", "Asistent", "Strokovni sodelavec", "Tehnik", "Koordinator",
		"Vodja oddelka", "Študentsko delo",
	} {
		if got := Parse(title).Category; got != "" {
			t.Errorf("Parse(%q).Category = %q, want unresolved", title, got)
		}
	}
}
