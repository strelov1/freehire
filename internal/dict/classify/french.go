package classify

import (
	"regexp"
	"strings"
)

// frenchWorkplaceTable lists the WORKPLACES a title names — the bank, the insurer, the
// hotel, the shop, the haulier — read after frenchCategoryTable and only on a title that
// names no technical work (frenchTechTitle). A workplace says what a generic head does
// there ("Conseiller banque", "Agent hôtel"), but on a technical title it is the client's
// sector: "Tech Lead Java - Transport, secteur public", "Data Engineer - Assurance".
var frenchWorkplaceTable = []aliasEntry{
	{"hotel de ville", categoryNone}, // the town hall
	{"credit", "finance"},
	{"bancaire", "finance"},
	{"banque", "finance"},
	{"assurance", "finance"},
	{"assurances", "finance"},
	{"transport", "logistics"},
	{"restaurant", "hospitality"},
	{"hotel", "hospitality"},
	{"magasin", "retail"},
	{"boutique", "retail"},
	{"supermarche", "retail"},
	{"hypermarche", "retail"},
	{"grande distribution", "retail"},
	{"laboratoire", "science"},
	{"pharmacie", "healthcare"},
	{"ehpad", "healthcare"},
}

// frenchTechTitle is a French title naming technical work, whose workplace word is the
// client's sector rather than the job.
var frenchTechTitle = regexp.MustCompile(`\b(?:developpeur|developpement|logiciel|logiciels|informatique|si|it|data|donnees|cloud|ia|cyber\w*|devops|sap|erp|crm|dba|reseau|reseaux|systeme|systemes|web|digital|numerique|tech|java|python)\b`)

// frenchGenderNotice is the gender notice French postings append or weave in: "(H/F)",
// "H/F", "F/H", "(F/H/X)", "H/F/NB", "(H-F)", "M/F", "(h/f/d)".
var frenchGenderNotice = regexp.MustCompile(`\(?\b[hfm]\s*[/\-|]\s*[hfm](?:\s*[/\-|]\s*(?:x|n|nb|d))?\b\)?`)

// frenchInlineGender is the feminine ending a word carries after a point médian, a dot,
// a slash or inside a parenthesis — "développeur·se", "ingénieur.e", "chargé(e)",
// "technicien/ne", "directeur/trice", "conseiller·ère·s", "fonctionnel(le)". It is cut, so
// the masculine word before it matches. The dot is held to the endings that are never
// a file extension or a version ("Vue.js", ".NET").
var (
	frenchInlineGender = regexp.MustCompile(`(\p{L})(?:[·•]|\.|/\s?|-)(?:e|es|ne|nes|le|les|se|euse|euses|rice|rices|trice|trices|ère|ere|ères|eres|ice|fe|ve|sse)(?:[·•.]s)?\b`)
	frenchParenGender  = regexp.MustCompile(`(\p{L})\((?:e|es|ne|le|se|euse|rice|trice|ère|ere|ice|fe|ve|s|sse)\)s?`)
)

// frenchWordFold maps a feminine or plural title word onto the masculine the table
// carries. Its endings are the regular ones: -euse → -eur ("vendeuse"), -trice → -teur
// ("directrice"), -ienne → -ien ("technicienne"), -ière → -ier ("infirmière"), -eure →
// -eur ("ingénieure"), -ée → -é ("chargée"), -elle → -el ("fonctionnelle"), -ale → -al
// ("commerciale"). The irregular ones are listed. A word only folds when the result is
// still a word, so the fold is safe on the place and company names a title carries.
func frenchWordFold(w string) string {
	if m, ok := frenchIrregularFold[w]; ok {
		return m
	}
	for _, s := range frenchSuffixFold {
		if strings.HasSuffix(w, s[0]) && len(w) > len(s[0])+2 {
			return strings.TrimSuffix(w, s[0]) + s[1]
		}
	}
	return w
}

var frenchSuffixFold = [][2]string{
	{"euse", "eur"},
	{"trice", "teur"},
	{"ienne", "ien"},
	{"iere", "ier"},
	{"eure", "eur"},
	{"ee", "e"},
	{"elle", "el"},
	{"ale", "al"},
}

