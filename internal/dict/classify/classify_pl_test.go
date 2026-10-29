package classify

import (
	"strings"
	"testing"
)

// TestPolishHeadsAndFields pins the Polish title vocabulary. A Polish title names a
// head and the field it works in, inflected for case and gender ("Specjalista ds.
// sprzedaży", "Kierowniczka Projektu"), so a role noun or a field resolves in any of its
// forms and a bare generic head does not. Every title here is a live posting from the
// Polish crawl (erecruiter, traffit, nofluffjobs, solidjobs, smartrecruiters,
// teamtailor, recruitee, eures), measured 2026-10-29.
func TestPolishHeadsAndFields(t *testing.T) {
	cases := []struct {
		title, category, seniority string
	}{
		// The role nouns, in both genders and in the plural.
		{"Higienistka Stomatologiczna / Higienista Stomatologiczny - CS Wilanów", "healthcare", ""},
		{"Asystent/Asystentka stomatologiczna", "healthcare", ""},
		{"Psycholog/Psycholożka- Wrocław", "healthcare", ""},
		{"Elektroradiolog / Elektroradiolożka", "healthcare", ""},
		{"Lekarki / Lekarze specjaliści lub w trakcie specjalizacji / Katowice", "healthcare", ""},
		{"Pracownik Medyczny (k/m)", "healthcare", ""},
		{"Kurierzy / Kurierki / Przewoźnicy / Przewoźniczki", "logistics", ""},
		{"Przedstawiciel/ka handlowy/a", "sales", ""},
		{"Kupiec Operacyjny (K/M)", "operations", ""},
		{"Tokarz manualny (K/M)", "skilled_trades", ""},
		{"Frezer CNC (k/m)", "skilled_trades", ""},
		{"Lakiernik samochodowy (K/M)", "skilled_trades", ""},
		{"Instruktor / Instruktorka Sportów Walki", "education", ""},
		{"Radczyni Prawna/Radca Prawny", "legal", ""},
		{"Ratownik / Ratowniczka na basenie", "public_safety", ""},
		// The heads, by their field.
		{"Architekt Hurtowni Danych (100% zdalnie)", "architecture", ""},
		{"Architekt bezpieczeństwa aplikacji IT", "security", ""},
		{"Analityk Danych (SQL, BI)", "data_analytics", ""},
		{"Analityk Biznesowo- Systemowy", "business_analysis", ""},
		{"Administrator/-ka infrastruktury", "devops", ""},
		{"Inżynier(-ka) Sieciowy(-a) w Dziale Usług IT", "network_engineering", ""},
		{"Inżynier(-ka) Systemowy(-a) w Dziale Usług IT", "devops", ""},
		{"Specjalista ds. bezpieczeństwa informacji (K/M)", "security", ""},
		{"Specjalista ds. IT (k/m)", "it_support", ""},
		{"Młodsza Specjalistka / Młodszy Specjalista ds. Wsparcia Technicznego", "it_support", "junior"},
		{"Specjalistka / Specjalista ds. Testów Automatycznych", "qa", ""},
		{"Kierownik Projektu IT (K/M)", "project_management", ""},
		{"Konsultant/ka Wdrożeniowy/a ERP", "customer_success", ""},
		{"Konsultant(ka) SAP S/4HANA Group Reporting", "solutions_engineering", ""},
		{"Doradczyni/Doradca klienta korporacyjnego", "sales", ""},
		{"Doradczyni/Doradca klientów ds. leasingu", "finance", ""},
		{"St. specjalistka / St. specjalista ds. budowania relacji z klientem zamożnym", "finance", "senior"},
		{"Specjalista/Specjalistka ds. Kredytów Hipotecznych", "finance", ""},
		{"Opiekun / Opiekunka Klubowicza – Well Fitness", "support", ""},
		{"Opiekun / Opiekunka Medyczna", "healthcare", ""},
		{"Zastępca/Zastępczyni Managera/Managerki Sklepu", "retail", ""},
		{"Wsparcie Sklepu (umowa-zlecenie)", "retail", ""},
		{"Dowódca Zmiany (K/M) Piaseczno", "public_safety", ""},
		{"Ochrona / magazyn - Pruszków (k/m)", "public_safety", ""},
		{"Praca od zaraz! Sprzątanie w galerii handlowej - Gdańsk-Matarnia", "personal_services", ""},
		{"Operator/ Operatorka Sprzętu (Walec/ Rozściełacz) - roboty bitumiczne", "skilled_trades", ""},
		{"Operator obrabiarek CNC (K/M)", "manufacturing", ""},
		{"Operator/ Operatorka wózka jezdniowego Logistyka", "logistics", ""},
		{"Inżynier / Inżynierka Serwisu Mobilnego", "industrial_engineering", ""},
		{"Asystent/ka Zarządu (K/M)", "administration", ""},
		// The order of the bands: a role noun outranks the field it shares a title with.
		{"Elektroradiolog / Elektroradiolożka z obsługą recepcji - Dental Sense", "healthcare", ""},
		{"Ochrona danych osobowych - Główny specjalista ds. ochrony danych osobowych (m/k)", "legal", ""},
		{"Specjalista ds. Ochrony Przeciwpożarowej, Dział Bezpieczeństwa i Ochrony", "public_safety", ""},
		// A perk and a client's sector are not the work.
		{"Pracownica / Pracownik Obsługi Zwrotów Odzieży – darmowy transport Zielona Góra", "", ""},
		{"Ekspert ds. Analiz Sektorowych – branża produkcyjna (k/m)", "", ""},
		// "Systemów" is the genitive of the systems an engineer designs, not the
		// sysadmin's "systemowy".
		{"Inżynier Systemów Hydraulicznych (współpraca z GE Vernova)", "", ""},
		// The bare generic heads resolve nothing.
		{"Specjalista ds. Technicznych", "", ""},
		{"Koordynator/Koordynatorka Zespołu Operacji", "", ""},
		{"Rekrutacja spontaniczna (wszystkie obszary)", "", ""},

		// The grades.
		{"Mł. specjalista / specjalistka ds. urządzeń mobilnych", "", "junior"},
		{"Młodszy/a Operator/ka Maszyn (k/m)", "manufacturing", "junior"},
		{"Starsza Specjalistka / Starszy Specjalista ds. Bezpieczeństwa ICT", "security", "senior"},
		{"Specjalista/Starszy Specjalista ds. kontroli ICT", "", "senior"},
		{"Specjalista ds. konfiguracji Eximee Regular", "", "middle"},
		{"Stażystka/Stażysta w obszarze ryzyka kredytowego", "finance", "intern"},
		{"Płatny Staż w Dziale Technicznym (k/m)", "", "intern"},
		{"Praktykant/ka (branża mostowa)", "", "intern"},
		{"Kierownik / Kierowniczka Zespołu Innowacji", "", "lead"},
		{"Lider / Liderka zespołu monitoringu IT", "", "lead"},
		{"Lider Linii", "", "lead"},
		{"Prezes Zarządu", "", "c_level"},
		{"Dyrektor Finansowy", "finance", "c_level"},
		// A plain director, a deputy and an assistant state no grade; "ML" is machine
		// learning, not "Mł."; a programme desk runs the internships.
		{"Dyrektor Regionalny Ogród", "", ""},
		{"Zastępca Dyrektora Generalnego", "", ""},
		{"Asystent kierownika zespołu", "", ""},
		{"Koordynator ds. staży i praktyk", "", ""},
		{"Inżynier ML", "", ""},
		// An elderly person's age is not the grade.
		{"Opiekunka osoby starszej (k/m)", "healthcare", ""},
		{"Opiekun / Opiekunka osób starszych - Niemcy", "healthcare", ""},
		{"Opieka nad starszą panią w Warszawie", "", ""},
	}
	for _, c := range cases {
		got := Parse(c.title)
		if got.Category != c.category || got.Seniority != c.seniority {
			t.Errorf("Parse(%q) = {%q, %q}, want {%q, %q}", c.title, got.Category, got.Seniority, c.category, c.seniority)
		}
	}
}

