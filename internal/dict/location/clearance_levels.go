package location

import (
	"regexp"
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/dict/vocab"
	"github.com/strelov1/freehire/internal/platform/htmltext"
)

// ClearanceLevelsFromDescription names the security-clearance levels a job description
// states, in vocab.ClearanceLevelValues order. It returns nil exactly when
// RequiresClearanceFromDescription is false, so the two columns can never disagree: a
// posting either states a clearance (and carries at least one level) or carries none.
//
// The ladder is the US one (confidential, secret, top_secret, ts_sci, public_trust)
// plus interim, a STATUS stated beside a level ("interim Secret required to start"),
// and other. A posting may carry several: "Active Secret, must be able to obtain
// TS/SCI" is both.
//
// Everything outside that ladder is "other":
//
//   - Non-US schemes (UK SC/DV/eDV/BPSS/CTC, AU Baseline/NV1/NV2/PV, NATO) do not line
//     up rung for rung (UK SC is not US Secret), so folding them into US chips would
//     assert an equivalence no vetting agency makes, while a ladder each would put a
//     dozen near-empty chips in the filter.
//   - A posting that states a clearance but names no level ("an active security
//     clearance is required") must not land under "no detected clearance requirement",
//     which would contradict requires_clearance.
//
// Each rung is matched only in an anchored form (a following "clearance", a leading
// "active"/"interim"/"dod", or the value of a clearance field), never as a bare word:
// "secret" alone is "trade secrets" or "secret management", and "confidential" alone
// is "kept confidential".
func ClearanceLevelsFromDescription(desc string) []string {
	text := strings.ToLower(htmltext.ToText(desc))
	flat := strings.Join(strings.Fields(text), " ")
	if !requiresClearanceText(text, flat) {
		return nil
	}
	text, flat = levelText(text), levelText(flat)
	found := map[string]bool{}
	for _, r := range clearanceLevelRules {
		// Flattened text too: a field written without a colon puts its label and value
		// on separate lines ("Clearance Level\nSecret"). The line-broken form is read
		// first because its negation check stops at line breaks, the narrower reading.
		if r.assertedIn(text) || r.assertedIn(flat) {
			found[r.level] = true
		}
	}
	fields := clearanceFieldValues(flat)
	for _, value := range fields {
		for _, r := range clearanceLevelRules {
			if r.assertedIn(value) {
				found[r.level] = true
			}
		}
		// Inside a clearance field "interim" needs no further anchor: "Clearance:
		// Active Secret (Interim OK)".
		if containsWord(value, "interim") {
			found[vocab.ClearanceInterim] = true
		}
	}
	if !hasLadderLevel(found) || namesForeignScheme(text, fields) {
		found[vocab.ClearanceOther] = true
	}
	levels := make([]string, 0, len(found))
	for _, l := range vocab.ClearanceLevelValues {
		if found[l] {
			levels = append(levels, l)
		}
	}
	return levels
}

// levelText prepares stripped text for the rung rules. Quotes and emphasis go, since
// "a “Secret” security clearance" and a bolded "*secret*" hide the rung from the
// patterns. "without polygraph" goes because it QUALIFIES the rung ("TS/SCI without
// polygraph" is TS/SCI) and negatedSentence would read its "without" as a denial.
func levelText(s string) string {
	return withoutPolygraph.ReplaceAllString(levelNoise.Replace(s), " ")
}

var (
	// "u.s." folds to "us" so its periods cannot read as sentence ends ("an interim
	// and/or final U.S. Secret clearance" would break the interim rule's run).
	levelNoise       = strings.NewReplacer("“", "", "”", "", "\"", "", "‘", "", "’", "", "*", "", "u.s.", "us")
	withoutPolygraph = regexp.MustCompile(`\b(?:with or )?without (?:an? )?(?:ci |fs |full[- ]scope |lifestyle )?poly(?:graph)?\b`)
)

// clearanceLevelRule is one rung's anchored forms. mask lists the rungs whose matches
// are blanked out before this one is tried, so "Top Secret/SCI" is ts_sci only and
// does not also read as top_secret or, through its last word, as secret.
//
// cues are substrings every match of pattern contains. A description that holds none
// of them is skipped without running the regexp: the patterns open on word
// alternations RE2 cannot prefilter, and a posting is usually several kilobytes.
type clearanceLevelRule struct {
	level   string
	cues    []string
	pattern *regexp.Regexp
	mask    []*regexp.Regexp
}

// assertedIn reports whether the rung is stated anywhere in text outside a sentence
// that denies it and outside a field label whose value denies it.
func (r clearanceLevelRule) assertedIn(text string) bool {
	if !slices.ContainsFunc(r.cues, func(c string) bool { return strings.Contains(text, c) }) {
		return false
	}
	for _, m := range r.mask {
		text = m.ReplaceAllStringFunc(text, func(s string) string { return strings.Repeat(" ", len(s)) })
	}
	for _, loc := range r.pattern.FindAllStringIndex(text, -1) {
		if deniedLabel(text, loc[1]) || levelDenied(text, loc[0], loc[1]) {
			continue
		}
		return true
	}
	return false
}

