package classify

import (
	"regexp"
	"strings"
	"unicode"
)

// Croatian, Serbian, Bosnian and Montenegrin (BCS) titles are one language family, so
// they share one table. Two things set them apart from the languages before them.
//
// First, the words INFLECT. A title names its role in the nominative ("Voditelj
// trgovine") but the field after it stands in the genitive, the locative or the
// instrumental — "Specijalist za podršku klijentima", "Radnik u proizvodnji", "Rad u
// skladištu". The main table lists whole surface forms, so it reads "prodaja" and not
// "prodaje", "prodaji" or "prodajom". An alias here may therefore end a word in "*",
// which stands for the word's stem followed by one of the case and gender endings in
// bcsEndings ("prodaj*" is prodaja, prodaje, prodaji, prodaju, prodajom; "kuhar*" is
// kuhar, kuhara, kuharica). The endings are a closed list, so a stem never reaches
// into an unrelated longer word.
//
// Second, Serbian is written in two scripts. A title in Cyrillic ("Програмер",
// "Возач") is transliterated into the Latin alphabet before this table reads it, by
// the official one-to-one mapping (serbianLatin) — but only a title that shows it is
// Serbian (looksSerbianCyrillic). Russian, Ukrainian and Bulgarian titles have tables of
// their own, the main table's Cyrillic aliases.
//
// The aliases are matched on a FOLDED title: transliterated, the gender forms cut
// ("(m/ž)", "Voditelj/ica", "Inženjer/ka", "MLAĐI/A"), and the diacritics dropped,
// đ written "dj" — job boards print both "Vozač" and "Vozac", and "računovođa" and
// "racunovodja". Before the BCS table, the main table is asked once more of the
// transliterated, gender-cut title, so a Cyrillic "Програмер" finds the main table's
// "programer".
//
// The table is a FALLBACK: it runs only after every other table resolved nothing, so
// no existing answer changes. Every entry must also be safe on the English, Slovenian,
// Polish, Czech and Slovak titles that reach the same fallback; the stems are BCS
// words that those languages do not spell, and the ones that need it are argued
// beside their entry.

// bcsEndings are the case and gender endings a "*" stem may take. They cover the
// nominative, genitive, dative, accusative, instrumental and locative of the noun and
// adjective declensions in both numbers, and the feminine agent suffixes (-ica, -ka,
// -kinja) with their own endings.
var bcsEndings = map[string]bool{
	"": true, "a": true, "e": true, "i": true, "o": true, "u": true,
	"om": true, "em": true, "ama": true, "ima": true, "ih": true, "im": true,
	"og": true, "oj": true, "ome": true, "omu": true, "ega": true, "emu": true, "ej": true,
	"ov": true, "ova": true, "ovi": true, "ove": true, "ovima": true,
	"ev": true, "eva": true, "evi": true, "eve": true, "evima": true,
	"ica": true, "ice": true, "ici": true, "icu": true, "icom": true, "icama": true,
	"ka": true, "ke": true, "ki": true, "ku": true, "kom": true, "kama": true,
	"kinja": true, "kinje": true, "kinji": true, "kinju": true,
}

// bcsToken is one word of a BCS alias: a literal word, or a stem that takes an ending.
// A diacritic token is matched against the title's words with their diacritics kept.
type bcsToken struct {
	text      string
	stem      bool
	diacritic bool
}

type bcsEntry struct {
	tokens    []bcsToken
	canonical string
}

func compileBCS(table []aliasEntry) []bcsEntry {
	out := make([]bcsEntry, 0, len(table))
	for _, e := range table {
		var toks []bcsToken
		alias, diacritic := strings.CutPrefix(e.alias, "=")
		for _, w := range strings.Fields(alias) {
			if s, ok := strings.CutSuffix(w, "*"); ok {
				toks = append(toks, bcsToken{text: s, stem: true, diacritic: diacritic})
			} else {
				toks = append(toks, bcsToken{text: w, diacritic: diacritic})
			}
		}
		out = append(out, bcsEntry{tokens: toks, canonical: e.canonical})
	}
	return out
}

