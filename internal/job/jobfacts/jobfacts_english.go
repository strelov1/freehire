package jobfacts

import (
	"regexp"
	"strings"
)

// English-level detection. Precision-first like the matchers above: it resolves an
// explicit CEFR code or a well-known level phrase (EN + RU + PL — the Telegram
// sources are Russian-heavy, and Polish boards like NoFluffJobs/JustJoinIT state the
// requirement in Polish, e.g. "Angielski na poziomie min. B2+") and emits "" when
// nothing is stated. Every signal must sit near an English keyword, so a bare
// "B2"/"advanced"/"native" is not misread out of context ("B2B SaaS", "advanced
// degree", "native macOS app"). Values are members of vocab.EnglishLevelValues.
var (
	// reEnglishKw gates the whole parse and anchors every phrase: english_level is
	// about English, so a description that never names it yields nothing.
	reEnglishKw = regexp.MustCompile(`english|английск|angielsk`)
	// A CEFR code counts only adjacent (either order) to an English keyword.
	reCEFRForward = regexp.MustCompile(`(?:english|английск\w*)[^.\n]{0,20}\b([abc][12])\b`)
	reCEFRBack    = regexp.MustCompile(`\b([abc][12])\b[^.\n]{0,20}(?:english|английск\w*)`)
	// Polish CEFR proximity tolerates one interior "." that the EN/RU gap above
	// disallows: Polish postings near-universally phrase the requirement as "na
	// poziomie min. B2" — the abbreviation dot in "min." would otherwise break the
	// EN/RU-style no-period gap and hide an otherwise unambiguous, adjacent CEFR code.
	reCEFRForwardPl = regexp.MustCompile(`angielsk\w*[^.\n]{0,20}\.?[^.\n]{0,10}\b([abc][12])\b`)
	reCEFRBackPl    = regexp.MustCompile(`\b([abc][12])\b[^.\n]{0,10}\.?[^.\n]{0,20}angielsk\w*`)

	// Level phrases (checked for English proximity via near). The intermediate family
	// carries its prefix so "upper-intermediate"→b2 and "pre-intermediate"→a2 resolve
	// without a lookbehind (RE2 has none); the Russian "средн" and Polish "średni"
	// families mirror it via their own "above this level" prefix.
	reNative     = regexp.MustCompile(`\bnative\b|родн\w*|носител\w*`)
	reFluentAdv  = regexp.MustCompile(`fluen\w*|\badvanced\b|свободн\w*|продвинут\w*|biegł\w*|zaawansowan\w*`)
	reInterFam   = regexp.MustCompile(`\b(upper[\s-]?|pre[\s-]?)?intermediate\b`)
	reRuMidFam   = regexp.MustCompile(`(выше\s+)?средн\w*`)
	rePlMidFam   = regexp.MustCompile(`(wyższy\s+)?średni\w*`)
	reConvers    = regexp.MustCompile(`\bconversational\b|разговорн\w*|komunikatywn\w*`)
	reElementary = regexp.MustCompile(`\belementary\b|\bbeginner\b|начальн\w*|początkując\w*`)
	reBasic      = regexp.MustCompile(`\bbasic\b|базов\w*|podstawow\w*`)
	reNoEnglish  = regexp.MustCompile(`no english|english (?:is )?not required|without english|без английск\w*|bez angielsk\w*`)
)

// englishScanMaxRunes bounds how much of the description EnglishLevel's near/spanNear
// matching runs over. near/spanNear are O(#keyword-matches × #phrase-matches) in the
// worst case (no pair of matches ever falls within englishWindow of each other), so an
// unbounded, repetitive input — a scraped/SEO-padded description repeating
// "english"/CEFR-like tokens many times without ever pairing them closely — could turn
// derivation of one job into a multi-second, CPU-bound stall on the otherwise fast,
// per-job ingest path. Sized generously above any real job posting; this only ever
// engages on pathological input.
const englishScanMaxRunes = 20000