// TestPolishReadsOnlyPolishTitles pins the guard that keeps the Polish stems off other
// languages: "operator", "administrator", "architekt" and "(m/f/d)" are English and
// German too, and a title the earlier tables left unresolved stays unresolved.
func TestPolishReadsOnlyPolishTitles(t *testing.T) {
	cases := []struct {
		title, category, seniority string
	}{
		{"Overnight Shift - $31/hr - Autonomous Vehicle Operator (Testing and Quality Control)", "", ""},
		{"Transport Administrator", "", ""},
		{"HR Analytics & Reporting Specialist (m/f/d)", "", ""},
		{"Regular Full Time Associate", "", ""},
		{"St. Louis Area Coordinator", "", ""},
	}
	for _, c := range cases {
		got := Parse(c.title)
		if got.Category != c.category || got.Seniority != c.seniority {
			t.Errorf("Parse(%q) = {%q, %q}, want {%q, %q}", c.title, got.Category, got.Seniority, c.category, c.seniority)
		}
	}
}

// TestPolishAliasesAreFolded holds every Polish alias to the space foldPolish maps a
// title into: lowercase, no diacritics, single spaces. An alias the fold would rewrite
// could never match.
func TestPolishAliasesAreFolded(t *testing.T) {
	for _, table := range [][]aliasEntry{polishCategoryTable, polishSeniorityTable} {
		for _, e := range table {
			plain := strings.ReplaceAll(e.alias, "*", "")
			if f := strings.Join(polishWords(foldPolish(plain)), " "); f != plain {
				t.Errorf("alias %q folds to %q", e.alias, f)
			}
		}
	}
}

// TestDataWarehouseIsNotAWarehouse pins the analytics store against the building.
func TestDataWarehouseIsNotAWarehouse(t *testing.T) {
	for title, want := range map[string]string{
		"Data Warehouse Developer":          "data_engineering",
		"Senior Data Warehouse Engineer":    "data_engineering",
		"Architekt IT – Data Warehouse":     "data_engineering",
		"Data Warehouse Architect":          "architecture",
		"Warehouse Associate":               "logistics",
		"Analityk IT / BI – Data Warehouse": "data_engineering",
	} {
		if got := Parse(title).Category; got != want {
			t.Errorf("Parse(%q).Category = %q, want %q", title, got, want)
		}
	}
}
