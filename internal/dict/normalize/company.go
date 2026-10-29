package normalize

import (
	"regexp"
	"slices"
	"strings"
	"unicode"
)

// legalSuffixes are the corporate-form words dropped from the tail of a company name before
// two names are compared. This is the module's ONLY such list — three packages used to keep
// their own and disagreed, silently, in both directions.
//
// Matched as whole trailing WORDS reduced to their ASCII letters, so one entry covers every
// spelling of a form: "bv" catches "BV", "B.V." and "b.v.". That is also why there is no entry
// for the punctuated Nordic and Romance forms ("A/S", "S.A.") — reducing the word to its
// letters already makes them "as" and "sa".
//
// A TOKEN EARNS ITS PLACE IN THE DATA, not by looking safe. The test is not whether a word
// seems dangerous but whether stripping it lands on a DIFFERENT existing employer:
//
//   - "co" was once excluded for colliding with ordinary words. All 297 catalogue companies
//     whose slug ends in it are "& Co." forms, and of the 25 largest merges the wider tokens
//     produce, every one lands on the right employer (Accenture GmbH → Accenture, Goldman
//     Sachs & Co. → Goldman Sachs).
//   - "pte" is small — 7 companies — and earns its place on one of them: without it,
//     "epam systems pte. ltd." keys apart from the EPAM Systems that holds 1,172 open jobs.
//   - "pvt" is the South-Asian private-company form and nothing else: every catalogue name
//     ending in it (measured 2026-10-27) is an Indian "Pvt", "Pvt Ltd" or "(Pvt) Ltd"
//     employer, and before it was listed "Zoho Corporation Pvt" keyed apart from Zoho.
//   - The European and Latin-American forms on the second line (freehire#3876) were measured
//     2026-10-29 against the companies an unsuffixed spelling already holds: Microsoft
//     s.r.o., Accenture Kft., JYSK d.o.o., Airbus S.A.S., Accenture LTDA and Davita SAS each
//     keyed apart from their own employer. Of the ~1,000 merges they produce, the wrong ones
//     sampled are the generic-name kind every form risks ("Hit d.d." onto HIT Inc, "SOLVO
//     S.A.S" onto a Russian Солво), not a word the form shares with ordinary names. Measured
//     and LEFT OUT for exactly that: "ad" (the Serbian a.d., but "Canopy A&D" reads as it),
//     "sc" (Mexican S.C., and the South Carolina at the end of a US place name), "pt"
//     (Indonesian PT, and physical therapy), "sia" (seven companies, none merged, and a name).
//     "oo" is the tail of the Polish "Sp. z o.o." — see [formQualifiers] for the rest of it.
//
// The letters-only comparison sees only ASCII, so a form spelled with other letters is listed
// in [accentedForms] instead: the Estonian "OÜ" reads "o" here, and a lone "o" is no form.
//
// "Private" is the same form written out, and it is NOT listed, because unlike "pvt" it is an
// ordinary word: "Julius Baer Private" and "Private Equity Partners" are names, not forms. It
// is stripped only as half of the compound "Private Limited" — see [formQualifiers].
//
// Two tails that look like they belong here and deliberately do not — they live in
// [boardNameTails] instead, where over-trimming costs a lookup rather than an employer:
//
//   - "spa" is the Italian S.p.A. AND the literal word. "Hilton Luxor Resort & Spa" carries
//     more open jobs than any genuine S.p.A. in the catalogue, so stripping it would merge a
//     hotel into a resort chain — the collision the excluded-"co" comment feared, in the one
//     place it is real.
//   - "group" is not a corporate form at all, it is part of a brand. Every "X Group" / "X"
//     pair in the catalogue does look like one employer (Thales, Securitas, Bosch), but
//     "group" is an ordinary word by which two businesses legitimately differ, so merging on
//     it is a judgement. Judgements belong to cmd/merge-companies, which shows a dry run,
//     elects by job count and records a reversible alias — not to a pure function applied
//     silently to every new posting.
var legalSuffixes = map[string]struct{}{
	"corporation": {}, "limited": {}, "gmbh": {}, "corp": {}, "llc": {}, "ltd": {},
	"inc": {}, "incorporated": {}, "plc": {}, "llp": {}, "lp": {}, "cic": {}, "cio": {},
	"srl": {}, "pty": {}, "pte": {}, "pvt": {}, "bv": {}, "nv": {}, "aps": {},
	"ab": {}, "ag": {}, "kg": {}, "oy": {}, "sa": {}, "as": {}, "se": {}, "co": {},
	"doo": {}, "jdoo": {}, "dd": {}, "sro": {}, "kft": {}, "zrt": {}, "nyrt": {}, "ood": {},
	"eood": {}, "ead": {}, "shpk": {}, "oyj": {}, "asa": {}, "uab": {}, "ou": {}, "ehf": {},
	"sl": {}, "ltda": {}, "lda": {}, "sas": {}, "sarl": {}, "sagl": {}, "bvba": {}, "ug": {},
	"bhd": {}, "oo": {},
}