// matches reports whether word is the token: the literal word, or the stem followed
// by one of bcsEndings.
func (t bcsToken) matches(word string) bool {
	if !t.stem {
		return word == t.text
	}
	rest, ok := strings.CutPrefix(word, t.text)
	return ok && bcsEndings[rest]
}

// bcsTitle is a folded title's words twice over: with their diacritics, and in ASCII.
type bcsTitle struct {
	diacritic, ascii []string
}

// matchBCSOrdered returns the canonical value of the first entry (in table order)
// whose tokens occur as consecutive words of the folded title, or "".
func matchBCSOrdered(title bcsTitle, table []bcsEntry) string {
	for _, e := range table {
		n := len(e.tokens)
		for i := 0; i+n <= len(title.ascii); i++ {
			ok := true
			for j, t := range e.tokens {
				words := title.ascii
				if t.diacritic {
					words = title.diacritic
				}
				if !t.matches(words[i+j]) {
					ok = false
					break
				}
			}
			if ok {
				return e.canonical
			}
		}
	}
	return ""
}

// serbianLatin is the official Serbian Cyrillic-to-Latin mapping (Gaj's alphabet).
// The Macedonian letters ѓ, ќ and ѕ are mapped too, as Macedonian titles read the same
// way.
var serbianLatin = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'ђ': "đ", 'е': "e", 'ж': "ž",
	'з': "z", 'и': "i", 'ј': "j", 'к': "k", 'л': "l", 'љ': "lj", 'м': "m", 'н': "n",
	'њ': "nj", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'ћ': "ć", 'у': "u",
	'ф': "f", 'х': "h", 'ц': "c", 'ч': "č", 'џ': "dž", 'ш': "š",
	'ѓ': "đ", 'ќ': "ć", 'ѕ': "dz",
}

