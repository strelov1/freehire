package classify

import (
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// Turkish IT titles. Kariyer.net (kariyernet) files its technical postings under the site's IT
// work areas, and measured on 2026-11-17 the English dictionary placed 30 of the slice's 437
// titles as technical and found a category for 59: "Bilgi İşlem Uzmanı" (the IT specialist),
// "Yazılım Geliştirme Uzmanı" (the software developer) and "Sistem Uzmanı" named nothing it
// reads. A posting with neither a category nor a technical flag is hidden from search.
//
// Turkish is agglutinative — "Uzman", "Uzmanı", "Uzmanları" — so the table matches STEMS on the
// folded title, exactly as the Polish table does (polish.go): an alias word ending in "*" matches
// any word that starts with it, and the Polish word splitter and phrase matcher are reused
// as they are. An alias opening with "^" must start the title: "Sistem Uzmanı" is the system
// specialist, "Kalite Sistem Uzmanı" is the quality department's.
//
// foldTurkish drops the diacritics and the dotless ı, so every alias is written plain
// ("bilgi islem", "guvenlik", "kidemli"). Dropping a diacritic merges distinct words ("is",
// work, and "iş") — the table spells only phrases whose folded form is safe, and the English
// titles that reach the same fallback never carry them: the table only reads a title that
// looksTurkish, which asks for a Turkish role word.
//
// Each answer follows the English twin of the same title (IT Specialist is it_support, Business
// Analyst business_analysis, SAP Consultant solutions_engineering). Where the twin is
// questionable the title is left unresolved rather than copied: the English "IT Manager" is
// devops, so "Bilgi İşlem Müdürü" is vetoed with categoryNone; "Sistem Mühendisi" (the English
// "System Engineer" is software_engineering) and "Veri Tabanı Uzmanı" resolve nothing.

// turkishCategoryTable lists the Turkish category aliases on the folded title, in precedence order.
var turkishCategoryTable = []aliasEntry{
	// Security, machine learning and AI before the fields they sit in.
	{"siber guvenlik*", "security"},
	{"bilgi guvenlig*", "security"},
	{"veri guvenlig*", "security"},
	{"ag ve guvenlik*", "security"},
	{"makine ogrenme*", "ml_ai"},
	{"yapay zeka muhendis*", "ai_engineering"},
	// Data.
	{"veri ambari*", "data_engineering"},
	{"veri tabani yonetim*", "devops"},
	{"is zekasi*", "data_analytics"},
	{"veri analist*", "data_analytics"},
	{"veri analitig*", "data_analytics"},
	// The software roles that are not plain development, before the development head.
	{"yazilim mimar*", "architecture"},
	{"cozum mimar*", "architecture"},
	{"yazilim test*", "qa"},
	{"platform tester*", "qa"},
	{"yazilim proje*", "project_management"},
	{"yazilim destek*", "it_support"},
	{"yazilim satis*", "sales"},
	// The IT managers: the English twin files them under devops, which says nothing about the work.
	{"bilgi islem mudur*", categoryNone},
	{"bilgi islem yonetici*", categoryNone},
	{"bilgi teknolojileri mudur*", categoryNone},
	{"bilgi teknolojileri yonetici*", categoryNone},
	// The service desk and the IT department's specialist.
	{"kullanici destek*", "it_support"},
	{"sistem destek*", "it_support"},
	{"bt teknik destek*", "it_support"},
	{"bt destek*", "it_support"},
	{"it destek*", "it_support"},
	{"it uzman*", "it_support"},
	{"ict uzman*", "it_support"},
	{"bilgi islem*", "it_support"},
	{"bilgi teknolojileri uzman*", "it_support"},
	{"bilgi teknolojileri it uzman*", "it_support"},
	{"bilisim teknolojileri*", "it_support"},
	{"bt uzman*", "it_support"},
	// The network, ahead of the system desk: "Sistem ve Network Lideri" is the network lead.
	{"network uzman*", "network_engineering"},
	{"network muhendis*", "network_engineering"},
	{"network lider*", "network_engineering"},
	{"network sorumlu*", "network_engineering"},
	{"bt sistem uzman*", "system_administration"},
	{"^sistem uzman*", "system_administration"},
	{"^sistem yonetim*", "system_administration"},
	{"sistem ve yedekleme*", "system_administration"},
	{"ag ve sistem yonetim*", "system_administration"},
	// The cloud engineer's twin is devops.
	{"bulut muhendis*", "devops"},
	{"bulut altyapi*", "devops"},
	{"cloud muhendis*", "devops"},
	// The development head, the last of the software roles and ahead of ERP and SAP, so "SAP BW - BI Yazılım Uzmanı" is the
	// software developer: every other "yazilim ..." title is the
	// software developer's, as "Software Specialist" would be if the English table named it.
	{"yazilim muhendis*", "software_engineering"},
	{"yazilim gelistir*", "software_engineering"},
	{"yazilim uzman*", "software_engineering"},
	{"yazilimci*", "software_engineering"},
	{"yazilim sef*", "software_engineering"},
	{"yazilim eleman*", "software_engineering"},
	{"yazilim sorumlu*", "software_engineering"},
	{"yazilim stajyer*", "software_engineering"},
	{"abap gelistir*", "software_engineering"},
	// ERP and SAP: the consultant and the module specialist, as in English.
	{"erp", "solutions_engineering"},
	{"sap", "solutions_engineering"},
	{"netsis*", "solutions_engineering"},
	// Business analysis and project management.
	{"is analist*", "business_analysis"},
	{"proje yonetici*", "project_management"},
	{"proje mudur*", "project_management"},
}

// turkishSeniorityTable lists the Turkish grade words: "Kıdemli" is senior, a "Stajyer" an
// intern, and an "Uzman Yardımcısı" (specialist's assistant) the junior rung. "Uzman" alone, the
// bare head of nearly every title, states no grade.
var turkishSeniorityTable = []aliasEntry{
	{"stajyer*", "intern"},
	{"kidemli", "senior"},
	{"uzman yardimci*", "junior"},
	{"uzman yrd", "junior"},
}

// turkishTechTerms are the Turkish titles that are confidently technical whatever category they
// resolve, the counterpart of techTitleTerms. The same rule holds: anchored to the discipline.
// "Yazılım Satış Uzmanı" is a software seller and "Yapay Zeka Uzmanı" an AI specialist at a food
// maker, so neither bare "yazilim" nor bare "yapay zeka" is here.
var turkishTechTerms = []aliasEntry{
	{"yazilim muhendis*", ""},
	{"yazilim gelistir*", ""},
	{"yazilim uzman*", ""},
	{"yazilimci*", ""},
	{"yazilim destek*", ""},
	{"yazilim test*", ""},
	{"yazilim mimar*", ""},
	{"yazilim sef*", ""},
	{"yazilim eleman*", ""},
	{"yazilim sorumlu*", ""},
	{"yazilim stajyer*", ""},
	{"bilgi islem*", ""},
	{"bilgi teknolojileri*", ""},
	{"bilgi sistemleri*", ""},
	{"bilgi guvenlig*", ""},
	{"bilisim teknolojileri*", ""},
	{"bt uzman*", ""},
	{"siber guvenlik*", ""},
	{"yapay zeka muhendis*", ""},
	{"makine ogrenme*", ""},
	{"veri tabani yonetim*", ""},
	{"bt destek*", ""},
	{"bt altyapi*", ""},
	{"bt teknik*", ""},
	{"it destek*", ""},
	{"it uzman*", ""},
	{"ict uzman*", ""},
}

// turkishCues are the Turkish role words that let a title into the Turkish tables. An English,
// German or Polish title carries none, so a table word like "sap" or "erp" cannot reach it.
var turkishCues = []string{
	"uzman", "uzmani", "uzmanlari", "sorumlu", "sorumlusu", "muhendis", "danisman", "yonetici", "yonetmen",
	"mudur", "eleman", "personel", "gelistir", "stajyer", "kidemli", "destek", "teknisyen", "yazilim",
	"bilgi", "islem", "guvenlik", "guvenlig", "yapay", "zeka", "veri", "analisti", "mimari", "yrd",
	"yardimcisi", "ogrenme", "lider", "yonetim", "ekip", "bilisim", "bulut", "altyapi",
	// The non-IT roles and fields (turkish_general.go). Each is a Turkish word no English title
	// carries; "operator" and "market" are deliberately not cues, being English words too.
	"satis", "magaza", "muhasebe", "finans", "insan", "kaynaklari", "ozluk", "pazarlama", "uretim", "depo",
	"lojistik", "sevkiyat", "kalite", "temizlik", "garson", "asci", "kasiyer", "sekreter", "resepsiyon",
	"sofor", "surucu", "hemsire", "hekim", "doktor", "eczaci", "ogretmen", "egitmen", "avukat", "hukuk",
	"usta", "tezgahtar", "kuafor", "berber", "reyon", "gorevli", "operatoru", "isci", "sigorta", "gayrimenkul",
	"emlak", "musteri", "cagri", "idari", "ofis", "bakim", "onarim", "elemani", "temsilci", "teknikeri",
	"mutfak", "bordro", "tahsilat", "butce", "satin", "ithalat", "ihracat", "gumruk", "kurye", "nakliye",
	"tibbi", "hasta", "saglik", "guvenlik", "ressam", "teknik", "santiye", "planlama", "otomasyon", "stok", "otel",
	"misafir", "mimar", "musavir", "kaynakci", "tercuman", "tesisatci", "boyaci", "marangoz", "elektrikci",
	"tamirci", "barmen", "bulasikci", "fizyoterapist", "diyetisyen", "psikolog", "laboratuvar", "kimyager",
	"biyolog", "grafiker", "masor", "makyoz", "estetisyen", "restoran", "komi", "ebe", "ceviri", "fotograf",
}

// foldTurkish prepares a lowercased title for the Turkish tables: the diacritics are dropped
// ("güvenlik" → "guvenlik", "şef" → "sef"), the dotless ı written as i, and the combining dot
// that lowercasing "İ" leaves behind removed with them.
func foldTurkish(lower string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(lower) {
		switch {
		case unicode.Is(unicode.Mn, r):
		case r == 'ı':
			b.WriteRune('i')
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// looksTurkish reports whether a folded title's words carry a Turkish role word.
func looksTurkish(words []string) bool {
	for _, w := range words {
		for _, cue := range turkishCues {
			if strings.HasPrefix(w, cue) {
				return true
			}
		}
	}
	return false
}

// matchTurkish returns the canonical of the first alias, in table order, that occurs in the
// words, or "". An alias opening with "^" matches only at the start of the title.
func matchTurkish(words []string, table []aliasEntry) (string, bool) {
	for _, e := range table {
		alias, anchored := strings.CutPrefix(e.alias, "^")
		toks := strings.Fields(alias)
		for i := 0; i+len(toks) <= len(words); i++ {
			if anchored && i > 0 {
				break
			}
			if polishPhraseAt(words[i:], toks) {
				return e.canonical, true
			}
		}
	}
	return "", false
}

// turkishSplit splits a folded title into its words. A hyphen separates ("Mersin-Bilgi İşlem"):
// no Turkish alias is hyphenated.
func turkishSplit(folded string) []string {
	return strings.FieldsFunc(folded, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// turkishWords folds a lowercased title and splits it into words, reporting "" words when the
// title is not Turkish at all.
func turkishWords(lower string) []string {
	words := turkishSplit(foldTurkish(lower))
	if !looksTurkish(words) {
		return nil
	}
	return words
}

// matchTurkishCategory resolves the category of a lowercased title the other tables left
// unresolved. A blind alias answers categoryNone.
func matchTurkishCategory(lower string) string {
	words := turkishWords(lower)
	if c, ok := matchTurkish(words, turkishCategoryTable); ok {
		return c
	}
	c, _ := matchTurkish(words, turkishGeneralCategoryTable)
	return c
}

// matchTurkishSeniority resolves the grade of a lowercased title the other tables left unresolved.
func matchTurkishSeniority(lower string) string {
	s, _ := matchTurkish(turkishWords(lower), turkishSeniorityTable)
	return s
}

// isTurkishTech reports whether a lowercased title states a confidently technical role in Turkish.
func isTurkishTech(lower string) bool {
	_, ok := matchTurkish(turkishSplit(foldTurkish(lower)), turkishTechTerms)
	return ok
}