// CompanySlug is [Slug] with any trailing corporate forms removed: "Arch Capital Group Ltd."
// becomes "arch-capital-group". A Japanese form is the exception — it is moved to the tail as a
// Latin mark rather than removed, "株式会社Speee" becoming "speee-kk"; [japaneseForms] argues
// why. An empty or untransliterable name yields "".
//
// Stripping repeats rather than stopping at one match: compound forms are ordinary ("Acme
// GmbH & Co. KG", "Atlassian Pty Ltd"), and a single pass would leave half the form behind.
//
// The match runs on the name's own words rather than on [Slug]'s output, because Slug has
// already turned "B.V." into two word breaks ("booking-b-v") where no form can be recognised.
// Reducing each word to its ASCII letters instead sees the form whatever its punctuation.
//
// A single-word name is never stripped: "Limited" stays "limited", because an empty slug
// silently matches nothing while a visibly odd company row can be found and fixed.
func CompanySlug(name string) string { return trimTail(name, IsLegalFormBefore, true) }

// formQualifiers are the words that are a corporate form only as the first half of a compound,
// directly before the form it qualifies: the Indian and Sri Lankan "Private Limited", and its
// abbreviation "(P) Ltd".
//
// They cannot join [legalSuffixes], which strips a word wherever it ends a name, because both
// are ordinary words in their own right. "Private" ends real names — "Julius Baer Private",
// "William Blair - Private" — and stripping it there would fold a private bank onto its parent;
// "(P)" alone ends "Milton Roy India (P)", where nothing says it is a form at all. Before a
// stripped "Limited" neither reading is open: "Acme Private Limited" is the company form
// 1,671 catalogue companies carry (2026-10-27), and keying it at `acme-private` split every
// one of them from its own unsuffixed spelling.
//
// The qualified form must itself have been STRIPPED, not merely be the next word, so "Private
// Bank" and "Private Limited Partners" keep their "Private".
//
// The same rule carries the other multi-word forms, each word keyed to the one it may precede:
// the Malaysian "Sdn Bhd", and the Polish "Sp. z o.o." with its spelled-out "Spółka z o.o."
// and the half-glued "Sp.z o.o.". Since each word needs the next one already stripped, the walk
// takes "o.o.", then "z", then "Sp." — and "Generation Z" keeps its "Z". The spaced "o. o." is
// closed up before the walk (see [CloseSpacedForms]). "Sp. z.o.o." is not reached: "z.o.o." is
// the letters "zoo", which ends real names.
var formQualifiers = map[string]map[string]struct{}{
	"private": {"limited": {}, "ltd": {}},
	"p":       {"limited": {}, "ltd": {}},
	"sdn":     {"bhd": {}},
	"z":       {"oo": {}},
	"sp":      {"z": {}},
	"spka":    {"z": {}}, // "Spółka", whose "ó" and "ł" the letters-only comparison drops
	"spolka":  {"z": {}}, // the same word as sources write it in ASCII
	"spz":     {"oo": {}},
}

// IsLegalFormBefore is [IsLegalForm] for a word read in its place: next is the word right after
// it, which the caller has ALREADY stripped as a form ("" when word is the last word, or the
// word after it was kept). It answers yes for everything IsLegalForm does, and for a
// [formQualifiers] word standing directly before a form it qualifies.
//
// It is exported for the callers that walk a name's tail with their own word breaks and must
// stay on the company key's vocabulary (merge-companies' nameWords, mailmatch's stripLegalForm,
// collections' significantFields) — each after [CloseSpacedForms]:
// a walk that asked IsLegalForm alone would leave "Private" behind where the key drops it.
func IsLegalFormBefore(word, next string) bool {
	if IsLegalForm(word) {
		return true
	}
	if next == "" {
		return false
	}
	_, qualified := formQualifiers[letters(word)][letters(next)]
	return qualified
}