// EnglishLevel resolves the required English level from the description, returning
// one of vocab.EnglishLevelValues or "" when nothing is stated. When several levels
// are named it returns the lowest (the minimum requirement); an explicit "no English"
// phrase resolves to "none" only when no positive level is present.
func EnglishLevel(description string) string {
	s := strings.ToLower(hardRequirementText(description))
	s = truncateRunes(s, englishScanMaxRunes)
	if !reEnglishKw.MatchString(s) {
		return ""
	}

	// Scanned once and reused by every near()/spanNear() call below instead of each one
	// re-running FindAllStringIndex over the whole string — near was doing that on every
	// one of its five call sites, and the per-match loops did it once per match.
	kws := reEnglishKw.FindAllStringIndex(s, -1)

	levels := map[string]bool{}
	for _, m := range reCEFRForward.FindAllStringSubmatch(s, -1) {
		levels[m[1]] = true
	}
	for _, m := range reCEFRBack.FindAllStringSubmatch(s, -1) {
		levels[m[1]] = true
	}
	for _, m := range reCEFRForwardPl.FindAllStringSubmatch(s, -1) {
		levels[m[1]] = true
	}
	for _, m := range reCEFRBackPl.FindAllStringSubmatch(s, -1) {
		levels[m[1]] = true
	}
	if near(s, kws, reNative) {
		levels["native"] = true
	}
	if near(s, kws, reFluentAdv) {
		levels["c1"] = true
	}
	if near(s, kws, reConvers) {
		levels["b1"] = true
	}
	if near(s, kws, reElementary) {
		levels["a1"] = true
	}
	if near(s, kws, reBasic) {
		levels["a2"] = true
	}
	for _, m := range reInterFam.FindAllStringSubmatchIndex(s, -1) {
		if !spanNear(s, kws, m[0], m[1]) {
			continue
		}
		switch {
		case m[2] < 0: // no prefix group — plain "intermediate"
			levels["b1"] = true
		case strings.HasPrefix(s[m[2]:m[3]], "upper"):
			levels["b2"] = true
		case strings.HasPrefix(s[m[2]:m[3]], "pre"):
			levels["a2"] = true
		default:
			levels["b1"] = true
		}
	}
	for _, m := range reRuMidFam.FindAllStringSubmatchIndex(s, -1) {
		if !spanNear(s, kws, m[0], m[1]) {
			continue
		}
		if m[2] >= 0 { // "выше средн..." — above intermediate
			levels["b2"] = true
		} else {
			levels["b1"] = true
		}
	}
	for _, m := range rePlMidFam.FindAllStringSubmatchIndex(s, -1) {
		if !spanNear(s, kws, m[0], m[1]) {
			continue
		}
		if m[2] >= 0 { // "wyższy średni..." — upper-intermediate
			levels["b2"] = true
		} else {
			levels["b1"] = true
		}
	}

	if len(levels) == 0 {
		if reNoEnglish.MatchString(s) {
			return "none"
		}
		return ""
	}
	best := ""
	for lv := range levels {
		if best == "" || englishRank[lv] < englishRank[best] {
			best = lv
		}
	}
	return best
}

// spanNear reports whether [start,end) is within englishWindow bytes of any span,
// with no sentence boundary (. or newline) in the gap — so a level word and an
// English keyword in different sentences ("native iOS apps. English docs") don't
// bind. An overlap always counts.
func spanNear(s string, spans [][]int, start, end int) bool {
	for _, m := range spans {
		var lo, hi int
		switch {
		case start >= m[1]:
			lo, hi = m[1], start
		case m[0] >= end:
			lo, hi = end, m[0]
		default:
			return true // overlap
		}
		if hi-lo <= englishWindow && !strings.ContainsAny(s[lo:hi], ".\n") {
			return true
		}
	}
	return false
}
