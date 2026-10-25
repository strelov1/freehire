package classify

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/strelov1/freehire/internal/dict/wordmatch"
)

// German titles name their role in a COMPOUND, and the compound's head is its last
// morpheme: a Kranken|pfleger, an Alten|pfleger and a Kinderkranken|pfleger are all
// nurses, a Stapler|fahrer and a Berufskraft|fahrer both drive. Listing every
// compound as a whole-word alias — the shape the main tables use — cannot keep up:
// measured 2026-10-25 over the 123,605 open postings of the German ATS crawl
// (coveto, rexx, xing, dvinci, concludis, onlyfy, kenjo), 85% resolved no category
// and 98% no grade, and the unresolved titles were a long tail of compounds whose
// heads are a few dozen words.
//
// So the German tables match a word's TAIL. An alias written "-pfleger" is a head:
// it matches any word that ends in it ("krankenpfleger", "pfleger"). An alias
// without the dash is a whole-word phrase, matched exactly as the main tables match
// ("koch", "fachkraft für lagerlogistik"). Precedence is table order across both
// kinds, so the most specific entry is listed first.
//
// A head is resolved PER WORD: each word takes the first head in the table it ends
// in, and only then do the words compete by table order. That is what lets a longer
// head veto a shorter one inside one word without blanking the title — "begleiter"
// ends in "leiter", so "-begleiter" is listed first with categoryNone and an
// Alltagsbegleiter states no grade, while the "Teamleiter" beside it still states
// one. A whole-word phrase carrying categoryNone blocks the whole title, as in the
// main table.
//
// The gender forms need nothing in most spellings: "(m/w/d)", "/in", "_in", ":in"
// and "*in" are separated from the word by a non-letter, so the head is still the
// word's tail. The fused forms are folded per word before the match — "Erzieherin"
// and "ErzieherInnen" end in "-in"/"-innen", which is cut once so the masculine head
// matches. A word whose head is only spelled in the feminine (Ärztin) lists it.
//
// These tables are a FALLBACK: they run only when the main table resolved nothing,
// so a title that already resolves keeps its answer. That placement is deliberate —
// a German head never outvotes an alias the main table was ordered around.

