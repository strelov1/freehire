package location

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// germanWorkModePhrases is the German counterpart of descriptionWorkModePhrases, built
// from the German ATS crawl (coveto, rexx, dvinci, xing, concludis, onlyfy, kenjo:
// ~124k open postings on 2026-10-25, 99% of them with no work mode at all).
//
// The German register differs from the English one in a way that decides the ORDER.
// English prose names the arrangement ("hybrid role", "fully remote"). German prose
// mostly lists home working as a BENEFIT: "Homeoffice möglich", "2 Tage Homeoffice",
// "mobiles Arbeiten" in a perks list. Each of these offers home working beside an
// office, which is hybrid. Remote is the rarer statement, and German writes it with an
// explicit totality ("100% Homeoffice", "vollständig remote", "ausschließlich im
// Homeoffice"). A remote phrase therefore CONTAINS a hybrid one ("100% Homeoffice
// möglich" holds "Homeoffice möglich"), so remote is checked first here, the reverse of
// the English list. Onsite is last and fills only an explicit refusal of home working.
//
// Every phrase is matched against the folded text (tags out, entities decoded,
// whitespace collapsed, see foldMarkup). German postings lean on list markup for their
// benefits, so "<li>Homeoffice</li><li>möglich" style splits and "&amp;" are common.
// A phrase must end on a word boundary: "mobile Arbeit" must not match "mobile
// Arbeitsmaschinen", a trade ("Mechatroniker für mobile Arbeitsmaschinen").
//
// Absent on purpose:
//   - bare "remote": it is mostly "Remote-Support" or "Fernwartung/Remote", a technical
//     service. Only "remote" bound to an arrangement word counts.
//   - bare "hybrid"/"hybride": "hybride Umgebungen" (IT), "hybride Projekte" (method),
//     "die hybride Arbeitswelt" (a company's product). Only the work-model forms count.
//   - "vor Ort", "deutschlandweit", "bundesweit", "Präsenz": each says where the job is
//     or that the employer is national, never the arrangement on its own.
var germanWorkModePhrases = []struct {
	mode    string
	phrases []string
}{
	{"remote", []string{
		"100% homeoffice", "100 % homeoffice", "100% home-office", "100 % home-office",
		"100% home office", "100 % home office",
		"100% mobiles arbeiten", "100 % mobiles arbeiten", "100% mobil", "100 % mobil",
		"vollständig remote", "komplett remote", "ausschließlich remote", "voll remote",
		"rein remote", "full remote", "remote-position", "remote-stelle",
		"vollständig im homeoffice", "komplett im homeoffice", "ausschließlich im homeoffice",
		"vollständig aus dem homeoffice", "komplett aus dem homeoffice",
		"ausschließlich aus dem homeoffice",
		"vollständig von zu hause", "komplett von zu hause", "ausschließlich von zu hause",
		"vollständig von zuhause", "komplett von zuhause", "ausschließlich von zuhause",
		"vollständig mobil", "ausschließlich mobil",
		"deutschlandweit remote", "bundesweit remote", "remote aus ganz deutschland",
		"remote in ganz deutschland",
	}},
	{"hybrid", []string{
		"homeoffice möglich", "home-office möglich", "home office möglich",
		"homeoffice-möglichkeit", "homeoffice-möglichkeiten", "homeoffice möglichkeit",
		"homeoffice möglichkeiten", "homeofficemöglichkeit", "homeofficemöglichkeiten",
		"home-office-möglichkeit", "home-office-möglichkeiten",
		"möglichkeit zum homeoffice", "möglichkeit auf homeoffice", "möglichkeit für homeoffice",
		"möglichkeit zu homeoffice", "möglichkeit im homeoffice", "möglichkeit, im homeoffice",
		"teilweise im homeoffice", "tageweise im homeoffice", "anteilig im homeoffice",
		"homeoffice-anteil", "homeofficeanteil", "homeoffice anteil",
		"anteiliges homeoffice", "anteiligem homeoffice", "anteilig homeoffice",
		"homeoffice nach absprache", "homeoffice-tage", "homeoffice tage",
		"tage homeoffice", "tage im homeoffice", "tag homeoffice", "tag im homeoffice",
		"homeoffice-option", "homeoffice-optionen", "homeoffice option", "homeoffice optionen",
		"homeoffice-regelung", "homeoffice-regelungen",
		"mobiles arbeiten", "mobilem arbeiten", "mobilen arbeiten", "mobile arbeit",
		"mobiles working", "mobile working", "mobile-office", "mobile office", "mobiles office",
		"hybrides arbeiten", "hybride arbeit", "hybrides arbeitsmodell", "hybride arbeitsmodelle",
		"hybriden arbeitsmodell", "hybrider arbeitsplatz", "hybrides arbeitszeitmodell",
		"hybride arbeitszeitmodelle", "hybridmodell", "hybrid zu arbeiten", "hybrid arbeiten",
		"arbeitsmodell: hybrid", "hybride arbeitsweise",
		"remote-anteil", "remoteanteil", "remote anteil", "teilweise remote", "anteilig remote",
		"remote-tage", "remote möglich", "remote-arbeit möglich",
		"telearbeit",
	}},
	{"onsite", []string{
		"kein homeoffice", "kein home-office", "kein home office", "keine homeoffice",
		"homeoffice ist nicht möglich", "homeoffice nicht möglich",
		"homeoffice ist leider nicht möglich", "homeoffice ist in dieser position leider nicht möglich",
		"homeoffice ist in dieser position nicht möglich",
		"kein mobiles arbeiten", "ausschließlich vor ort",
		// Not "100% vor Ort": its one live hit was a FIRST MONTH "mit einem Pensum von 80 bis
		// 100 % vor Ort" before hybrid work opens up, the trial-period shape that also kept
		// "fully on-site" out of remoteDenialPhrases.
	}},
}

