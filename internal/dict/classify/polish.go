package classify

import (
	"regexp"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Polish titles name a HEAD and the FIELD it works in, as Spanish and French ones do —
// "Specjalista ds. sprzedaży", "Kierownik Działu Logistyki", "Inżynier serwisu",
// "Opiekun Klienta" — but Polish INFLECTS, so the head and the field reach a title in
// whichever case and gender the phrase puts them: a programista, programistka,
// programisty and programistów are one word, and the field after "ds." stands in the
// genitive ("sprzedaży", "logistyki", "księgowości"). The main table carries a block of
// Polish surface forms (the CEE block, freehire#3513) and they hold what they list,
// but measured 2026-10-29 over 5,096 open postings with a Polish title (erecruiter,
// traffit, nofluffjobs, solidjobs, smartrecruiters, teamtailor, recruitee, eures, read
// per source with `countries @> {pl}` where the source spans countries), 35.5% still
// resolved no category and 95.6% no grade, and the unresolved titles were the same few
// dozen stems in cases and genders the block did not spell.
//
// So the Polish tables match a STEM. Every alias is a sequence of whole words on the
// folded title (foldPolish: lowercased, the diacritics dropped, "ł" → "l"); a word
// written with a trailing "*" matches any word that STARTS with it — Polish inflects
// at the end of the word, where German compounds put their head at the start. A
// multi-word alias matches consecutive words: "specjalist* ds sprzedaz*". A word
// without the "*" matches exactly ("ds", "it", "hr").
//
// The table is ordered in the Spanish bands, and table order is the precedence:
//
//  1. a head and field whose answer differs from the field's default, and the vetoes
//     ("Programista CNC" runs a machine tool, "Opiekun Klienta" keeps an account,
//     "Ochrona danych osobowych" is data protection, not the guard);
//  2. the role nouns that name a trade by themselves (pielęgniarka, magazynier,
//     handlowiec, księgowa, kurier, tokarz);
//  3. the fields themselves (sprzedaży, logistyki, rekrutacji, kadr i płac), which give
//     a generic head its category.
//
// A generic head with no field — specjalista, kierownik, koordynator, ekspert,
// asystent, konsultant, doradca, inżynier, operator, referent, inspektor — resolves
// nothing, as "Specialist" and "Manager" resolve nothing in the main table. Each
// answer files the role the way its English twin is filed: "Relationship Manager" is
// finance, so the bank's "Specjalista ds. relacji z klientem" is too; "Implementation
// Consultant" is customer success, "SAP Consultant" solutions engineering.
//
// The table is a FALLBACK: it runs only after the main, German, Dutch, Danish,
// Spanish, Portuguese and French tables resolved nothing, and only on a title that
// looks Polish (looksPolish), because a stem is a loose match — "operator", "tester",
// "administrator", "architekt" begin English, German and Czech words too. Before it,
// the main table is asked once more of the gender-cut title: "Inżynier(-ka)
// Sieciowy(-a)" and "Specjalist(k)a" hide an alias behind the notice.
var polishCategoryTable = []aliasEntry{
	// Band 1: vetoes and the heads whose field is not their answer.

	// Not a posting: a talent pool, a referral programme, a "send us your CV".
	{"aplikacj* spontaniczn*", categoryNone},
	{"rekrutacj* spontaniczn*", categoryNone},
	{"program* polecen*", categoryNone},
	{"przyszl* rekrutac*", categoryNone},
	// The machine shop's programmer, before the software one, as the main table pins it.
	{"programist* cnc", "manufacturing"},
	{"programist* plc", categoryNone},
	{"programist* robot*", categoryNone},
	{"programist* sterownik*", categoryNone},
	{"technolog* programist*", "industrial_engineering"},
	// Data protection is the lawyer's, and fire protection the guard's (the French
	// "sécurité incendie"), before the guard's own "ochrona" below.
	{"ochron* danych", "legal"},
	{"ochron* przeciwpozarow*", "public_safety"},
	{"ppoz", "public_safety"},
	{"ochron* srodowisk*", categoryNone},
	// Information security, before the guard's physical "bezpieczeństwo".
	{"bezpieczenstw* informacj*", "security"},
	{"bezpieczenstw* it", "security"},
	{"bezpieczenstw* ict", "security"},
	{"bezpieczenstw* ot", "security"},
	{"bezpieczenstw* aplikacj*", "security"},
	{"bezpieczenstw* system*", "security"},
	{"bezpieczenstw* sieci*", "security"},
	{"bezpieczenstw* architektur*", "security"},
	{"bezpieczenstw* teleinformatyczn*", "security"},
	{"bezpieczenstw* cyfrow*", "security"},
	{"cyberbezpieczenstw*", "security"},
	{"zarzadzani* podatnosci*", "security"},
	{"architekt* bezpieczenstw*", "security"},
	{"audytor* it", "security"},
	{"bezpieczenstw* fizyczn*", "public_safety"},
	{"bezpieczenstw* obiekt*", "public_safety"},
	// The bank's and the insurer's relationship desk ("Relationship Manager" is finance).
	{"budowani* relacj*", "finance"},
	{"relacj* z klient* zamozn*", "finance"},
	{"relacj* z klient* firmow*", "finance"},
	{"relacj* z klient* premium", "finance"},
	{"relacj* z klient* korporacyjn*", "finance"},
	{"relacji i wsparci* klient* hipoteczn*", "finance"},
	// The account keeper is sales, the gym's member desk is customer service, the
	// medical carer is a carer.
	{"opiekun* klient* b2b", "sales"},
	{"opiekun* klubowicz*", "support"},
	{"opiekun* medyczn*", "healthcare"},
	{"opiekun* osob* starsz*", "healthcare"},
	{"opiekun* pacjent*", "healthcare"},
	{"opiekun* magazyn*", "logistics"},
	// The advisor is whatever he advises on: the corporate and business client's banker
	// sells; a mortgage, a loan or a lease is finance's ("Mortgage Advisor").
	{"doradc* klient* korporacyjn*", "sales"},
	{"doradc* klient* biznesow*", "sales"},
	{"doradczyn* klient* korporacyjn*", "sales"},
	{"doradczyn* klient* biznesow*", "sales"},
	{"doradc* ds klient* biznesow*", "sales"},
	{"doradczyn* ds klient* biznesow*", "sales"},
	{"doradc* techniczno-handlow*", "sales"},
	{"doradc* techniczno handlow*", "sales"},
	{"doradc* ds ubezpieczen*", "sales"},
	{"doradc* ds leasing*", "finance"},
	{"doradc* leasingow*", "finance"},
	{"doradc* klient* ds leasing*", "finance"},
	{"doradc* kredytow*", "finance"},
	{"partner* biznesow* ds klient*", "sales"},
	{"ds msp", "sales"},
	// The implementation consultant is customer success; the ERP/SAP one is solutions
	// engineering, as "SAP Consultant" is.
	{"konsultant* wdrozeniow*", "customer_success"},
	{"konsultant* ds wdrozen*", "customer_success"},
	{"konsultant* sap", "solutions_engineering"},
	{"konsultant* erp", "solutions_engineering"},
	{"konsultant* ds system* erp", "solutions_engineering"},
	{"konsultant* ifs", "solutions_engineering"},
	{"konsultant* ds dynamics", "solutions_engineering"},
	{"konsultanci i developerzy sap", "solutions_engineering"},
	{"wdrozen* system* erp", "customer_success"},
	{"wdrozen* erp", "customer_success"},
	{"konsultant* w zespol* obslug* telefoniczn*", "support"},
	{"konsultant* telefoniczn*", "support"}, // the call centre ("Call Center Agent")
	// The IT service engineer and the helpdesk, before the plain service engineer.
	{"serwis* it", "it_support"},
	{"specjalist* ds it", "it_support"}, // "IT Specialist"
	{"specjalist* it", "it_support"},
	{"wsparci* it", "it_support"},
	{"wsparci* techniczn*", "it_support"},
	{"wsparci* uzytkownik*", "it_support"},
	{"obslug* informatyczn*", "it_support"},
	{"urzadzen* drukujac*", "it_support"},
	{"sprzet* it", "it_support"},
	// Software testing; a laboratory's reliability or pressure "testy" are not QA.
	{"tester*", "qa"},
	{"testow* automatyczn*", "qa"},
	{"testow* wydajnosciow*", "qa"},
	{"testow* manualn*", "qa"},
	{"testow* oprogramowani*", "qa"},
	{"koordynator* testow", "qa"},
	{"lider* testow", "qa"},
	{"specjalist* ds testow", "qa"},
	// The architects: the security one above, the software and systems ones here, and
	// "Architect" alone is architecture in the main table too.
	{"architekt* wnetrz*", "design"},
	{"architektk* wnetrz*", "design"},
	{"architekt*", "architecture"},
	// The analysts, by their field (a bare "Analyst" resolves nothing).
	{"analityk* danych", "data_analytics"},
	{"analityczk* danych", "data_analytics"},
	{"analityk* hurtown* danych", "data_analytics"},
	{"analityk* bi", "data_analytics"},
	{"analityk* sas", "data_analytics"},
	{"analityczk* sas", "data_analytics"},
	{"analiz* danych", "data_analytics"},
	{"analityk* biznesow*", "business_analysis"},
	{"analityk* system*", "business_analysis"},
	{"analityczk* system*", "business_analysis"},
	{"analiz* biznesow*", "business_analysis"},
	{"analityk* it", "software_engineering"}, // "IT Analyst"
	{"analityczk* it", "software_engineering"},
	{"analityk* laboratoryjn*", "science"},
	{"analityk* finansow*", "finance"},
	{"hurtown* danych", "data_engineering"},
	// The system and network administrators ("System Administrator" is devops,
	// "Network Administrator" network engineering).
	{"administrator* sieci*", "network_engineering"},
	{"administrator* infrastruktur*", "devops"},
	{"administrator* system*", "devops"},
	{"administratork* system*", "devops"},
	{"administrator* it", "devops"},
	{"administrator* middleware", "devops"},
	{"administrowani* system*", "devops"},
	{"administrator* baz* danych", "devops"},
	{"administrator* aplikacj*", "devops"},
	{"inzynier* sieciow*", "network_engineering"},
	{"inzynier* systemowy", "devops"},
	{"inzynierk* systemowa", "devops"},
	{"siec* informatyczn*", "network_engineering"},
	{"sieci komputerowych", "network_engineering"},
	{"sieci teleinformatycznych", "network_engineering"},
	{"telekomunikacj*", "network_engineering"},
	{"inzynier* danych", "data_engineering"},
	{"inzynierk* danych", "data_engineering"},
	// The deputy-manager seats of a store and a club are the store's and the club's.
	{"kierownik* sklep*", "retail"},
	{"kierowniczk* sklep*", "retail"},
	{"managerk* sklep*", "retail"},
	{"manager* sklep*", "retail"},
	// The diagnostics laboratory is a medical one; the medical laboratory is healthcare.
	{"diagnostyk* laboratoryjn*", "healthcare"},
	{"laboratori* medyczn*", "healthcare"},
	{"asystent* stomatologiczn*", "healthcare"},
	{"asystentk* stomatologiczn*", "healthcare"},
	{"technik* dentystyczn*", "healthcare"},
	{"techniczk* dentystyczn*", "healthcare"},
	{"technik* farmaceutyczn*", "healthcare"},
	{"techniczk* farmaceutyczn*", "healthcare"},
	{"sterylizacj* medyczn*", "healthcare"},
	{"pracownik* medyczn*", "healthcare"},
	{"ratownik* medyczn*", "healthcare"},
	{"ratownik*", "public_safety"}, // the lifeguard ("Lifeguard" is public safety)
	{"ratowniczk*", "public_safety"},
	// The project desks.
	{"asystent* kierownik* projekt*", "project_management"},
	{"kierownik* projekt*", "project_management"},
	{"kierowniczk* projekt*", "project_management"},
	{"kierownik* ds projekt*", "project_management"},
	{"menedzer* projekt*", "project_management"},
	{"menadzer* projekt*", "project_management"},
	{"koordynator* projekt*", "project_management"},
	{"prowadzeni* projekt*", "project_management"},
	{"pmo", "project_management"},
	// The guard: the security firm's "ochrona", its shift commander and its patrol.
	{"ochrona", "public_safety"},
	{"w ochronie", "public_safety"},
	{"pracownik* ochrony", "public_safety"},
	{"pracownik* patrolu", "public_safety"},
	{"patrol* interwencyjn*", "public_safety"},
	{"dowodc* zmiany", "public_safety"},
	{"operator* system* monitoringu", "public_safety"},
	{"operator* monitoringu", "public_safety"},
	// The heavy plant and the machine shop ("Heavy Equipment Operator", "Machinist").
	{"operator* koparki", "skilled_trades"},
	{"operator* ladowark*", "skilled_trades"},
	{"operator* zurawia", "skilled_trades"},
	{"operator* sprzetu", "skilled_trades"},
	{"operator* wezla", "skilled_trades"},
	{"operator* wozka", "logistics"},
	{"operator* cnc", "manufacturing"},
	{"operator* obrabiar*", "manufacturing"},
	{"operator* maszyn*", "manufacturing"},
	{"operator* linii", "manufacturing"},
	{"operator* prasy", "manufacturing"},
	{"operator* lasera", "manufacturing"},
	{"operator* robotow", "manufacturing"},
	{"operator* lutowania", "manufacturing"},
	{"operator* centrow", "manufacturing"},
	{"operator* procesow skrawania", "manufacturing"},
	{"operator* dlutownic*", "manufacturing"},
	{"operator* tokar*", "manufacturing"},
	// Service and the field: the service engineer is industrial engineering, the
	// service technician a trade ("Service Engineer", "Service Technician").
	{"inzynier* serwis*", "industrial_engineering"},
	{"inzynierk* serwis*", "industrial_engineering"},
	{"technik* serwis*", "skilled_trades"},
	{"techniczk* serwis*", "skilled_trades"},
	{"inzynier* jakosci", "industrial_engineering"},
	{"inzynierk* jakosci", "industrial_engineering"},
	{"inspektor* kontroli jakosci", "manufacturing"},
	{"inspektor* jakosci", "manufacturing"},
	{"inspektork* jakosci", "manufacturing"},
	{"inzynier* procesu", "industrial_engineering"},
	{"inzynierk* procesu", "industrial_engineering"},
	{"inzynier* presales", "solutions_engineering"},
	{"inzynierk* presales", "solutions_engineering"},
	{"kosztorysant*", "industrial_engineering"}, // "Estimator"
	{"geotechnik*", "industrial_engineering"},
	{"utrzymani* ruchu", "skilled_trades"},
	{"utrzymani* obiektu", "skilled_trades"},
	{"technik* obiektu", "skilled_trades"},
	{"robot* drogow*", "skilled_trades"},
	{"robot* wodno-kanalizacyjn*", "skilled_trades"},
	{"budownictw*", "skilled_trades"},
	{"budowlan*", "skilled_trades"},
	// The cleaner and the janitor.
	{"utrzymani* czystosci", "personal_services"},
	{"sprzatani*", "personal_services"},
	{"sprzatajac*", "personal_services"},
	{"pracownik* gospodarcz*", "personal_services"},
	// The office: the board's and the office's assistant, the archive, the front desk.
	{"asystent* zarzad*", "administration"},
	{"asystentk* zarzad*", "administration"},
	{"asystent* biur*", "administration"},
	{"asystentk* biur*", "administration"},
	{"biur* zarzad*", "administration"},
	{"obslug* zarzad*", "administration"},
	{"wsparci* zarzad*", "administration"},
	{"obslug* organow", "administration"},
	{"pracownik* biurow*", "administration"},
	{"pracownik* administracyjn*", "administration"},
	{"pracowniczk* administracyjn*", "administration"},
	{"wsparci* administracyjn*", "administration"},
	{"ds administracyjn*", "administration"},
	// The lawyer and the legal department.
	{"radca prawn*", "legal"},
	{"radczyn* prawn*", "legal"},
	{"prawnik*", "legal"},

	// Band 2: the role nouns that name a trade by themselves.
	{"programist*", "software_engineering"},
	{"programisc*", "software_engineering"},
	{"developer* hurtown*", "data_engineering"},
	{"pielegniar*", "healthcare"},
	{"lekar*", "healthcare"},
	{"chirurg*", "healthcare"},
	{"chirurzk*", "healthcare"},
	{"psychiatr*", "healthcare"},
	{"psycholog*", "healthcare"},
	{"psycholozk*", "healthcare"},
	{"neuropsycholog*", "healthcare"},
	{"neuropsycholozk*", "healthcare"},
	{"psychoterapeut*", "healthcare"},
	{"fizjoterapeut*", "healthcare"},
	{"fizjoterapi*", "healthcare"},
	{"elektroradiolog*", "healthcare"},
	{"elektroradiolozk*", "healthcare"},
	{"higienist*", "healthcare"},
	{"stomatolog*", "healthcare"},
	{"optometryst*", "healthcare"},
	{"podolog*", "healthcare"},
	{"podolozk*", "healthcare"},
	{"audiofonolog*", "healthcare"},
	{"audiofonolozk*", "healthcare"},
	{"logoped*", "healthcare"},
	{"neurologoped*", "healthcare"},
	{"protetyk* sluchu", "healthcare"},
	{"protetyczk* sluchu", "healthcare"},
	{"farmaceut*", "healthcare"},
	{"magister farmacji", "healthcare"},
	{"magistra farmacji", "healthcare"},
	{"epidemiologi*", "healthcare"},
	{"ksiegow*", "finance"},
	{"handlowiec", "sales"},
	{"handlowc*", "sales"},
	{"przedstawiciel* handlow*", "sales"},
	{"przedstawicielk* handlow*", "sales"},
	{"sprzedawc*", "retail"},
	{"sprzedawczyn*", "retail"},
	{"kasjer*", "retail"},
	{"magazynier*", "logistics"},
	{"kierowc*", "logistics"},
	{"kurier*", "logistics"},
	{"przewoznik*", "logistics"},
	{"przewoznic*", "logistics"},
	{"dyspozytor", "logistics"},
	{"dyspozytorka", "logistics"},
	{"kupiec*", "operations"}, // the buyer ("Buyer")
	{"kupc*", "operations"},
	{"rekruter*", "recruiting"},
	{"rekruterk*", "recruiting"},
	{"tokarz*", "skilled_trades"}, // "Machinist"
	{"frezer*", "skilled_trades"},
	{"hartownik*", "skilled_trades"},
	{"traser*", "skilled_trades"},
	{"lakierni*", "skilled_trades"}, // the painter and his shop ("Auto Body Painter")
	{"tapicer*", "skilled_trades"},
	{"slusarz*", "skilled_trades"},
	{"kowal", "skilled_trades"},
	{"pomocnik* mechanik*", "skilled_trades"},
	{"pomocnik* lakiernik*", "skilled_trades"},
	{"instruktor*", "education"}, // "Fitness Instructor"
	{"instruktork*", "education"},
	{"kucharz*", "hospitality"},
	{"pracownik* kuchni", "hospitality"},
	{"kawiarni*", "hospitality"},
	{"restauracj*", "hospitality"},
	{"informatyk*", "devops"}, // as the main table files "informatyk"
	{"informatyczn*", "devops"},

	// Band 3: the fields.
	{"sprzedaz*", "sales"},
	{"rozwoj* biznes*", "sales"},
	{"pozyskani* klient*", "sales"},
	{"eksport*", "sales"},
	{"logistyk*", "logistics"},
	{"logistyczn*", "logistics"},
	{"transport*", "logistics"},
	{"spedycj*", "logistics"},
	{"magazyn*", "logistics"},
	{"zakup*", "operations"},
	{"rekrutac*", "recruiting"},
	{"kadr*", "hr"},
	{"hr", "hr"},
	{"personaln*", "hr"},
	{"wynagrodzen*", "finance"}, // "Payroll"
	{"ksiegowosc*", "finance"},
	{"rachunkowosc*", "finance"},
	{"finans*", "finance"},
	{"kontroling*", "finance"},
	{"windykac*", "finance"},
	{"naleznosc*", "finance"},
	{"kredyt*", "finance"},
	{"marketing*", "marketing"},
	{"mediow* spolecznosciow*", "marketing"},
	{"branz* produkcyjn*", categoryNone}, // the client's sector, not the work
	{"produkcj*", "manufacturing"},
	{"produkcyjn*", "manufacturing"},
	{"sklep*", "retail"},
	{"fryzjer*", "personal_services"},
	{"kosmetyczk*", "personal_services"},
	{"salon* optyczn*", "retail"},
	{"optyki i stylizacji", "retail"},
	{"salon*", "retail"},
	{"aptek*", "healthcare"},
	{"farmacj*", "healthcare"},
	{"medyczn*", "healthcare"},
	{"hotel*", "hospitality"},
	// The front desk, the archive and the legal department: a field, so a role noun
	// above outranks it ("Elektroradiolog z obsługą recepcji" is the radiographer).
	{"recepcj*", "administration"},
	{"recepcyjn*", "administration"},
	{"archiw*", "administration"},
	{"prawn*", "legal"},
	{"gastronom*", "hospitality"},
	{"dwh", "data_engineering"},
}

// polishOnlyWords are words no other language reaching the fallback spells this way,
// so a Polish title written without its diacritics ("Specjalista ds. IT", "Kierownik
// projektu") still counts as Polish. Only the Polish spelling counts: "operator",
// "administrator", "architekt", "technik", "referent", "inspektor", "lider" and
// "ekspert" are English, German, Czech, Spanish or Danish words as well, so their
// feminine ("operatorka", "architektka") stands in for them.
var polishOnlyWords = regexp.MustCompile(`(?:^|[^\p{L}])(?:ds|specjalist\p{L}*|kierowni\p{L}*|pracowni\p{L}*|programist\p{L}*|inzynier\p{L}*|ksiegow\p{L}*|magazynier\p{L}*|kierowc\p{L}*|przedstawiciel\p{L}*|sprzedawc\p{L}*|handlowiec|handlowc\p{L}*|asystent\p{L}*|doradc\p{L}*|doradczyni|konsultantk\p{L}*|opiekun\p{L}*|pielegniar\p{L}*|koordynatork\p{L}*|starsz[ayae]\p{L}*|mlodsz[ayae]\p{L}*|stazyst\p{L}*|praktykant\p{L}*|analityk\p{L}*|analityczk\p{L}*|architektk\p{L}*|kosztorysant|tokarz|lekarz\p{L}*|lekark\p{L}*|higienistk\p{L}*|kurierzy|kurierk\p{L}*|przewoznik\p{L}*|zastepc\p{L}*|zastepczyni|liderk\p{L}*|menedzer\p{L}*|menadzer\p{L}*|inspektork\p{L}*|referentk\p{L}*|wsparcie|stanowisk\p{L}*|techniczk\p{L}*|instruktork\p{L}*|operatork\p{L}*|administratork\p{L}*|dyrektor\p{L}*|prezes\p{L}*|zespol\p{L}*|dzial\p{L}*|obslugi|sprzedazy|zatrudnimy|radca|radczyni)(?:$|[^\p{L}])`)

// polishLetters are the letters only Polish among the fallback's languages writes.
var polishLetters = regexp.MustCompile(`[ąćęłńśźż]`)

// polishGenderNotice is the gender notice of a Polish posting — "(K/M)", "(m/k)",
// "(k/m/n)", "K/M". It needs the "k" (kobieta): "(m/f/d)" is German and English.
var polishGenderNotice = regexp.MustCompile(`\(\s*(?:k\s*/\s*m|m\s*/\s*k)(?:\s*[/|,]\s*[nxd])?\s*\)|(?:^|\s)(?:k\s*/\s*m|m\s*/\s*k)(?:\s*/\s*[nx])?(?:$|\s)`)

// polishEndings are the case endings only Polish spells this way: the adjective's
// "-owy/-owa/-owych" (Czech and Slovak write "-ový/-ová", with a "v"), the genitive's
// "-nego" and "-cji", and the abstract noun's "-ości" ("Architekt Systemowy",
// "Analityk Hurtowni Danych", "Specjalista ds. jakości" without its diacritics).
var polishEndings = regexp.MustCompile(`(?:\p{L}{3}(?:owy|owa|owe|owej|owych|owego|owym|nego|cji|osci)|(?:^|[^\p{L}])danych)(?:$|[^\p{L}])`)

// looksPolish reports whether a lowercased title shows it is written in Polish: a
// letter only Polish writes, its gender notice, a word only Polish spells this way, or
// a Polish case ending.
func looksPolish(lower string) bool {
	if polishLetters.MatchString(lower) || polishGenderNotice.MatchString(lower) {
		return true
	}
	if polishFeminineSuffix.MatchString(lower) {
		return true
	}
	folded := foldPolish(lower)
	return pairsPolishFeminine(folded) || polishOnlyWords.MatchString(folded) || polishEndings.MatchString(folded)
}

// polishFeminineSuffix is the feminine ending appended to a masculine head:
// "Administrator/-ka", "Konsultant(ka)", "Specjalista/tka". The Spanish "/a" is not one.
var polishFeminineSuffix = regexp.MustCompile(`\p{L}\s*(?:/\s*-?|\(\s*-?)\s*(?:ka|tka|czka)(?:$|[^\p{L}])`)

// pairsPolishFeminine reports whether the title spells a role in both genders the
// Polish way — "Ratownik / Ratowniczka", "Optometrysta / Optometrystka", "Radczyni
// Prawna/Radca Prawny": two words around a slash sharing a stem, one of them ending in
// the feminine "-ka" or "-yni". It reads the folded title, so a stem is plain ASCII.
func pairsPolishFeminine(folded string) bool {
	parts := strings.Split(folded, "/")
	for i := 0; i+1 < len(parts); i++ {
		a, b := lastWord(parts[i]), firstWord(parts[i+1])
		if len(a) < 4 || len(b) < 4 {
			continue
		}
		for _, p := range [][2]string{{a, b}, {b, a}} {
			fem, masc := p[0], p[1]
			feminine := strings.HasSuffix(fem, "ka") || strings.HasSuffix(fem, "yni")
			if feminine && strings.HasPrefix(fem, masc[:len(masc)-2]) {
				return true
			}
		}
	}
	return false
}

func lastWord(s string) string {
	f := strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) })
	if len(f) == 0 {
		return ""
	}
	return f[len(f)-1]
}