var frenchIrregularFold = map[string]string{
	"assistante":     "assistant",
	"assistantes":    "assistant",
	"consultante":    "consultant",
	"enseignante":    "enseignant",
	"debutante":      "debutant",
	"alternante":     "alternant",
	"gerante":        "gerant",
	"surveillante":   "surveillant",
	"accompagnante":  "accompagnant",
	"intervenante":   "intervenant",
	"commercante":    "commercant",
	"representante":  "representant",
	"agente":         "agent",
	"experte":        "expert",
	"avocate":        "avocat",
	"apprentie":      "apprenti",
	"bouchere":       "boucher",
	"boulangere":     "boulanger",
	"menagere":       "menager",
	"hotesse":        "hote",
	"maitresse":      "maitre",
	"cheffe":         "chef",
	"cheffes":        "chef",
	"administrative": "administratif",
	"educative":      "educatif",
	"gouvernante":    "gouvernant",
	"preposee":       "prepose",
}

// foldFrenchGender cuts the gender notices and inline feminine endings out of a
// lowercased title, keeping its accents: the main table is asked again of it.
func foldFrenchGender(lower string) string {
	lower = strings.NewReplacer("&#39;", "'", "&amp;", "&", "’", "'").Replace(lower)
	lower = frenchParenGender.ReplaceAllString(lower, "$1")
	lower = frenchInlineGender.ReplaceAllString(lower, "$1")
	lower = frenchGenderNotice.ReplaceAllString(lower, " ")
	return strings.Join(strings.Fields(lower), " ")
}

// foldFrench prepares a lowercased title for the French tables: the gender forms cut
// (foldFrenchGender), the accents dropped, and every word folded onto its masculine
// singular (frenchWordFold).
func foldFrench(lower string) string {
	s := foldSpanish(foldFrenchGender(lower))
	var b strings.Builder
	start := -1
	flush := func(end int) {
		if start >= 0 {
			b.WriteString(frenchWordFold(s[start:end]))
			start = -1
		}
	}
	for i, r := range s {
		if r >= 'a' && r <= 'z' {
			if start < 0 {
				start = i
			}
			continue
		}
		flush(i)
		b.WriteRune(r)
	}
	flush(len(s))
	return b.String()
}

// looksFrench reports whether a title shows it is written in French: a French gender
// notice, an accented letter French uses, an elided article, or a French function
// word or contract term. A point médian counts only between two letters
// ("développeur·se"): some feeds separate an English title's parts with a bullet
// ("Maintenance Planner • Austin"). "Stage" is not one: Dutch and English titles carry it too
// ("Data Stage Consultant", "Launch Engineer, Stage 0"). Many of the table's words are also English ones —
// "commercial", "assistant", "agent", "boutique", "restaurant", "production" — and an
// English title the main table left unresolved must not be read as French.
var looksFrenchRe = regexp.MustCompile(`[éèêàçùâîôûëï]|\b[hf]\s*/\s*[hf]\b|\b(?:d|l|qu)'\p{L}|\b(?:de|des|du|et|en|pour|au|aux|chez|avec|sur)\b|\b(?:cdi|cdd|alternance|alternant|stagiaire|apprenti|interim)\b|\p{L}[·•]\p{L}|\((?:e|ne|euse|trice|rice)\)`)

func looksFrench(lower string) bool {
	return looksFrenchRe.MatchString(strings.ReplaceAll(lower, "’", "'")) || frenchOnlyHead.MatchString(foldSpanish(lower))
}