// germanCategoryTable lists the German category aliases, heads ("-…") and whole-word
// phrases, in precedence order.
var germanCategoryTable = []aliasEntry{
	// Vetoes first: words whose tail looks like a head below and is not one.
	{"-landschaftspfleger", categoryNone}, // a landscape keeper, not a nurse
	{"-denkmalpfleger", categoryNone},     // a monument conservator
	{"-grünpfleger", categoryNone},
	{"-baumpfleger", categoryNone},      // an arborist
	{"-tierpfleger", categoryNone},      // an animal keeper
	{"-pferdepfleger", categoryNone},    // a stable hand
	{"-alltagsbegleiter", "healthcare"}, // the elderly-care companion, before the veto below
	{"-schulbegleiter", "education"},    // a pupil's classroom aide
	{"-begleiter", categoryNone},
	{"-halbleiter", categoryNone}, // a semiconductor
	{"-programmierer", categoryNone},
	{"-informatiker", categoryNone},
	{"-systemtechniker", categoryNone},
	{"-medizintechniker", categoryNone},
	{"-zahntechniker", categoryNone},

	// Public safety. Listed before the office block: a security firm's
	// "Pforten- / Empfangsmitarbeiter" is the gatekeeper, not the receptionist.
	{"-sicherheitsmitarbeiter", "public_safety"},
	{"-sicherheitskraft", "public_safety"},
	{"-sicherheitskräfte", "public_safety"},
	{"-schutzkraft", "public_safety"}, // Werkschutzkraft
	{"-deeskalationskraft", "public_safety"},
	{"-detektiv", "public_safety"},
	{"-revierfahrer", "public_safety"}, // the mobile patrol of a security firm
	{"-interventionsfahrer", "public_safety"},
	{"-fahrkartenkontrolleur", "public_safety"},
	{"-pförtner", "public_safety"},
	{"pforten", "public_safety"},
	{"-wachmann", "public_safety"},
	{"-ticketprüfer", "public_safety"},
	{"-securitymitarbeiter", "public_safety"},
	{"-veranstaltungssecurity", "public_safety"},
	{"luftsicherheitskontrolle", "public_safety"},
	{"-polizist", "public_safety"},
	{"-polizeibeamter", "public_safety"},
	{"-feuerwehrmann", "public_safety"},
	{"-brandmeister", "public_safety"},
	{"-rettungsschwimmer", "public_safety"},
	{"objektschutz", "public_safety"},
	{"werkschutz", "public_safety"},
	{"sicherheitsdienst", "public_safety"},
	{"sicherheitsbereich", "public_safety"},
	{"geld- und werttransport", "public_safety"},
	{"werttransport", "public_safety"},

	// Occupational safety — the Fachkraft für Arbeitssicherheit (SiFa) audits a plant;
	// it is not the guard above.
	{"arbeitssicherheit", "occupational_safety"},
	{"arbeitsschutz", "occupational_safety"},
	{"-sicherheitsfachkraft", "occupational_safety"},
	{"-sicherheitsingenieur", "occupational_safety"},
	{"sifa", "occupational_safety"},

	// Social services and childcare sit before the nurse heads they end in.
	{"-heilerziehungspfleger", "social_services"},
	{"-heilerzieher", "social_services"},
	{"-heilpädagoge", "social_services"},
	{"-sozialpädagoge", "social_services"},
	{"-sozialarbeiter", "social_services"},
	{"-sozialassistent", "social_services"},
	{"eingliederungshilfe", "social_services"},
	{"sozialpädagogische fachkraft", "social_services"},
	{"-kinderpfleger", "education"},
	// A Raumpfleger cleans rooms.
	{"-raumpfleger", "personal_services"},

	// Healthcare.
	{"-arzt", "healthcare"},
	{"-ärztin", "healthcare"},
	{"-ärzte", "healthcare"},
	{"-mediziner", "healthcare"},
	{"-pfleger", "healthcare"},
	{"-pflegekraft", "healthcare"},
	{"-pflegefachkraft", "healthcare"},
	{"-pflegehilfskraft", "healthcare"},
	{"-dialysefachkraft", "healthcare"},
	{"-praxisanleiter", "healthcare"}, // the nurse who trains the apprentices on the ward
	{"-pflegekräfte", "healthcare"},
	{"-pflegefachkräfte", "healthcare"},
	{"-pflegehelfer", "healthcare"},
	{"-pflegeassistent", "healthcare"},
	{"-pflegeassistenz", "healthcare"},
	{"-pflegefachassistent", "healthcare"},
	{"-pflegefachmann", "healthcare"},
	{"-pflegefachfrau", "healthcare"},
	{"-pflegefachperson", "healthcare"},
	{"-pflegedienstleitung", "healthcare"},
	{"-pflegedienstleiter", "healthcare"},
	{"-wohnbereichsleitung", "healthcare"},
	{"-wohnbereichsleiter", "healthcare"},
	{"-krankenschwester", "healthcare"},
	{"-hebamme", "healthcare"},
	{"-therapeut", "healthcare"},
	{"-logopäde", "healthcare"},
	{"-psychologe", "healthcare"},
	{"-apotheker", "healthcare"},
	{"-sanitäter", "healthcare"},
	{"-optiker", "healthcare"},
	{"-optikermeister", "healthcare"},
	{"-akustiker", "healthcare"},
	{"-heilpraktiker", "healthcare"},
	{"-masseur", "healthcare"},
	{"-podologe", "healthcare"},
	// The companion of the elderly-care home (Betreuungskraft nach §43b SGB XI).
	{"-betreuungskraft", "healthcare"},
	{"-krankenpflege", "healthcare"},
	{"-altenpflege", "healthcare"},
	{"-intensivpflege", "healthcare"},
	{"pflege", "healthcare"},
	{"medizinische fachangestellte", "healthcare"},
	{"medizinische/r fachangestellte", "healthcare"},
	{"zahnmedizinische/r fachangestellte", "healthcare"},
	{"medizinischer fachangestellter", "healthcare"},
	{"zahnmedizinische fachangestellte", "healthcare"},
	{"zahnmedizinischer fachangestellter", "healthcare"},
	{"operationstechnische", "healthcare"},
	{"operationstechnischer", "healthcare"},
	{"anästhesietechnische", "healthcare"},
	{"anästhesietechnischer", "healthcare"},

	// Education.
	{"-erzieher", "education"},
	{"-lehrer", "education"},
	{"-lehrkraft", "education"},
	{"-lehrkräfte", "education"},
	{"-pädagoge", "education"},
	{"-dozent", "education"},
	{"-tagesmutter", "education"},
	{"pädagogische fachkraft", "education"},
	{"pädagogische fachkräfte", "education"},
	{"pädagogischer mitarbeiter", "education"},
	{"pädagogische mitarbeiterin", "education"},

	// Personal services.
	{"-reinigungskraft", "personal_services"},
	{"-reinigungskräfte", "personal_services"},
	{"-reinigungsmitarbeiter", "personal_services"},
	{"-reiniger", "personal_services"},
	{"-friseur", "personal_services"},
	{"-kosmetiker", "personal_services"},
	{"-hausmeister", "personal_services"},
	{"-hauswirtschafter", "personal_services"},
	{"-hauswirtschaftskraft", "personal_services"},
	{"-haushaltshilfe", "personal_services"},

	// Engineering.
	{"-bauingenieur", "industrial_engineering"},
	{"-elektroingenieur", "industrial_engineering"},
	{"-maschinenbauingenieur", "industrial_engineering"},
	{"-verfahrensingenieur", "industrial_engineering"},
	{"-konstrukteur", "engineering_design"},
	{"-bauzeichner", "engineering_design"},
	{"technischer zeichner", "engineering_design"},
	{"technische zeichnerin", "engineering_design"},
	{"-fachplaner", "engineering_design"},
	{"-elektroplaner", "engineering_design"},
	{"-tga-planer", "engineering_design"},

	// The trades.
	{"-mechatroniker", "skilled_trades"},
	{"-elektroniker", "skilled_trades"},
	{"-elektriker", "skilled_trades"},
	{"-elektrofachkraft", "skilled_trades"},
	{"-mechaniker", "skilled_trades"},
	{"-monteur", "skilled_trades"},
	{"-installateur", "skilled_trades"},
	{"-schlosser", "skilled_trades"},
	{"-schweißer", "skilled_trades"},
	{"-lackierer", "skilled_trades"},
	{"maler", "skilled_trades"},
	{"malerin", "skilled_trades"},
	{"-malermeister", "skilled_trades"},
	{"-schreiner", "skilled_trades"},
	{"-tischler", "skilled_trades"},
	{"-zimmerer", "skilled_trades"},
	{"-dachdecker", "skilled_trades"},
	{"-maurer", "skilled_trades"},
	{"-fliesenleger", "skilled_trades"},
	{"-bodenleger", "skilled_trades"},
	{"-estrichleger", "skilled_trades"},
	{"-parkettleger", "skilled_trades"},
	{"-raumausstatter", "skilled_trades"},
	{"-trockenbauer", "skilled_trades"},
	{"-metallbauer", "skilled_trades"},
	{"-klempner", "skilled_trades"},
	{"-spengler", "skilled_trades"},
	{"-glaser", "skilled_trades"},
	{"-stuckateur", "skilled_trades"},
	{"-gerüstbauer", "skilled_trades"},
	{"-straßenbauer", "skilled_trades"},
	{"-tiefbauer", "skilled_trades"},
	{"-kanalbauer", "skilled_trades"},
	{"-rohrleitungsbauer", "skilled_trades"},
	{"-heizungsbauer", "skilled_trades"},
	{"-anlagenbauer", "skilled_trades"},
	{"-karosseriebauer", "skilled_trades"},
	{"-fahrzeugbauer", "skilled_trades"},
	{"-werkzeugmacher", "skilled_trades"},
	{"-zerspaner", "skilled_trades"},
	{"-dreher", "skilled_trades"},
	{"-fräser", "skilled_trades"},
	{"-servicetechniker", "skilled_trades"},
	{"-haustechniker", "skilled_trades"},
	{"-kundendiensttechniker", "skilled_trades"},
	{"-wartungstechniker", "skilled_trades"},
	{"-gebäudetechniker", "skilled_trades"},
	{"-kältetechniker", "skilled_trades"},
	{"-bauleiter", "skilled_trades"}, // the site manager, filed with the trades
	{"-bauleitung", "skilled_trades"},
	{"-polier", "skilled_trades"},

	// Manufacturing.
	{"-produktionshelfer", "manufacturing"},
	{"-produktionsmitarbeiter", "manufacturing"},
	{"-baumaschinenführer", "skilled_trades"}, // the construction-site plant operator
	{"-maschinenführer", "manufacturing"},
	{"-maschinenbediener", "manufacturing"},
	{"-anlagenführer", "manufacturing"},
	{"-produktionsplaner", "manufacturing"},

	// Logistics.
	// The excavator and crane drivers work a construction site.
	{"-baggerfahrer", "skilled_trades"},
	{"-kranfahrer", "skilled_trades"},
	{"-fahrer", "logistics"},
	{"-lokführer", "logistics"},
	{"-triebfahrzeugführer", "logistics"},
	{"-staplerführer", "logistics"},
	{"-lagerist", "logistics"},
	{"-lagermitarbeiter", "logistics"},
	{"-lagerhelfer", "logistics"},
	{"-lagerarbeiter", "logistics"},
	{"-kommissionierer", "logistics"},
	{"-zusteller", "logistics"},
	{"-kurier", "logistics"},
	{"-logistiker", "logistics"},
	{"-umzugshelfer", "logistics"},
	{"-tourenplaner", "logistics"},
	{"-personaldisponent", "recruiting"}, // a staffing agency's recruiter, before "-disponent"
	{"-disponent", "logistics"},
	{"fachkraft für lagerlogistik", "logistics"},
	{"fachkräfte für lagerlogistik", "logistics"},
	{"lagerlogistik", "logistics"},

	// Hospitality.
	{"-koch", "hospitality"},
	{"köchin", "hospitality"},
	{"-küchenchef", "hospitality"},
	{"-küchenhilfe", "hospitality"},
	{"-küchenhelfer", "hospitality"},
	{"-küchenmitarbeiter", "hospitality"},
	{"-kellner", "hospitality"},
	{"-servicekraft", "hospitality"},
	{"-spülkraft", "hospitality"},
	{"-restaurantfachmann", "hospitality"},
	{"-restaurantfachfrau", "hospitality"},
	{"-hotelfachmann", "hospitality"},
	{"-hotelfachfrau", "hospitality"},
	{"-barkeeper", "hospitality"},

	// Retail.
	{"-verkäufer", "retail"},
	{"-kassierer", "retail"},
	{"-verkaufskraft", "retail"},
	{"-marktleiter", "retail"},
	{"-verkaufshilfe", "retail"},
	{"verkauf", "retail"},

	// Finance, ahead of the office clerk.
	{"-kreditsachbearbeiter", "finance"},
	{"-buchhalter", "finance"},
	{"buchhaltung", "finance"},
	{"-steuerberater", "finance"},
	{"-steuerfachangestellte", "finance"},
	{"-steuerfachangestellter", "finance"},
	{"-steuerfachwirt", "finance"},
	{"-wirtschaftsprüfer", "finance"},
	{"-finanzberater", "finance"},
	{"-finanzierungsberater", "finance"},
	{"-vermögensberater", "finance"},
	{"-anlageberater", "finance"},
	{"-bankkaufmann", "finance"},
	{"-bankkauffrau", "finance"},

	// Law.
	{"-rechtsanwaltsfachangestellte", "legal"},
	{"-notarfachangestellte", "legal"},
	{"-anwalt", "legal"},
	{"-anwältin", "legal"},
	{"-jurist", "legal"},
	{"-notar", "legal"},
	{"-justiziar", "legal"},

	// People.
	{"-personalberater", "recruiting"},
	{"-personalreferent", "hr"},
	{"-personalsachbearbeiter", "hr"},
	{"-personalleiter", "hr"},
	{"-personalleitung", "hr"},

	// The office.
	{"-sachbearbeiter", "administration"},
	{"-bürokaufmann", "administration"},
	{"-bürokauffrau", "administration"},
	{"-büroassistent", "administration"},
	{"-bürokraft", "administration"},
	{"-verwaltungsfachangestellte", "administration"},
	{"-verwaltungsfachangestellter", "administration"},
	{"-verwaltungsangestellte", "administration"},
	{"-verwaltungsmitarbeiter", "administration"},
	{"-sekretär", "administration"},
	{"-sekretärin", "administration"},
	{"-empfangsmitarbeiter", "administration"},
	{"-rezeptionist", "administration"},
	{"-industriekaufmann", "administration"},
	{"-industriekauffrau", "administration"},
	{"-teamassistenz", "administration"},
	{"-vorstandsassistenz", "administration"},
	{"-projektassistenz", "administration"},
	{"büromanagement", "administration"},
	{"assistenz der geschäftsführung", "administration"},
	{"assistent der geschäftsführung", "administration"},

	// Projects.
	{"-projektleiter", "project_management"},
	{"-projektleitung", "project_management"},
	{"-projektmanager", "project_management"},
	{"-projektkoordinator", "project_management"},

	// Sales.
	{"-kundenberater", "sales"},
	{"-vertriebsmitarbeiter", "sales"},
	{"-außendienstmitarbeiter", "sales"},
	{"-vertriebspartner", "sales"},
	{"-vertriebsleiter", "sales"},
	{"-verkaufsleiter", "sales"},
	{"-agenturleiter", "sales"}, // an insurer's agency
	{"-versicherungsvertreter", "sales"},
	{"-handelsvertreter", "sales"},
	{"-vertriebler", "sales"},
	{"-makler", "sales"}, // Immobilienmakler, Versicherungsmakler

	// Support.
	{"-kundenservicemitarbeiter", "support"},
	{"kundenservice", "support"},

	// Science and design.
	{"-laborant", "science"},
	{"-laboranten", "science"},
	{"-chemiker", "science"},
	{"-biologe", "science"},
	{"-mediengestalter", "design"},
	{"-grafiker", "design"},
	{"-fotograf", "creative"},
}