func firstWord(s string) string {
	f := strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) })
	if len(f) == 0 {
		return ""
	}
	return f[0]
}

// polishGenderSuffix is the feminine ending a Polish title appends to the masculine:
// "Inżynier(-ka)", "Sieciowy(-a)", "Specjalista/-ka", "Konsultant/ka", "Specjalist(k)a",
// "Młodszy/a". It is cut, so the masculine the main table spells is what remains.
var polishGenderSuffix = regexp.MustCompile(`(\p{L})(?:\s*\(\s*-?\s*(?:ka|a|tka|czka|y|i)\s*\)|/\s*-?\s*(?:ka|a|tka|czka)(?:$|[^\p{L}])|\(k\)a)`)

// foldPolishGender cuts the gender forms out of a lowercased title, keeping its
// diacritics: the main table is asked again of it.
func foldPolishGender(lower string) string {
	lower = polishGenderNotice.ReplaceAllString(lower, " ")
	lower = polishGenderSuffix.ReplaceAllStringFunc(lower, func(m string) string {
		sub := polishGenderSuffix.FindStringSubmatch(m)
		if strings.HasSuffix(m, "(k)a") {
			return sub[1] + "a"
		}
		tail := ""
		if r := []rune(m); !unicode.IsLetter(r[len(r)-1]) && r[len(r)-1] != ')' {
			tail = string(r[len(r)-1])
		}
		return sub[1] + tail
	})
	return strings.Join(strings.Fields(lower), " ")
}