// boardNameTails are the tails an ATS board id drops that the company key must keep. They are
// a SECOND vocabulary rather than entries in [legalSuffixes] because they are not corporate
// forms, and the two are read at opposite prices: dropping one from a board-id guess costs a
// probe against a board that simply does not answer, while dropping it from the company key
// re-keys an employer silently, on every posting, forever. See legalSuffixes' own note above
// for the evidence on each — a resort that folds onto a hotel chain, and a brand word two
// businesses legitimately differ by.
//
// Matched by the same letters-only comparison, so one entry covers every spelling: "spa"
// catches "SpA" and "S.p.A." alike. That punctuated form is why this is judged on a name's
// WORDS rather than on a slug's segments — [Slug] has already turned "S.p.A." into "s-p-a",
// where no tail can be recognised.
var boardNameTails = map[string]struct{}{
	"group": {}, "spa": {},
}

// IsBoardNameTail reports whether a single word is a tail an ATS board id commonly drops
// ("Group", "S.p.A."). It deliberately does NOT answer for corporate forms — those are
// [IsLegalForm]'s vocabulary — so a caller that wants both asks both, and neither list can
// grow into the other by accident.
//
// It is exported for the callers that hold a slug rather than a name and must judge one
// segment of it on its own (cmd/harvest-ats, over a platform profile slug like `doodle-ag`).
func IsBoardNameTail(word string) bool {
	_, ok := boardNameTails[letters(word)]
	return ok
}

// BoardNameSlug is [CompanySlug] widened by [boardNameTails]: the board id a company's name
// suggests, rather than the key the company is filed under. "Acme S.p.A." yields `acme` where
// CompanySlug yields `acme-s-p-a`.
//
// Use it only to PROPOSE a board id to cmd/harvest-boards, which keeps a candidate solely on
// live evidence — a slug that happens to be another employer's board is rejected by the probe
// and the name gate, not accepted on the resemblance. It must never key a company.
func BoardNameSlug(name string) string {
	return trimTail(name, func(word, next string) bool {
		return IsLegalFormBefore(word, next) || IsBoardNameTail(word)
	}, false)
}

// trimTail is the shared walk under [CompanySlug] and [BoardNameSlug]: drop trailing words the
// caller's vocabulary recognises, then slug what is left. The two differ only in that
// vocabulary, and sharing the walk is what keeps their single-word and repeat rules identical.
//
// keepJapaneseMark decides what becomes of a Japanese form cut from either end: the company key
// keeps it as one Latin mark at the tail (see [japaneseForms]), a board-id guess drops it.
//
// isTail is asked about each word together with the word after it that the walk has just
// dropped ("" for the last word), which is what lets a compound form like "Private Limited"
// be recognised without either half being a form on its own.
func trimTail(name string, isTail func(word, next string) bool, keepJapaneseMark bool) string {
	rest, mark := trimJapaneseForms(name)
	rest = CloseSpacedForms(rest)
	words := strings.FieldsFunc(dropApostrophes(rest), isWordBreak)
	next := ""
	for len(words) > 1 && isTail(words[len(words)-1], next) {
		next = words[len(words)-1]
		words = words[:len(words)-1]
	}
	if keepJapaneseMark && mark != "" {
		words = append(words, mark)
	}
	return Slug(strings.Join(words, " "))
}

// CloseSpacedForms closes up the Polish "o.o." written with a space inside it, "Sp. z o. o.",
// which 82 catalogue companies carry (2026-10-29). Split there, it is two words of the letter
// "o", and a lone "o" cannot be a form — it would strip the end of any name with an initial.
// Closing the gap before the walk turns it into the "o.o." [formQualifiers] already reads.
// It is anchored to the end of the name (past a closing bracket, "Acme (Sp. z o. o.)") and
// needs the "z" before it, so only that tail moves: the spaced Balkan "d. o. o." would
// otherwise close up to "d o.o.", which reads as no form.
//
// Every walk over a name's tail calls it before splitting the name into words, for the same
// reason each asks [IsLegalFormBefore]: a walk that skipped it would count "o." "o." as two
// name words where the company key drops them.
func CloseSpacedForms(name string) string {
	return spacedPolishTail.ReplaceAllString(name, "${1}o.o.")
}

var spacedPolishTail = regexp.MustCompile(`(?i)([\s\p{Zs}.]z[\s\p{Zs}]+)o\.[\s\p{Zs}]+o\.?[\s\p{Zs})\]]*$`)