var (
	tsSCIPattern = regexp.MustCompile(
		`\b(?:ts|top[ -]secret) ?(?:/{1,2} ?|- ?|(?:(?:security )?clearance )?(?:with (?:an? )?)?)sci\b|\bsci (?:clearance|eligib|access)|` +
			`\btop secret ?/ ?sensitive compartmented\b`)
	topSecretPattern = regexp.MustCompile(`\btop[ -]secret\b|\bts (?:security )?clearance\b`)
	// secret needs an anchor on one side: a leading word ("active", "dod", "obtain and
	// maintain a", "clearance level") or a trailing one ("clearance", with
	// "security"/"us"/"dod" between; levelText folds "u.s." to "us") or ladder phrasing
	// ("secret or higher", "secret/top secret"). "nato secret" is deliberately not an
	// anchor: NATO's scheme is "other" (see foreignSchemePhrases).
	secretPattern = regexp.MustCompile(
		`\b(?:active|interim|current|final|existing|dod|dow|defense|issued|us|government|level 2|level of|obtain|maintain|hold|possess)(?: an?)? secret\b|` +
			`\bclearance(?: (?:level|type|requirement|required|required to start|for the role|of))?:? secret\b|` +
			`\(secret\)|\bsecret(?: (?:security|dod|level|government|us))* clearance\b|` +
			`\bsecret (?:or (?:higher|above|top)|eligib)|\bsecret[ -]level\b|\bsecret ?/ ?top\b`)
	confidentialPattern = regexp.MustCompile(
		`\bconfidential(?: (?:security|level))* clearance\b|\bconfidential or (?:higher|above)\b`)
	// public trust is a US suitability determination, not a clearance: "obtain a public
	// trust", "public trust determination". The bare pair stays out for the reason
	// clearancePhrases gives.
	publicTrustPattern = regexp.MustCompile(
		`\bpublic trust(?: (?:security|level|position))* (?:clearance|background|investigation|determination|level|or higher)\b|` +
			`\bpublic trust/other required\b|\bclearance(?: level)? public trust\b|\(public trust\)|\bpublic trust \(tier|\bpublic trust eligib|` +
			`\bobtain (?:and maintain )?(?:an? )?(?:client-issued )?(?:position of )?public trust\b|` +
			`\bclearance at the public trust level\b|\b(?:dhs|doj|irs|va|cbp|federal|moderate|high|low) public trust\b`)
	// interim reaches its clearance through a short run of qualifiers ("an interim
	// and/or final U.S. Secret clearance"). The run stops at a sentence or clause break
	// so the job-title sense ("Interim IT Director") cannot borrow a later clearance.
	interimPattern = regexp.MustCompile(
		`\binterim\b[^.;\n]{0,40}?\b(?:clearance|secret|ts|sci)\b|\b(?:secret|clearance)\s*\(interim\)`)
)

var clearanceLevelRules = []clearanceLevelRule{
	{level: vocab.ClearanceTSSCI, cues: []string{"sci"}, pattern: tsSCIPattern},
	{
		level: vocab.ClearanceTopSecret, cues: []string{"top secret", "top-secret", "ts "},
		pattern: topSecretPattern, mask: []*regexp.Regexp{tsSCIPattern},
	},
	{
		level: vocab.ClearanceSecret, cues: []string{"secret"},
		pattern: secretPattern, mask: []*regexp.Regexp{tsSCIPattern, topSecretPattern},
	},
	{level: vocab.ClearanceConfidential, cues: []string{"confidential"}, pattern: confidentialPattern},
	{level: vocab.ClearancePublicTrust, cues: []string{"public trust"}, pattern: publicTrustPattern},
	{level: vocab.ClearanceInterim, cues: []string{"interim"}, pattern: interimPattern},
}

// clearanceFieldValues returns the value of every clearance field in flat that does
// not deny the requirement, trimmed to the field itself, with a bare rung rewritten
// into the anchored form the prose rules match (inside a field the label IS the
// anchor: "Clearance Type: Secret" reads as "secret clearance").
//
// Fields are found overlapping, not with FindAll: ATS forms nest them ("Security
// Clearance Type: DoD Clearance: Confidential"), and a non-overlapping scan spends the
// outer label's capture on the inner label and never sees the value. Each value is cut
// at the next colon because flattened, the next field follows straight on.
func clearanceFieldValues(flat string) []string {
	var out []string
	for offset := 0; offset < len(flat); {
		loc := clearanceLabel.FindStringSubmatchIndex(flat[offset:])
		if loc == nil {
			break
		}
		valueStart := offset + loc[2]
		// htmltext renders a bolded label as "*Clearance Type:*", so the value opens
		// with the label's closing asterisk.
		value := strings.TrimLeft(flat[valueStart:offset+loc[3]], "* ")
		if i := strings.IndexByte(value, ':'); i >= 0 {
			value = strings.TrimSpace(value[:i])
		}
		offset = valueStart
		if valueDenies(value) {
			continue
		}
		// "TS/SCI" is left alone: the rewrite would split it into "ts clearance/sci",
		// which reads as Top Secret and no longer as TS/SCI.
		if !tsSCIPattern.MatchString(value) {
			value = fieldRung.ReplaceAllString(value, "$1 clearance")
		}
		out = append(out, value)
	}
	return out
}