// germanNegations, found just before a remote or hybrid phrase, turn it into its
// opposite ("kein Homeoffice möglich", "nicht 100% remote"). The onsite family carries
// its own negations as whole phrases, so a negated match is simply skipped here and the
// scan goes on to the onsite family (see germanPhraseMatches for the one onsite phrase
// negation does apply to).
var germanNegations = []string{"kein ", "keine ", "nicht ", "ohne "}

// germanNegationWindow is how far before a phrase a negation still governs it: one or
// two short words ("kein reines", "nicht"). Wider, and an unrelated earlier clause would
// cancel a statement that stood on its own.
const germanNegationWindow = 12

// germanRefusals, found just AFTER a remote or hybrid phrase, withdraw it ("Mobiles
// Arbeiten ist in dieser Position nicht möglich"). The match is then passed over rather
// than read as onsite: onsite is reserved for the explicit refusals in the onsite family,
// which carry their own wording ("Homeoffice ist nicht möglich").
var germanRefusals = []string{"nicht möglich", "nicht vorgesehen", "nicht angeboten",
	"ausgeschlossen", "leider nicht"}

// germanRefusalWindow is how far past a phrase a refusal still governs it: a short
// predicate ("ist in dieser Position leider nicht möglich"). The window also stops at the
// end of the sentence, so "Mobiles Arbeiten ist möglich. Homeoffice ist nicht möglich."
// keeps its offer.
const germanRefusalWindow = 40

// refusedAfter reports whether a germanRefusals phrase follows end in the same sentence.
// "nicht ausgeschlossen" (not ruled out) is an offer, not a refusal.
func refusedAfter(text string, end int) bool {
	tail := text[end:min(len(text), end+germanRefusalWindow)]
	if i := strings.IndexAny(tail, ".;!?"); i >= 0 {
		tail = tail[:i]
	}
	for _, r := range germanRefusals {
		i := strings.Index(tail, r)
		if i < 0 {
			continue
		}
		if r == "ausgeschlossen" && strings.HasSuffix(tail[:i], "nicht ") {
			continue
		}
		return true
	}
	return false
}

// germanWorkModeTriggers gate the fold: only a description carrying one of these stems
// can match any phrase above, and foldMarkup is not free on the ~99% that carry none.
// Each is ONE word, so neither a tag nor an entity can split it ("vor<br>Ort",
// "Home&nbsp;Office"); the two-word phrases are only ever matched on the folded text.
var germanWorkModeTriggers = []string{"home", "hause", "mobil", "hybrid", "remote",
	"telearbeit", "ausschließlich"}

// germanWorkMode derives a work mode from German arrangement prose, "" when none of
// germanWorkModePhrases is stated. lower is the description already lowercased.
func germanWorkMode(desc, lower string) string {
	if !containsAny(lower, germanWorkModeTriggers) {
		return ""
	}
	text := foldMarkup(desc)
	for _, wm := range germanWorkModePhrases {
		for _, p := range wm.phrases {
			if germanPhraseMatches(text, p, wm.mode) {
				return wm.mode
			}
		}
	}
	return ""
}

// germanPhraseMatches reports whether p occurs in text as a whole phrase (the rune after
// it is not a letter), read for the family mode it belongs to. A remote or hybrid
// occurrence is passed over when a germanNegations word precedes it or a germanRefusals
// phrase follows it, and the scan continues, so a posting that refuses home working in one
// sentence and offers it in another still reads the offer.
//
// The onsite family IS the refusals ("kein Homeoffice"), so negation does not apply to it
// — except to the one onsite phrase that is not itself a refusal, "ausschließlich vor Ort",
// which "nicht ausschließlich vor Ort" turns around. And an onsite phrase must not run on
// into a compound: "Kein Homeoffice-Verbot" says there is no ban.
func germanPhraseMatches(text, p, mode string) bool {
	refusal := mode == "onsite" && (strings.HasPrefix(p, "kein") || strings.Contains(p, "nicht"))
	offset := 0
	for {
		i := strings.Index(text[offset:], p)
		if i < 0 {
			return false
		}
		start, end := offset+i, offset+i+len(p)
		offset = end
		r, size := utf8.DecodeRuneInString(text[end:])
		if end < len(text) && unicode.IsLetter(r) {
			continue
		}
		if mode == "onsite" && r == '-' {
			if next, _ := utf8.DecodeRuneInString(text[end+size:]); unicode.IsLetter(next) {
				continue
			}
		}
		if !refusal && containsAny(text[max(0, start-germanNegationWindow):start], germanNegations) {
			continue
		}
		if mode != "onsite" && refusedAfter(text, end) {
			continue
		}
		return true
	}
}