// dropApostrophes removes the possessive/elision marks a name carries so they cannot become
// word breaks in the key. "Kohl's" is one word and belongs at `kohls`, which is also what a
// person types; [Slug] alone renders it `kohl-s`.
//
// It matters more than it looks. Once the canonical slug is DERIVED from a name rather than
// chosen among stored slugs, that artefact can no longer lose to the clean sibling spelling —
// it elected `kohl-s` over `kohls` across 2,939 postings. Both the ASCII and the typographic
// mark are covered, since sources use either.
func dropApostrophes(name string) string {
	return strings.NewReplacer("'", "", "\u2019", "", "\u02bc", "").Replace(name)
}

// isWordBreak reports whether a rune separates two words of a company name. Everything [Slug]
// would drop separates, EXCEPT "." and "/" — those two live inside the very forms this is
// looking for ("B.V.", "A/S"), so splitting on them would hide the form instead of finding it.
//
// Whitespace alone is not enough: "Sun Technologies,Inc." is how aggregators routinely write
// the name, and 13,730 companies in the catalogue carry a form with no space before it.
// Splitting here rather than on Slug's output is also what keeps "Foo.com" one word.
func isWordBreak(r rune) bool {
	return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '.' && r != '/'
}

// japaneseForms are the Japanese corporate forms, written out and abbreviated, each with the
// Latin mark it becomes in the company key. They are a separate list from [legalSuffixes]
// because they cannot be matched the same way: the letters-only comparison reduces a kanji
// word to nothing, and Japanese writes no space between the form and the name, so
// "株式会社サイバーエージェント" is a single word to the tail walk. They are matched as a
// literal PREFIX or SUFFIX of the name — Japanese puts the form on either side
// ("株式会社リクルート", "トヨタ自動車株式会社") — and both sides key alike.
//
// Before this list the form reached [Slug] whole and was romanised as Chinese: 株式会社
// became "zhu-shi-hui-she", measured 2026-10-26 on ~12k companies (hrmos, herp, engage,
// mynavi), and one employer written with the form before and after its name keyed as two.
//
// THE FORM IS KEPT AS A MARK, NOT DROPPED, unlike every Latin form — and the data is why. A
// token earns its place by whether stripping it lands on a DIFFERENT employer, and these fail
// that test where the Latin ones pass it: once the form is gone, a small Japanese firm's name
// romanises into a short Latin word some unrelated company already owns. Dropping it put
// 726 Japanese companies on someone else's key — 株式会社タケダ on Takeda's, 株式会社COMPASS on
// Compass's, 株式会社Walk on a Danish ApS. `takeda-kk` keeps the variants of one Japanese
// employer together and every foreign namesake apart.
//
// One mark per form, because the forms are different legal entities: 有限会社Foo and
// 株式会社Foo are not one company spelled two ways.
//
// Only the business forms. The non-profit and institutional ones (医療法人, 学校法人,
// 一般社団法人) are deliberately absent: they compound ("社会医療法人社団") in ways a literal
// list does not cover, and together they key fewer than 300 companies.
var japaneseForms = []struct{ form, mark string }{
	{"株式会社", "kk"}, {"(株)", "kk"}, {"（株）", "kk"}, {"㈱", "kk"},
	{"有限会社", "yk"}, {"(有)", "yk"}, {"（有）", "yk"}, {"㈲", "yk"},
	{"合同会社", "gk"}, {"(同)", "gk"}, {"（同）", "gk"},
	{"合名会社", "gmk"}, {"合資会社", "gsk"},
}

// trimJapaneseForms cuts [japaneseForms] from either end of a name, repeatedly, with the spaces
// around them — half-width and the full-width U+3000 that Japanese text uses — and returns
// what is left with the mark of the first form it cut ("" when it cut none).
//
// Like the tail walk, it never empties a name: "株式会社" alone is returned as it is, unmarked,
// because an empty slug silently matches nothing while a visibly odd row can be found and fixed.
func trimJapaneseForms(name string) (rest, mark string) {
	const spaces = " \t\u3000"
	name = strings.Trim(name, spaces)
	for {
		trimmed, cut := name, ""
		for _, f := range japaneseForms {
			if r, ok := strings.CutPrefix(trimmed, f.form); ok {
				trimmed, cut = r, f.mark
			}
			if r, ok := strings.CutSuffix(trimmed, f.form); ok {
				trimmed, cut = r, f.mark
			}
		}
		trimmed = strings.Trim(trimmed, spaces)
		if trimmed == name || trimmed == "" {
			return name, mark
		}
		if mark == "" {
			mark = cut
		}
		name = trimmed
	}
}