// frenchOnlyHead is a head noun no other language reaching the fallback spells this
// way, so a title written in capitals without its accents ("DEVELOPPEUR PYTHON",
// "INGENIEUR RESEAU") still counts as French.
var frenchOnlyHead = regexp.MustCompile(`\b(?:developpeur|developpeuse|ingenieur|ingenieure|technicien|technicienne|conseiller|conseillere|vendeur|vendeuse|responsable|chargee|gestionnaire|infirmier|infirmiere|preparateur|cariste|magasinier|serveur|serveuse|cuisinier|comptable|secretaire|employe|ouvrier|caissier|caissiere|electricien|mecanicien|commerciale|commerciaux|auxiliaire|aide-soignant|aide-soignante|educateur|educatrice|animateur|animatrice|formateur|formatrice|enseignant|enseignante|directeur|directrice|chauffeur|livreur|coiffeur|coiffeuse|logistique|informatique|alternance|stagiaire|juriste|barbier|avocat|medecin|kinesitherapeute|plombier|soudeur|macon|couvreur|menuisier|operateur|operatrice|agente|assistante|analyste|architecte|administrateur|administratrice|consultante|testeur|receptionniste|negociateur|acheteur|approvisionneur|projeteur|dessinateur|automaticien|logisticien|manutentionnaire|plongeur|veterinaire|opticien|pharmacien|psychologue|orthophoniste|chimiste|laborantin|metallier|chaudronnier|carrossier|chauffagiste|frigoriste)\b`)

// frenchChefOf is the French "chef de/d'/des/du X": in English, and in the main
// table, a chef cooks, but French makes the word a head for any lead — "Chef de projet",
// "Chef d'équipe", "Chef de rayon", "Chef de chantier". Only the kitchen's own (partie,
// cuisine, rang, salle, bar, brigade, cave, pâtisserie, réception) keep the main table's
// reading; every other one is rewritten to "cheffe", which the main table does not
// carry and foldFrench maps back to "chef" for the French table.
var frenchChefOf = regexp.MustCompile(`\bchef((?:/fe|\(fe\)|[·.]fe)?\s+(?:d['’]\s*|de\s+(?:la\s+|l['’]\s*)?|des\s+|du\s+))(\p{L}+)`)

var frenchKitchenField = map[string]bool{
	"cuisine": true, "partie": true, "rang": true, "salle": true, "bar": true,
	"brigade": true, "cave": true, "pâtisserie": true, "patisserie": true,
	"réception": true, "reception": true, "plonge": true, "cuisines": true,
}

// frenchCFO is the French electrician's "CFO" — courants forts, the power wiring beside
// "CFA", courants faibles — which the main table reads as the chief financial officer:
// "Technicien de maintenance CFO/CFA", "Conducteur de travaux CFO". It is rewritten only
// on a French title that also names electrical or building work (frenchElectricalWork),
// so "Directeur financier (CFO)" stays the executive. Its boundaries are spelled out
// because RE2's \b is ASCII-only and would never sit before an "é".
var (
	frenchCFO            = regexp.MustCompile(`\bcfo\b`)
	frenchElectricalWork = regexp.MustCompile(`(?:^|[^\p{L}\p{N}_])(?:cfa|maintenance|multitechnique|[ée]lectri\p{L}*|travaux|chantier|technicien\p{L}*|[ée]tudes?|affaires|courants?|ing[ée]nieur\p{L}*|r[ée]f[ée]rent\p{L}*)(?:$|[^\p{L}\p{N}_])`)
)

// unchefFrench rewrites a French non-kitchen "chef de X" so the main table's "chef" (a
// cook) does not read it, and a French electrician's "CFO" so it does not read the
// executive (frenchCFO).
func unchefFrench(lower string) string {
	lower = strings.ReplaceAll(lower, "&#39;", "'") // an apostrophe some feeds leave escaped
	if strings.Contains(lower, "cfo") && looksFrench(lower) && frenchElectricalWork.MatchString(lower) {
		lower = frenchCFO.ReplaceAllString(lower, "courant fort")
	}
	if !strings.Contains(lower, "chef") {
		return lower
	}
	return frenchChefOf.ReplaceAllStringFunc(lower, func(m string) string {
		sub := frenchChefOf.FindStringSubmatch(m)
		if frenchKitchenField[sub[2]] {
			return m
		}
		return "cheffe" + sub[1] + sub[2]
	})
}