// germanSeniorityTable lists the German grade aliases in precedence order.
var germanSeniorityTable = []aliasEntry{
	{"-begleiter", categoryNone},
	{"-halbleiter", categoryNone},
	{"-anleiter", categoryNone}, // the Praxisanleiter trains apprentices
	{"-anleitung", categoryNone},
	{"-begleitung", categoryNone}, // a Kindergartenbegleitung accompanies, it does not lead
	{"geschäftsführer", "c_level"},
	{"geschäftsführerin", "c_level"},
	{"-chefarzt", "c_level"},
	{"-chefärztin", "c_level"},
	{"vorstand", "c_level"},
	// The trainee grades outrank the lead heads: a "Praktikant Bauleitung" is an
	// intern in site management, not its lead.
	{"-praktikum", "intern"},
	{"-praktikant", "intern"},
	{"-werkstudent", "intern"},
	{"werkstudierende", "intern"},
	{"werkstudierender", "intern"},
	{"ausbildung", "intern"},
	{"azubi", "intern"},
	{"azubis", "intern"},
	{"auszubildende", "intern"},
	{"auszubildender", "intern"},
	{"lehrling", "intern"},
	{"duales studium", "intern"},
	{"dualer student", "intern"},
	{"duale studentin", "intern"},
	{"leitender", "lead"},
	{"leitende", "lead"},
	{"-leiter", "lead"},
	{"-leitung", "lead"},
	{"-vorarbeiter", "lead"},
	{"-schichtführer", "lead"},
	{"-oberarzt", "senior"},
	{"-oberärztin", "senior"},
	{"-assistenzarzt", "junior"},
	{"-assistenzärztin", "junior"},
	{"berufseinsteiger", "junior"},
	{"-absolvent", "junior"},
}