// IsLegalForm reports whether a single word of a company name is a corporate form ("Ltd.",
// "GmbH", "B.V."), by the same letters-only comparison [CompanySlug] strips with.
//
// It is exported for callers that need the judgement without the slug — collections'
// RequireCountry counts how many WHITESPACE tokens of a register name are the name proper,
// and must not use [CompanySlug]'s word breaks, which split "T-Mobile" in two and would make
// a single-token name look specific enough to skip its headquarters check.
func IsLegalForm(word string) bool {
	if slices.ContainsFunc(japaneseForms, func(f struct{ form, mark string }) bool { return f.form == word }) {
		return true
	}
	if _, ok := legalSuffixes[letters(word)]; ok {
		return true
	}
	if _, ok := accentedForms[strings.ToLower(strings.TrimFunc(word, notLetter))]; ok {
		return true
	}
	return isGluedForms(word)
}

// accentedForms are the corporate forms whose letters are not all ASCII, matched as the whole
// lowercased word past any punctuation around it. [letters] cannot carry them: it keeps ASCII
// only, so "OÜ" reads "o". Folding accents inside letters instead was measured and refused
// (2026-10-29): it transliterates Cyrillic too, and "ООО "САС"" and "ООО "АГ"" both became
// `ooo` — two employers on one key — while initials like "С.А." started reading as forms.
var accentedForms = map[string]struct{}{"oü": {}}

// notLetter trims what may surround an [accentedForms] word — "OÜ.", "(OÜ)" — the way
// [letters] ignores it for every other form.
func notLetter(r rune) bool { return !unicode.IsLetter(r) }

// isGluedForms reports whether a word is several forms joined by dots with no space, "Pvt.Ltd."
// or "Pte.Ltd" — one word to the tail walk, since a dot is not a word break (it lives inside
// "B.V."), and as letters "pvtltd", which no entry names. It is a form when EVERY dotted part
// is one, read right to left as the walk reads words, so "Pvt.Ltd" and "P.Ltd" qualify while
// "Foo.com" does not. The punctuated single forms never reach this: "B.V." is already "bv".
func isGluedForms(word string) bool {
	parts := strings.FieldsFunc(word, func(r rune) bool { return r == '.' })
	if len(parts) < 2 {
		return false
	}
	next := ""
	for i := len(parts) - 1; i >= 0; i-- {
		if !IsLegalFormBefore(parts[i], next) {
			return false
		}
		next = parts[i]
	}
	return true
}

// letters lowercases a word down to its ASCII letters, so the punctuated and bare spellings
// of a legal form ("B.V.", "BV", "Ltd.") compare as one. It runs on the raw word rather than
// on its slug because Slug turns "B.V." into "b-v" — two tokens, matching no form — while
// leaving a name like "Foo.com" intact, which stripping dots globally would not.
func letters(word string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(word) {
		if r >= 'a' && r <= 'z' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// CompanyKey folds a company name to a comparable core: [CompanySlug] with the word breaks
// removed as well, so two sources that separate the words differently still agree.
//
// This is a comparison key, not a display or URL key — unlike [Slug] it joins the words with
// nothing and strips legal forms, both of which would be wrong in a path segment.
func CompanyKey(name string) string {
	return FoldSlug(CompanySlug(name))
}

// FoldSlug removes a slug's word breaks, turning it into the comparison key two sources that
// separated the words differently both land on: "dollar-tree" and "dollartree" fold alike.
//
// It is the same fold jobs.company_slug_folded stores (migration 0109) and the same one
// company_slug_aliases.folded_key is keyed by, which is what lets a spelling nobody ever
// merged reach a canon its folded form already owns. Take [CompanyKey] when you hold a NAME;
// this is for when the slug is already in hand.
func FoldSlug(slug string) string {
	return strings.ReplaceAll(slug, "-", "")
}

// SameCompany reports whether two company names denote the same employer.
//
// The comparison is deliberately mild — case, punctuation, script and a trailing corporate
// form are noise between one source's label and another's. It is equally deliberately not
// fuzzy: substring or prefix matching would treat every employer whose name contains a short
// common word as the same company ("Base" would match "Basecamp"). Two names that both fold
// to nothing are not a match, since that says nothing about either.
func SameCompany(a, b string) bool {
	ka := CompanyKey(a)
	return ka != "" && ka == CompanyKey(b)
}