// foldPolish prepares a lowercased title for the Polish tables: the gender forms cut,
// "mł." and "st." spelled out as the grades they abbreviate, and the diacritics
// dropped — "ł" by hand, since Unicode does not decompose it.
func foldPolish(lower string) string {
	lower = foldPolishGender(lower)
	lower = polishJuniorAbbrev.ReplaceAllString(lower, "${1}mlodszy ")
	lower = strings.ReplaceAll(lower, "ł", "l")
	var b strings.Builder
	for _, r := range norm.NFD.String(lower) {
		if !unicode.Is(unicode.Mn, r) {
			b.WriteRune(r)
		}
	}
	return polishSeniorAbbrev.ReplaceAllString(b.String(), "${1}starszy $2")
}

// polishJuniorAbbrev is "Mł." — the abbreviated "Młodszy". Only the spelling with
// "ł" and the period counts: a bare "ml" is machine learning.
var polishJuniorAbbrev = regexp.MustCompile(`(^|[^\p{L}])mł\.\s*`)

// polishSeniorAbbrev is "St." — the abbreviated "Starszy" — before a Polish head
// ("St. specjalistka", "St.specjalista"). "St." alone is a saint's or a street's.
var polishSeniorAbbrev = regexp.MustCompile(`(^|[^\p{L}])st\.\s*(specjalist|inzynier|ksiegow|analityk|programist|referent|asystent|konsultant|technik|administrator|koordynator|ekspert|inspektor|kierownik)`)