// matchFrenchCategory resolves the category of a lowercased title the other tables
// left unresolved: the main table once more on the gender-cut title, then the French
// table on the folded one. A blind alias answers categoryNone.
func matchFrenchCategory(lower string) string {
	if !looksFrench(lower) {
		return ""
	}
	c := ""
	if g := foldFrenchGender(lower); g != lower {
		c = matchOrdered(g, categoryTable)
	}
	if c != "" {
		return c
	}
	folded := foldFrench(lower)
	if c = matchOrdered(folded, frenchCategoryTable); c == "" && !IsTech(lower) && !frenchTechTitle.MatchString(folded) {
		c = matchOrdered(folded, frenchWorkplaceTable)
	}
	return c
}

// frenchSeniorityTable lists the French grade aliases, whole-word phrases on the folded
// title, in precedence order.
//
// The trainee grades lead, as in German and Spanish: "Stage - Chef de projet" is an
// intern, and so is an apprentice — French "alternance" is a work-study contract a
// student holds beside their degree, the closest this vocabulary has being the intern.
// "Confirmé" is the experienced rung between junior and senior; "Débutant" and "jeune
// diplômé" the junior. The team's own lead is the "chef d'équipe"; "Chef de projet" is
// a desk, not a grade, as "Project Manager" is none, and "responsable", "manager",
// "directeur" and "expert" state none either. Only the general, executive and financial
// directors are the board.
var frenchSeniorityTable = []aliasEntry{
	{"president directeur general", "c_level"},
	{"pdg", "c_level"},
	{"directeur general", "c_level"},
	{"directeur executif", "c_level"},
	{"directeur financier", "c_level"},
	{"directeur administratif et financier", "c_level"},
	{"stagiaire", "intern"},
	{"stagiaires", "intern"},
	{"stage", "intern"},
	{"alternance", "intern"},
	{"alternant", "intern"},
	{"alternants", "intern"},
	{"apprenti", "intern"},
	{"apprentis", "intern"},
	{"apprentissage", "intern"},
	{"contrat de professionnalisation", "intern"},
	{"contrat pro", "intern"},
	{"pfe", "intern"},
	{"chef d'equipe", "lead"},
	{"chef de groupe", "lead"},
	{"responsable d'equipe", "lead"},
	{"manager d'equipe", "lead"},
	{"confirme", "middle"},
	{"debutant", "junior"},
	{"jeune diplome", "junior"},
}

// assistantOfFrenchLead is the assistant OF a lead or an executive — "Assistant(e) de
// direction générale", "Assistante du Directeur Général", "Adjoint au chef d'équipe" —
// which states no grade of its own. It is cut before the French grade match.
var assistantOfFrenchLead = regexp.MustCompile(`\b(?:assistant|secretaire|adjoint|bras droit)\s+(?:(?:de\s+(?:la\s+)?|du\s+|au\s+|a\s+la\s+|des\s+)?)(?:direction generale|directeur general|pdg|chef d'equipe|chef de groupe|daf|directeur financier)\b`)

// frenchCareersDesk is a desk that RUNS the trainee programmes — "Responsable
// alternance", "Chargé de relations écoles et stages", "Gestionnaire des stages" — whose
// "alternance" or "stage" is the programme it looks after, not the holder's grade.
var frenchCareersDesk = regexp.MustCompile(`\b(?:responsable|gestionnaire|coordinateur|charge|chef de projet)\s+(?:(?:des|de l'|de la|du|de)\s+)?(?:relations? ecoles?(?:\s+et\s+(?:stages?|alternance))?|alternance|stages|pole alternance)\b`)

// englishStage is the English "stage" a French reading must not take for an internship:
// "Early-stage", "Seed Stage", "Stage Manager", "Backstage".
var englishStage = regexp.MustCompile(`\b(?:early|seed|late|growth|multi|any|pre-?seed|series [a-d])[\s-]stage\b|\bstage[\s-](?:manager|hand|door|gate|technician|crew)\b`)

// matchFrenchSeniority resolves the grade of a lowercased title the other tables left
// unresolved.
func matchFrenchSeniority(lower string) string {
	if !looksFrench(lower) {
		return ""
	}
	folded := foldFrench(englishStage.ReplaceAllString(lower, " "))
	folded = frenchCareersDesk.ReplaceAllString(folded, " ")
	folded = assistantOfFrenchLead.ReplaceAllString(folded, " ")
	return matchOrdered(folded, frenchSeniorityTable)
}