// germanFillerPhrases contain a grade word of the German table without stating a
// grade: a requirement ("with completed training") and the executive's assistant.
// They are cut before the German grade match, as gradeBlindPhrases are before the
// main one.
var germanFillerPhrases = []string{
	"abgeschlossener ausbildung", "abgeschlossene ausbildung", "abgeschlossenen ausbildung",
	"ohne ausbildung", "nach der ausbildung", "nach ausbildung",
	"assistenz der geschäftsführung", "assistent der geschäftsführung",
	"assistentin der geschäftsführung", "referent der geschäftsführung",
	"referentin der geschäftsführung",
}

// genderMarker is the bracketed gender notice German and Dutch titles carry —
// "(m/w/d)", "(w|m|d)", "(m/v/x)", "(v/m)". It is cut before the German match so that it cannot split a
// phrase: "Assistenz (m/w) der Geschäftsführung" is still that phrase.
var genderMarker = regexp.MustCompile(`\(\s*[mwdfxv](?:\s*[/|,·]\s*[mwdfxv])+\s*\)`)

// assistantOfLead is the assistant OF a lead — "Assistenz der Lagerleitung",
// "Assistent:in der Abteilungsleitung", "Assistenz Projektleitung" — which states no
// lead grade. It is cut before the German grade match.
var assistantOfLead = regexp.MustCompile(`\bassisten(?:z|t|tin|t:in|t\*in|t/in)?\s+(?:(?:der|des|dem|zur|im)\s+)?\p{L}*(?:leitung|leiters?|leiterin)\b`)