// polishWords splits a folded title into its words. A hyphen joins ("wodno-kanalizacyjnych",
// "back-end"), and the table spells hyphenated words whole.
func polishWords(folded string) []string {
	return strings.FieldsFunc(folded, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-'
	})
}

// matchPolish returns the canonical of the first alias (in table order) that occurs
// in the words, or "".
func matchPolish(words []string, table []aliasEntry) string {
	for _, e := range table {
		toks := strings.Fields(e.alias)
		for i := 0; i+len(toks) <= len(words); i++ {
			if polishPhraseAt(words[i:], toks) {
				return e.canonical
			}
		}
	}
	return ""
}

// polishPhraseAt reports whether the words begin with the alias tokens, a "*" token
// matching any word that starts with it.
func polishPhraseAt(words, toks []string) bool {
	for j, t := range toks {
		if stem, ok := strings.CutSuffix(t, "*"); ok {
			if !strings.HasPrefix(words[j], stem) {
				return false
			}
		} else if words[j] != t {
			return false
		}
	}
	return true
}

// matchPolishCategory resolves the category of a lowercased title the other tables
// left unresolved: the main table once more on the gender-cut title, then the Polish
// table on the folded one. A blind alias answers categoryNone.
func matchPolishCategory(lower string) string {
	if !looksPolish(lower) {
		return ""
	}
	if g := foldPolishGender(lower); g != lower {
		if c := matchOrdered(g, categoryTable); c != "" {
			return c
		}
	}
	return matchPolish(polishWords(polishPerks.ReplaceAllString(foldPolish(lower), " ")), polishCategoryTable)
}