// fieldRung is a clearance field whose value opens with a bare rung, optionally
// qualified ("Active Secret", "DoD Top Secret", "TS", "Public Trust (MBI)").
var fieldRung = regexp.MustCompile(
	`^((?:(?:active|current|dod|us)\s+)*(?:top[ -]secret|secret|ts|confidential|public trust))\b`)

// levelDenied reports whether the rung matched at text[start:end] is denied by the
// words right around it: a negation among the few words before it ("no Secret
// clearance") or a denial straight after it ("Secret clearance: not required").
//
// It is deliberately narrower than negatedSentence, which decides WHETHER a posting
// needs a clearance. By the time a rung is read the gate has answered yes, and a
// sentence-wide reading lost true levels ("must obtain a Public Trust clearance and
// cannot start before cleared" lost Public Trust to the "cannot").
func levelDenied(text string, start, end int) bool {
	before := text[max(0, start-levelNegationWindow):start]
	if i := strings.LastIndexAny(before, ".!?;\n"); i >= 0 {
		before = before[i+1:]
	}
	words := strings.Fields(before)
	for _, w := range words[max(0, len(words)-levelNegationWords):] {
		w = strings.Trim(w, ".,;:!?()\"'*")
		if w == "no" || w == "not" || w == "never" || strings.HasSuffix(w, "n't") || strings.HasPrefix(w, "non-") {
			return true
		}
	}
	return deniedAfter.MatchString(text[end:min(len(text), end+levelNegationWindow)])
}

// levelNegationWindow and levelNegationWords bound how far levelDenied looks.
const (
	levelNegationWindow = 40
	levelNegationWords  = 4
)

// deniedAfter is a denial written straight after the rung. A trailing "clearance(s)"
// is skipped first, because the interim rule's match ends at the rung: "interim Secret
// clearances are not allowed" reads the denial only past the noun.
var deniedAfter = regexp.MustCompile(`^[\s:*,(\-]*(?:(?:security\s+)?clearances?[\s:*,(\-]*)?(?:(?:is|are|though|but|but is)\s+)?(?:not|no|none|n/a)\b`)

// deniedLabel reports whether a rung matched in prose is in fact a field LABEL whose
// value denies it (a template carrying "Public Trust/Other Required: None" beside a
// real Secret field).
func deniedLabel(text string, end int) bool {
	m := labelTail.FindStringSubmatch(text[end:])
	return m != nil && valueDenies(strings.TrimSpace(m[1]))
}

var labelTail = regexp.MustCompile(`^(?:/other)?(?:\s+[a-z]+){0,3}?\s*\**\s*:\s*([^\n]{0,30})`)

// hasLadderLevel reports whether any US-ladder rung was found. interim does not count:
// it is a status beside a rung, and "interim clearance" alone names none.
func hasLadderLevel(found map[string]bool) bool {
	return slices.ContainsFunc(vocab.ClearanceLevelValues, func(l string) bool {
		return found[l] && l != vocab.ClearanceInterim && l != vocab.ClearanceOther
	})
}

// foreignSchemePhrases are the non-US schemes clearancePhrases already recognises,
// listed again here only to sort a posting into "other", plus NATO's, which the
// requirement rule reaches through "security clearance". Nothing here can mark a
// posting the gate did not: this runs only after the gate has passed.
var foreignSchemePhrases = []string{
	"sc clearance", "sc cleared", "dv clearance", "dv cleared", "edv clearance",
	"ctc clearance", "bpss", "developed vetting", "baseline personnel security standard",
	"baseline clearance", "nv1", "nv2", "negative vetting", "positive vetting", "agsva",
	"nato secret", "nato security clearance", "nato clearance",
}

// foreignSchemeValues are the same schemes as a clearance field's bare value.
var foreignSchemeValues = []string{"sc", "dv", "edv", "ctc", "bpss", "baseline", "nv1", "nv2", "pv"}

// namesForeignScheme reports whether the posting names a non-US vetting scheme, in
// prose or as the value of one of its clearance fields.
func namesForeignScheme(text string, fields []string) bool {
	for _, p := range foreignSchemePhrases {
		if phraseAssertedOutsideLabel(text, p) {
			return true
		}
	}
	for _, value := range fields {
		for _, v := range foreignSchemeValues {
			if containsWord(value, v) {
				return true
			}
		}
	}
	return false
}