// foldGermanSeniority prepares a lowercased title for the German grade match: the
// gender notice is cut, then every phrase that carries a grade word without stating
// a grade.
func foldGermanSeniority(lower string) string {
	lower = strings.Join(strings.Fields(genderMarker.ReplaceAllString(lower, " ")), " ")
	for _, phrase := range germanFillerPhrases {
		lower = strings.ReplaceAll(lower, phrase, " ")
	}
	return assistantOfLead.ReplaceAllString(lower, " ")
}

// matchHeads resolves an already-lowercased title against a head table (German, or
// Dutch in dutch.go): the first
// entry (in table order) that is a whole-word phrase of the title or the resolved head
// of one of its words. A word resolves to the first head it ends in, so a veto entry
// claims the word and contributes nothing. categoryNone is served as "".
func matchHeads(lower string, table []aliasEntry) string {
	lower = strings.Join(strings.Fields(genderMarker.ReplaceAllString(lower, " ")), " ")
	resolved := map[int]bool{}
	for _, w := range strings.FieldsFunc(lower, func(r rune) bool { return !unicode.IsLetter(r) && r != '-' }) {
		for _, part := range strings.Split(w, "-") {
			if i := headOf(part, table); i >= 0 {
				resolved[i] = true
			}
		}
		// A hyphenated compound ("TGA-Planer") is also tried whole.
		if strings.Contains(w, "-") {
			if i := headOf(w, table); i >= 0 {
				resolved[i] = true
			}
		}
	}
	for i, e := range table {
		var hit bool
		if strings.HasPrefix(e.alias, "-") {
			// A head veto claims its word and contributes nothing, so it never ends
			// the walk — another word of the title may still resolve.
			hit = resolved[i] && e.canonical != categoryNone
		} else {
			hit = wordmatch.Contains(lower, e.alias, wordmatch.UnicodeBoundary)
		}
		if hit {
			if e.canonical == categoryNone {
				return ""
			}
			return e.canonical
		}
	}
	return ""
}

// headOf returns the index of the first head entry the word ends in, or -1. The fused
// feminine endings are folded once, so "erzieherin" and "erzieherinnen" find
// "-erzieher".
func headOf(word string, table []aliasEntry) int {
	if word == "" {
		return -1
	}
	forms := []string{word}
	if s, ok := strings.CutSuffix(word, "innen"); ok && s != "" {
		forms = append(forms, s)
	} else if s, ok := strings.CutSuffix(word, "in"); ok && s != "" {
		forms = append(forms, s)
	}
	for i, e := range table {
		head, ok := strings.CutPrefix(e.alias, "-")
		if !ok {
			continue
		}
		for _, f := range forms {
			if strings.HasSuffix(f, head) {
				return i
			}
		}
	}
	return -1
}