// polishPerks are the benefits a title advertises beside the role — "darmowy transport"
// is the free shuttle to a warehouse, not a transport job.
var polishPerks = regexp.MustCompile(`\b(?:darmowy|bezplatny|zapewniony|zapewniamy)\s+(?:transport|dojazd)\b`)

// polishSeniorityTable lists the Polish grade aliases, stems on the folded title as in
// polishCategoryTable, in precedence order.
//
// The executive is the board's president and the general, executive and financial
// director, as in French and Spanish; a plain "dyrektor" and the "dyrektor
// zarządzający" of a bank's division state none, as "Director" and "Managing Director"
// state none in the main table. The trainee grades outrank the lead, as in German:
// "Stażysta w zespole" is an intern. The team's own lead is the "kierownik zespołu" and
// the "lider"; a "kierownik" alone is a manager, which is no grade. "Specjalista" is
// the ladder's middle rung by name only and states no grade either; "Regular" is the
// middle grade Polish IT postings write between Junior and Senior. A range ("Młodszy
// specjalista / Specjalista", "Specjalista/Starszy specjalista") keeps its upper bound:
// the senior grade is listed before the junior.
var polishSeniorityTable = []aliasEntry{
	{"prezes*", "c_level"},
	{"wiceprezes*", "c_level"},
	{"dyrektor* generaln*", "c_level"},
	{"dyrektork* generaln*", "c_level"},
	{"dyrektor* wykonawcz*", "c_level"},
	{"dyrektor* finansow*", "c_level"},
	{"stazyst*", "intern"},
	{"praktykant*", "intern"},
	{"staz", "intern"},
	{"stazu", "intern"},
	{"stazow*", "intern"},
	{"praktyki", "intern"},
	{"kierownik* zespol*", "lead"},
	{"kierowniczk* zespol*", "lead"},
	{"lider* zespol*", "lead"},
	{"liderk* zespol*", "lead"},
	{"lider* techniczn*", "lead"},
	{"lider* zmiany", "lead"},
	{"lider* programist*", "lead"},
	{"liderk* programist*", "lead"},
	{"lider* testow", "lead"},
	{"wiodac*", "lead"},
	{"starszy", "senior"},
	{"starsza", "senior"},
	{"starszego", "senior"},
	{"starszej", "senior"},
	{"regular", "middle"},
	{"mlodszy", "junior"},
	{"mlodsza", "junior"},
	{"mlodszego", "junior"},
	{"mlodszej", "junior"},
	{"absolwent*", "junior"},
}