// latinizeSerbian transliterates a lowercased title written in Serbian Cyrillic into
// the Latin alphabet, and returns any other title unchanged.
//
// A title is read as Serbian only when it says so (looksSerbianCyrillic): Russian,
// Ukrainian and Bulgarian titles reach this fallback too, and many of their words are
// spelled only with letters the Serbian alphabet shares — "Шеф-повар", "Контролер",
// "Аналитик в логистику", "Кухар". Transliterated, those read as BCS words ("šef",
// "kontroler", "logistiku", "kuhar") and would change rows the main table's own Cyrillic
// aliases left alone on purpose.
func latinizeSerbian(lower string) string {
	if !looksSerbianCyrillic(lower) {
		return lower
	}
	var b strings.Builder
	for _, r := range lower {
		if l, ok := serbianLatin[r]; ok {
			b.WriteString(l)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// looksSerbianCyrillic reports whether a lowercased title is written in Serbian (or
// Macedonian) Cyrillic: every Cyrillic letter in it is one the Serbian alphabet has, and
// it carries a letter only Serbian and Macedonian use (ђ, ј, љ, њ, ћ, џ) or one of the
// Serbian role words no Russian, Ukrainian or Bulgarian title spells (serbianCyrillicCue).
func looksSerbianCyrillic(lower string) bool {
	cyrillic, serbianOnly := false, false
	for _, r := range lower {
		if !unicode.Is(unicode.Cyrillic, r) {
			continue
		}
		if _, ok := serbianLatin[r]; !ok {
			return false
		}
		cyrillic = true
		switch r {
		case 'ђ', 'ј', 'љ', 'њ', 'ћ', 'џ':
			serbianOnly = true
		}
	}
	return cyrillic && (serbianOnly || serbianCyrillicCue.MatchString(lower))
}

// serbianCyrillicCue is a Serbian role word spelled without a Serbian-only letter, which
// no other Cyrillic language reaching the fallback writes this way: the Russian
// programmer is a "программист", the driver a "водитель", the seller a "продавец", the
// waiter an "официант", the technician a "техник". Each is a word stem; the boundary
// before it is spelled out, as RE2's \b is ASCII-only.
var serbianCyrillicCue = regexp.MustCompile(`(?:^|[^\p{L}])(?:програмер|возач|продавац|продавачиц|конобар|кувар|сарадни[кц]|саветни[кц]|медицинск[аи]|рачуновод|магацин|оператер|техничар|приправни[кц]|чистачиц|електричар|стручни|референт за|менаџ)`)

// bcsGenderNotice is the gender notice BCS titles carry: "(m/ž)", "(M/Ž)", "(ž/m)",
// "(m/ž/x)", "m/ž", "M/Ž/D". The bare form is cut only as a whole token.
var bcsGenderNotice = regexp.MustCompile(`\(\s*[mžz]\s*/\s*[mžz](?:\s*/\s*[xdo])?\s*\)|(?:^|\s|-)[mž]\s*/\s*[žm](?:\s*/\s*[xd])?(?:$|\s|[,;)])`)

// bcsGenderSlash is the inline feminine form written after a slash: "Voditelj/ica",
// "Inženjer/ka", "MLAĐI/A", "Operater/ka", "ISTRAŽIVAČ/ICA". The ending is cut, so the
// masculine word before it matches.
var bcsGenderSlash = regexp.MustCompile(`(\p{L})/(?:ica|ice|ka|ke|kinja|a|e)([\s,;:()]|$)`)

// foldBCSGender transliterates a lowercased title and cuts its gender forms, keeping
// the diacritics: the main table is asked again of it.
func foldBCSGender(lower string) string {
	lower = latinizeSerbian(lower)
	lower = bcsGenderNotice.ReplaceAllString(lower, " ")
	lower = bcsGenderSlash.ReplaceAllString(lower, "$1$2")
	return strings.Join(strings.Fields(lower), " ")
}

// bcsASCII drops the diacritics the BCS alphabets use and writes đ as "dj".
var bcsASCII = strings.NewReplacer("č", "c", "ć", "c", "š", "s", "ž", "z", "đ", "dj")

// bcsWords splits a title already folded by foldBCSGender into its words, with their
// diacritics and in ASCII (đ written "dj"; a word keeps its position in both).
func bcsWords(folded string) bcsTitle {
	var t bcsTitle
	for _, w := range strings.FieldsFunc(folded, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		t.diacritic = append(t.diacritic, w)
		t.ascii = append(t.ascii, bcsASCII.Replace(w))
	}
	return t
}

var (
	bcsCategory  = compileBCS(bcsCategoryTable)
	bcsSeniority = compileBCS(bcsSeniorityTable)
)

// matchBCSCategory resolves the category of a lowercased title the other tables left
// unresolved: the main table once more on the transliterated, gender-cut title, then
// the BCS table on the folded one. A blind alias answers categoryNone.
func matchBCSCategory(lower string) string {
	g := foldBCSGender(lower)
	if g != lower {
		if c := matchOrdered(g, categoryTable); c != "" {
			return c
		}
	}
	return matchBCSOrdered(bcsWords(g), bcsCategory)
}

// matchBCSSeniority resolves the grade of a lowercased title the other tables left
// unresolved: the main table once more on the transliterated, gender-cut title, then
// the BCS table with the assistants of a lead cut (bcsAssistantOfLead).
func matchBCSSeniority(lower string) string {
	g := foldBCSGender(lower)
	if g != lower {
		if s := matchOrdered(g, seniorityTable); s != "" {
			return s
		}
	}
	return matchBCSOrdered(bcsWords(bcsAssistantOfLead.ReplaceAllString(g, " ")), bcsSeniority)
}

// bcsCategoryTable lists the BCS category aliases, in precedence order, on the folded
// title: ASCII, and a word ending in "*" is a stem that takes a case or gender ending
// (bcsEndings). Each is filed where its English twin is.
//
// It runs in bands, as the Spanish table does: the open applications and the
// head-and-field pairs whose answer is not the field's; then the role nouns, which
// outrank the field ("Radnik u proizvodnji - Viljuškarista" drives a forklift); then
// the fields that give a generic head — voditelj, specijalist, referent, suradnik,
// radnik, operater, tehničar, asistent — its category; then the desk words.
var bcsCategoryTable = []aliasEntry{
	// Band 1. An open or general application names no role, whatever department it
	// mentions ("Otvorena molba u Sektoru informatike"), and neither does a
	// scholarship or a mystery shopper ("Tajni kupac", which English leaves unresolved).
	{"otvoren* prijav*", categoryNone},
	{"otvoren* molb*", categoryNone},
	{"otvoren* zamolb*", categoryNone},
	{"otvoren* aplikacij*", categoryNone},
	{"otvoren* pozicij*", categoryNone},
	{"opst* prijav*", categoryNone},
	{"stipendij*", categoryNone},
	{"stipendist*", categoryNone},
	{"tajn* kupac*", categoryNone},
	{"tajn* kupc*", categoryNone},
	// The software engineer by another name: full-stack ("cjeloviti razvoj"), the
	// computer and IT engineer, software development.
	{"inzenjer* cjelovit* razvoj*", "fullstack"},
	{"inzenjer* racunarstv*", "software_engineering"},
	{"inzenjer* informacijsk* tehnologij*", "software_engineering"},
	{"inzenjer* softver*", "software_engineering"},
	{"razvoj* softver*", "software_engineering"},
	{"razvoj* aplikacij*", "software_engineering"},
	{"developer* baz* podatak*", "software_engineering"},
	// The engineering disciplines, and the site and commissioning engineers, as
	// "Mechanical/Electrical/Civil/Site Engineer" are.
	{"inzenjer* strojarstv*", "industrial_engineering"},
	{"inzenjer* masinstv*", "industrial_engineering"},
	{"inzenjer* elektrotehnik*", "industrial_engineering"},
	{"inzenjer* gradjevin*", "industrial_engineering"},
	{"inzenjer* na gradilist*", "industrial_engineering"},
	{"pustanj* u pogon*", "industrial_engineering"},
	{"mrezn* inzenjer*", "network_engineering"},
	// Business development is sales, not software; an employee's development is HR.
	{"razvoj* poslovanj*", "sales"},
	{"razvoj* poslovn*", "sales"},
	{"razvoj* korporativn* poslovanj*", "sales"},
	{"razvoj* zaposlenik*", "hr"},
	// The insurer's claims handler, as "Claims adjuster" is.
	{"obrad* stet*", "finance"},
	{"procen* stet*", "finance"},
	{"procjen* stet*", "finance"},
	{"likvidacij* stet*", "finance"},
	// Quality assurance is the software tester or the plant's inspector, and the title
	// rarely says which.
	{"osiguranj* kvalitet*", categoryNone},
	{"kontrol* kvalitet*", "manufacturing"},
	{"kontrolor* kvalitet*", "manufacturing"},
	// The IT desks before their generic words: the technical and application support
	// desk, security, the network, the system and database administrators.
	{"tehnick* podrsk*", "it_support"},
	{"informatick* podrsk*", "it_support"},
	{"ict podrsk*", "it_support"},
	{"it podrsk*", "it_support"},
	{"aplikacijsk* podrsk*", "it_support"},
	{"it tehnicar*", "it_support"},
	{"ict tehnicar*", "it_support"},
	{"it servis*", "it_support"},
	{"informacijsk* sigurnost*", "security"},
	{"kibernetick* sigurnost*", "security"},
	{"cyber sigurnost*", "security"},
	{"aplikativn* sigurnost*", "security"},
	{"it sigurnost*", "security"},
	{"it rizik*", "security"},
	{"it administracij*", "devops"},
	{"it infrastruktur*", "devops"},
	{"sigurnost* na rad*", "occupational_safety"},
	{"zastit* na rad*", "occupational_safety"},
	{"mrezn* infrastruktur*", "network_engineering"},
	{"mrezn* administrator*", "network_engineering"},
	{"administrator* sustav*", "devops"},
	{"administrator* sistem*", "devops"},
	{"administrator* baz* podatak*", "devops"},
	// Analysts by their field.
	{"poslovn* analiticar*", "business_analysis"},
	{"analiticar* poslovn*", "business_analysis"},
	{"bi analiticar*", "data_analytics"},
	{"sql analiticar*", "data_analytics"},
	{"analiticar* podatak*", "data_analytics"},
	{"kreditn* rizik*", "finance"},
	// Category management is operations, as "Category Manager" is.
	{"menadzer* kategorij*", "operations"},
	{"upravljanj* kategorij*", "operations"},
	{"analiticar* kategorij*", "operations"},
	// Stock control is logistics, a shop's stock included ("Specijalist upravljanja
	// zalihama trgovina").
	{"upravljanj* zalih*", "logistics"},
	// The machine operator and the crane operator, as in English.
	{"operater* na stroj*", "manufacturing"},
	{"operater* stroj*", "manufacturing"},
	{"operater* na proizvodn*", "manufacturing"},
	{"operater* na pres*", "manufacturing"},
	{"operater* obaln* dizalic*", "skilled_trades"},
	{"operater* dizalic*", "skilled_trades"},
	{"lucke mehanizacij*", "skilled_trades"},
	// The executive's assistant is administration, as "Executive Assistant" is.
	{"pomocnik* izvrsn* direktor*", "administration"},
	{"pomocnic* izvrsn* direktor*", "administration"},
	// The computer technician is the IT help desk.
	{"tehnicar* za racunaln*", "it_support"},
	{"tehnicar* za racunalstv*", "it_support"},
	{"sistemsk* tehnicar*", "it_support"},
	{"agent* podrsk*", "support"},
	// The sales floor is the shop, not the sales team.
	{"prodajn* mjest*", "retail"},
	{"prodajn* mest*", "retail"},
	{"prodajn* centr*", "retail"},
	{"prodajn* centar", "retail"},
	{"prodajn* prostor*", "retail"},
	{"prodajn* objekt*", "retail"},
	{"maloprodajn* objekt*", "retail"},
	// The receptionist is administration ("recepcionar" in the main table), and so is
	// the reception desk; a betting shop's payment counter is the cashier.
	{"uplatn* mjest*", "retail"},
	{"uplatn* mest*", "retail"},

	// Band 2. The role nouns, in the inflected and feminine forms the main table does
	// not list.
	{"programer*", "software_engineering"},
	{"dizajner*", "design"},
	{"dizajn*", "design"},
	{"videograf*", "creative"},
	{"fotograf*", "creative"},
	{"uredjivac* videozapis*", "creative"},
	{"prevodilac*", "technical_writing"},
	{"prevoditelj*", "technical_writing"},
	{"racunovodj*", "finance"},
	{"knjigovodj*", "finance"},
	{"blagajnik*", "retail"},
	{"blagajnic*", "retail"},
	{"prodavac*", "retail"},
	{"prodavc*", "retail"},
	{"odvajac*", "logistics"},
	{"sastavljac*", "manufacturing"},
	{"slagac* polic*", "retail"},
	{"komercijalist*", "sales"},
	{"teleprodaj*", "sales"},
	{"terensk* zastupnik*", "sales"},
	{"vozac*", "logistics"},
	{"skladistar*", "logistics"},
	{"magacioner*", "logistics"},
	{"konobar*", "hospitality"},
	{"kuhar*", "hospitality"},
	{"kuvar*", "hospitality"},
	{"sobaric*", "hospitality"},
	{"medicinsk* sestr*", "healthcare"},
	{"medicinsk* tehnicar*", "healthcare"},
	{"doktor* medicin*", "healthcare"},
	{"doktor* dentaln* medicin*", "healthcare"},
	{"brijac*", "personal_services"},
	{"frizer*", "personal_services"},
	{"vizazist*", "personal_services"},
	{"kozmeticar*", "personal_services"},
	{"cistac*", "personal_services"},
	{"spremac*", "personal_services"},
	{"hidraulicar*", "skilled_trades"},
	{"elektroinstalater*", "skilled_trades"},
	{"instalater*", "skilled_trades"},
	{"zastitar*", "public_safety"},
	{"redar*", "public_safety"},
	{"nastavnik*", "education"},
	{"ucitelj*", "education"},
	{"tajnic*", "administration"},

	// Band 3. The fields.
	{"prodaj*", "sales"},
	{"prodajn*", "sales"},
	{"kljucn* kupac*", "sales"},
	{"kljucn* kupc*", "sales"},
	{"trgovin*", "retail"},
	{"prodavaonic*", "retail"},
	{"prodavnic*", "retail"},
	{"radnj*", "retail"},
	{"maloprodaj*", "retail"},
	{"kladionic*", "retail"},
	{"knjizar*", "retail"},
	{"kuhinj*", "hospitality"},
	{"kuhinjsk*", "hospitality"},
	{"restoransk*", "hospitality"},
	{"gostionic*", "hospitality"},
	{"rezervacij*", "hospitality"},
	{"rezervacijsk*", "hospitality"},
	{"hotelsk*", "hospitality"},
	{"restoran*", "hospitality"},
	{"ugostiteljstv*", "hospitality"},
	{"slasticarn*", "hospitality"},
	{"poslasticarn*", "hospitality"},
	{"gelaterij*", "hospitality"},
	{"priprem* hran*", "hospitality"},
	{"hotelsk* operacij*", "hospitality"},
	{"hotelsk* osoblj*", "hospitality"},
	{"animacij*", "hospitality"},
	{"recepcij*", "administration"},
	{"recepcijsk*", "administration"},
	{"domacinstv*", "personal_services"},
	{"skladist*", "logistics"},
	{"magacin*", "logistics"},
	{"logistik*", "logistics"},
	{"logisticki", "logistics"},
	{"spedicij*", "logistics"},
	{"dostav*", "logistics"},
	{"komisioniranj*", "logistics"},
	{"pakiranj*", "logistics"},
	{"pakovanj*", "logistics"},
	{"vozn* park*", "logistics"},
	{"transportn* radnik*", "logistics"},
	{"zalih*", "logistics"},
	{"nabav*", "operations"},
	{"nabavk*", "operations"},
	{"proizvodnj*", "manufacturing"},
	{"pogon*", "manufacturing"},
	{"tvornic*", "manufacturing"},
	{"fabrik*", "manufacturing"},
	{"tiskar*", "manufacturing"},
	{"konfekcij*", "manufacturing"},
	{"cnc", "manufacturing"},
	{"odrzavanj*", "skilled_trades"},
	{"gradilist*", "skilled_trades"},
	{"gradjenj*", "skilled_trades"},
	{"jak* struj*", "skilled_trades"},
	{"slab* struj*", "skilled_trades"},
	{"elektroinstalacij*", "skilled_trades"},
	{"autolimarij*", "skilled_trades"},
	{"gradjevinsk* poslovodj*", "skilled_trades"},
	{"racunovodstv*", "finance"},
	{"knjigovodstv*", "finance"},
	{"financij*", "finance"},
	{"finansij*", "finance"},
	{"kontroling*", "finance"},
	{"revizij*", "finance"},
	{"naplat*", "finance"},
	{"riznic*", "finance"},
	{"obracun* plac*", "finance"},
	{"ljudsk* potencijal*", "hr"},
	{"ljudsk* resurs*", "hr"},
	{"kadrovsk*", "hr"},
	{"kompenzacij*", "hr"},
	{"zaposljavanj*", "recruiting"},
	{"selekcij*", "recruiting"},
	{"regrutacij*", "recruiting"},
	{"marketing*", "marketing"},
	{"drustven* mrez*", "marketing"},
	{"intern* komunikacij*", "marketing"},
	{"robn* mark*", "marketing"},
	{"pravn* posl*", "legal"},
	{"pravn* sluzb*", "legal"},
	{"pravn* savjetnik*", "legal"},
	{"pravn* savetnik*", "legal"},
	{"radn* prav*", "legal"},
	{"korisnick* podrsk*", "support"},
	{"podrsk* klijent*", "support"},
	{"podrsk* korisnik*", "support"},
	{"korisnick* servis*", "support"},
	{"korisnick* sluzb*", "support"},
	{"korisnick* iskustv*", "support"},
	{"kontakt centr*", "support"},
	{"kontakt centar", "support"},
	{"pozivn* centr*", "support"},
	{"pozivn* centar", "support"},
	{"call centar", "support"},
	{"iskustv* kupac*", "support"},
	{"iskustv* korisnik*", "support"},
	{"prigovor*", "support"},
	{"odnos* s kupac*", "support"},
	{"odnos* s kupc*", "support"},
	{"obezbedjenj*", "public_safety"},
	{"obezbjedjenj*", "public_safety"},
	{"fizick* zastit*", "public_safety"},
	{"tjelesn* zastit*", "public_safety"},
	{"video nadzor*", "public_safety"},
	{"informatick*", "it_support"},
	{"informatik*", "it_support"},
	{"it sluzb*", "it_support"},
	{"it referent*", "it_support"},
	{"medicin*", "healthcare"},
	{"ljekarn*", "healthcare"},
	{"apotek*", "healthcare"},
	{"stomatolog*", "healthcare"},
	{"dermatovenerologij*", "healthcare"},
	{"ginekologij*", "healthcare"},
	{"pedijatrij*", "healthcare"},
	{"laboratorij*", "science"},

	// Band 4. The desk words.
	{"administracij*", "administration"},
	{"administrativn*", "administration"},
	{"uredsk*", "administration"},
}

// bcsSeniorityTable lists the BCS grade aliases on the folded title, in precedence
// order. An alias opening with "=" is matched on the title with its diacritics kept:
// "viša" is the senior grade, ASCII "visa" an English word, and "vođa" is the BCS
// leader where the Slovenian "vodja" is any manager.
//
// Only the general, executive and financial directors and the management board are the
// executive, as in the other tables; a "Direktor prodaje" states no grade, as "Sales
// Director" states none. The trainee grades outrank the lead. "Voditelj" alone is the
// manager and states none; the team's and the shift's own lead does.
var bcsSeniorityTable = []aliasEntry{
	{"generaln* direktor*", "c_level"},
	{"izvrsn* direktor*", "c_level"},
	{"financijsk* direktor*", "c_level"},
	{"finansijsk* direktor*", "c_level"},
	{"predsjednik* uprav*", "c_level"},
	{"predsednik* uprav*", "c_level"},
	{"clan* uprav*", "c_level"},
	{"pripravnik*", "intern"},
	{"pripravnic*", "intern"},
	{"praks*", "intern"},
	{"praktikant*", "intern"},
	{"stazist*", "intern"},
	{"=vođ*", "lead"},
	{"vodja tim*", "lead"},
	{"vodja smjen*", "lead"},
	{"vodja smen*", "lead"},
	{"voditelj* tim*", "lead"},
	{"voditelj* smjen*", "lead"},
	{"voditelj* smen*", "lead"},
	{"=šef*", "lead"},
	{"=vodeći", "lead"},
	{"=vodeća", "lead"},
	{"vodeci", "lead"},
	{"=viši", "senior"},
	{"=viša", "senior"},
	{"=višeg", "senior"},
	{"visi", "senior"},
	{"stariji", "senior"},
	{"starija", "senior"},
	{"mladj*", "junior"},
	{"=niži", "junior"},
	{"=niža", "junior"},
	{"nizi", "junior"},
	{"pocetnik*", "junior"},
	{"pocetnic*", "junior"},
}

// bcsAssistantOfLead is the assistant or deputy OF a lead or an executive — "Pomoćnik
// izvršnog direktora", "Zamjenik voditelja smjene", "Asistent šefa kuhinje" — which
// states no grade of its own. It is cut, on the folded title, before the grade match.
var bcsAssistantOfLead = regexp.MustCompile(`\b(?:pomo[cć]ni(?:k|ca)|asistent(?:ica)?|zamjeni(?:k|ca)|zameni(?:k|ca)|sekretar(?:ica)?|tajni(?:k|ca))\s+(?:(?:glavnog|generalnog|izvr[sš]nog|financijskog|finansijskog)\s+)?(?:direktor|voditelj|vo(?:dj|đ)|[sš]ef|predsjednik|predsednik)\p{L}*`)