// openingPolishLider is a title that OPENS with "Lider"/"Liderka": "Lider Obszaru",
// "Lider/Liderka Zmiany". Anywhere else the word may praise the employer ("lider
// rynku"), so only the opening word is read, as the Spanish "Líder".
var openingPolishLider = regexp.MustCompile(`^[^\p{L}\p{N}]*lider(?:ka)?\b`)

// assistantOfPolishLead is the assistant or deputy OF a lead or an executive —
// "Asystent kierownika zespołu", "Zastępca Dyrektora Generalnego", "Asystentka Prezesa
// Zarządu" — which states no grade of its own, and a desk that RUNS the trainee
// programmes ("Koordynator ds. staży", "Specjalista ds. praktyk i staży"). Both are cut
// before the grade match.
var assistantOfPolishLead = regexp.MustCompile(`\b(?:asystent\p{L}*|zastepc\p{L}*|zastepczyni|sekretar\p{L}*)\s+(?:(?:i|lub)\s+\p{L}+\s+)?(?:prezes\p{L}*|wiceprezes\p{L}*|dyrektor\p{L}*|kierownik\p{L}*|kierowniczk\p{L}*|lider\p{L}*|koordynator\p{L}*)(?:\s+(?:zespol\p{L}*|generaln\p{L}*|finansow\p{L}*|wykonawcz\p{L}*|zarzadu))?|\bds\s+(?:\p{L}+\s+i\s+)?(?:staz\p{L}*|praktyk\p{L}*)`)

// polishElderly is the elderly person a carer looks after — "Opiekunka osoby starszej",
// "Opiekun osób starszych", "Opieka nad starszą panią" — whose "starszy" is an age, not
// the grade.
var polishElderly = regexp.MustCompile(`\b(?:osob\p{L}*|pan\p{L}*|ludzi|pacjent\p{L}*|senior\p{L}*)\s+starsz\p{L}*|\bstarsz\p{L}*\s+(?:osob\p{L}*|pan\p{L}*|ludzi|pacjent\p{L}*|wiek\p{L}*|dzieci|mieszkan\p{L}*)`)

// matchPolishSeniority resolves the grade of a lowercased title the other tables left
// unresolved.
func matchPolishSeniority(lower string) string {
	if !looksPolish(lower) {
		return ""
	}
	folded := assistantOfPolishLead.ReplaceAllString(foldPolish(lower), " ")
	folded = polishElderly.ReplaceAllString(folded, " ")
	if openingPolishLider.MatchString(folded) {
		return "lead"
	}
	return matchPolish(polishWords(folded), polishSeniorityTable)
}
