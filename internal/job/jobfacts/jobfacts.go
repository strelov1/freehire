// Package jobfacts derives a job's employment type, education level, and minimum
// required experience deterministically from its title and description text. Like
// internal/dict/classify and internal/dict/location it is a curated matcher, not a model:
// it resolves explicit signals and emits nothing ("" / nil) for what it cannot
// resolve — it never guesses. Canonical enum values are members of the controlled
// vocabularies the enrichment contract defines (vocab.EmploymentTypeValues /
// vocab.EducationLevelValues).
package jobfacts

import (
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/candidate/hardconstraint/credentials"
	"github.com/strelov1/freehire/internal/dict/edulevel"
)

// reDegreeOptional matches a posting that offers a degree with an
// equivalent-experience alternative, so the hard-constraint evaluator can skip a
// false education blocker. It covers "or equivalent experience" (optionally with a
// qualifier like "work"/"practical") and the bare "degree or equivalent".
var reDegreeOptional = regexp.MustCompile(`(?i)(equivalent(?:\s+\w+){0,2}\s+experience|degree\s+or\s+equivalent)`)

// RequiredCertifications returns the canonical credential slugs the posting
// requires, scanned deterministically from the description with the shared
// credential vocabulary. Computed at read; nothing is stored.
func RequiredCertifications(description string) []string {
	return credentials.Scan(hardRequirementText(description))
}

// DegreeOptional reports whether the posting offers a degree "or equivalent
// experience", so a candidate without the degree is not falsely blocked.
func DegreeOptional(description string) bool {
	return reDegreeOptional.MatchString(description)
}

// Employment-type matchers, checked in precedence order: a "full-time internship"
// is an internship, a part-time contract is part-time, etc. "temporary" / "fixed
// term" map to contract (the closest vocabulary member). Bare \bintern\b is safe —
// the boundary keeps it out of "internal"/"international". The contract matcher also
// covers the unambiguous US-market shorthands for an independent contractor: 1099
// (the tax form) and "corp-to-corp". "consultant" is deliberately excluded — it is as
// often a full-time title as a contract arrangement.
//
// b2b/c2c also denote an independent-contractor arrangement in some markets, but the
// bare tokens collide with ubiquitous business-model prose ("B2B SaaS", "C2C
// marketplace") — so, like the bare "ms"/"bs" degree abbreviations below, they favour
// precision: reContractShorthand matches them only when an employment-context word
// sits right after ("C2C candidates", "B2B contract", "C2C only"), never standalone.
//
// A pay-policy sentence ("salaries for part-time roles will be pro-rated", and the
// full-time / permanent twin of that sentence) names a class of roles, not this
// posting's schedule, so that hit does not count. The same precision drops a legal
// instrument ("agreement contract", "breach of contract", "contract law"), a
// residency statement ("permanent resident"), and a travel perk ("temporary visa",
// "temporary housing"): the bare token would otherwise label the posting.
var (
	reInternship         = regexp.MustCompile(`\b(internship|intern|co-?op|working student|praktikum|werkstudent)\b`)
	rePartTime           = regexp.MustCompile(`\bpart[\s-]?time\b`)
	rePartTimePolicy     = regexp.MustCompile(`\bpart[\s-]?time\s+(?:roles|positions|employees|staff)\b`)
	reFullTimePolicy     = regexp.MustCompile(`\bfull[\s-]?time\s+(?:roles|positions|employees|staff)\b`)
	rePermanentPolicy    = regexp.MustCompile(`\bpermanent\s+(?:roles|positions|employees|staff)\b`)
	rePermanentStatus    = regexp.MustCompile(`\bpermanent\s+(?:resident|residency|authorization|address)\b`)
	reProrate            = regexp.MustCompile(`\bpro[\s-]?rat`)
	reContract           = regexp.MustCompile(`\b(contractor|contract|freelancer|freelance|fixed[\s-]?term|temporary|1099|corp[\s-]?to[\s-]?corp)\b`)
	reContractShorthand  = regexp.MustCompile(`\b(b2b|c2c)\b[\s:/.-]*(only|contract|contractor|candidates?|welcome|accepted|basis|engagement|employment|position|arrangement|w2)\b`)
	reContractInstrument = regexp.MustCompile(`\bagreement\s+contract\b|\bbreach\s+of\s+contract\b|\bcontract\s+law\b`)
	reTemporaryNotJob    = regexp.MustCompile(`\btemporary\s+(?:visa|accommodation|housing|permit|resident|residency)\b`)
	reFullTime           = regexp.MustCompile(`\b(full[\s-]?time|permanent)\b`)
	reFellowship         = regexp.MustCompile(`\bfellowship\b`)
	// French contract types, which a French posting states as a label ("Type de contrat :
	// CDI", "Contrat : CDD 6 mois") or in its title ("Stage - Assistant marketing",
	// "Alternance - Chargé RH"). A CDI is the permanent contract, a CDD and an intérim
	// mission are fixed-term (contract), and alternance and apprenticeship are work-study —
	// the closest member is the internship.
	//
	// The trainee words count in the TITLE only, or as a contract phrase in the body:
	// a body mentions them for everything else — "trois ans d'expérience (alternance
	// comprise)", "l'encadrement des stagiaires", "nous accueillons stagiaires et
	// alternants", "apprentissage automatique". The bare "stage" is English as well
	// ("Early Stage", "Stage Manager"), so a title counts it only as a French label.
	reTraineeTitleFR = regexp.MustCompile(`\b(stagiaire|alternance|alternante?|apprentie?|en apprentissage)\b|(?:^|[\-–|(:/]\s*)stage\b|\bstage\s*(?:$|[\-–|):/])|\bstage (?:de|d'|en)\b`)
	// reEnglishStage cuts what a title may say that is no trainee: the English stage and
	// its crafts, and French machine learning ("apprentissage automatique").
	reEnglishStage = regexp.MustCompile(`\b(?:early|seed|late|growth|multi|pre-?seed)[\s-]stage\b|\bstage[\s-](?:manager|hand|door|crew|technician|director|designer|electrician|carpenter|rigger|lighting|sound|production|management|coordinator|supervisor|combat)\b|\bapprentissage automatique\b`)
	// The phrases end on a letter RE2's \b cannot see past ("conventionné"), so the end
	// is spelled out.
	reTraineeBodyFR = regexp.MustCompile(`\b(convention de stage|stage de fin d'[ée]tudes|stage de \d+ mois|stage conventionn[ée]e?|offre de stage|contrat d'apprentissage|contrat de professionnalisation)(?:$|[^\p{L}\p{N}_])`)
	// The hours count as a label ("CDI à temps partiel", "Type d'emploi : Temps plein",
	// "Temps partiel 24h") or in the title, not as prose: "temps plein ou temps partiel",
	// "forfait jours, temps partiel…" in a perks list, "professionnels à temps partiel".
	rePartTimeFR   = regexp.MustCompile(`\b(?:cdi|cdd|poste|contrat|emploi)\s*(?:[:\-–]\s*)?(?:à |a |en )?(?:temps[ -]partiel|mi-temps)\b|\b(?:temps[ -]partiel|mi-temps)\s*(?:[(:\-–]\s*)?\d{1,2}\s*h\b`)
	reContractFR   = regexp.MustCompile(`\b(cdd|int[ée]rimaire|mission d'int[ée]rim|en int[ée]rim)\b|\bintérim\b`)
	reFullTimeFR   = regexp.MustCompile(`\bcdi\b|\b(?:cdd|poste|contrat|emploi)\s*(?:[:\-–]\s*)?(?:à |a |en )?temps[ -](?:plein|complet)\b`)
	reHoursTitleFR = regexp.MustCompile(`\b(temps[ -]partiel|mi-temps|temps[ -]plein|temps[ -]complet)\b`)
	// Polish contract types, stated as a label ("Forma zatrudnienia: umowa o pracę",
	// "Rodzaj umowy: B2B", "Wymiar etatu: pełny etat") or in the offer ("zatrudnienie w
	// oparciu o umowę zlecenie"). An "umowa o pracę" (UoP) is the employment contract and,
	// like the CDI, full time unless a part-time share is stated; B2B, the civil-law
	// "umowa zlecenie" and "umowa o dzieło", a replacement and a fixed-term contract are
	// contract; a paid internship and the "umowa o staż" are the internship.
	//
	// Each needs its contract word. A bare "B2B" is the business model as often ("sprzedaż
	// B2B", "klientów B2B"), so it counts only as the contract ("umowa B2B", "kontrakt
	// B2B", "na B2B", "forma współpracy: B2B", a salary "PLN/MD (B2B)"). A bare "staż" is
	// work experience ("nagroda za staż pracy", "staż pracy min. 3 lata"), so the body
	// counts it only in a contract phrase; the title's "Staż", "Stażysta", "Praktykant" and
	// "Praktyki" count as they stand. RE2's \b is ASCII-only, so a word ending in a Polish
	// letter spells its end out.
	reTraineeTitlePL  = regexp.MustCompile(`(?:^|[^\p{L}])(?:staż|stażyst\p{L}*|praktykant\p{L}*|praktyki|program staż\p{L}*)(?:$|[^\p{L}])`)
	reTraineeBodyPL   = regexp.MustCompile(`umow\p{L}* o staż|płatn\p{L}* staż|program\p{L}* stażow|ofert\p{L}* stażu|praktyk\p{L}* studenck`)
	rePartTimePL      = regexp.MustCompile(`niepełn\p{L}* (?:etat|wymiar)|częś\p{L}* etatu|pół etatu|\b[1-4]/[2-5] etatu|\b(?:[1-9]\d)\s?% etatu|\b0[,.]\d+ etatu|praca dodatkowa`)
	rePolishFullShare = regexp.MustCompile(`(?:^|[^\p{L}])(?:pełn\p{L}* etat|pełen etat|cały etat|pełn\p{L}* wymiar)`)
	reFullTimePL      = regexp.MustCompile(`umow\p{L}* o prac[eęy]|\buop\b|(?:^|[^\p{L}])(?:pełn\p{L}* etat|pełen etat|pełn\p{L}* wymiar\p{L}*|cały etat)|na czas nieokreślony`)
	reContractPL      = regexp.MustCompile(`umow\p{L}*[ -]zleceni|umow\p{L}* o dzieło|(?:umow\p{L}*|kontrakt|kontraktu|kontraktem|współprac\p{L}*|na|on|w oparciu o|(?:zatrudnieni\p{L}*|współprac\p{L}*|prac\p{L}*) w modelu)\s+b2b\b|(?:forma|rodzaj)\s+(?:współpracy|zatrudnienia|umowy)\s*:\s*(?:kontrakt\s+)?b2b\b|(?:pln|zł|zl|eur)(?:$|[^\p{L}])[^\n.]{0,30}\(b2b\)|umow\p{L}* na zastępstwo|czas określony|kontrakt gospodarcz`)
	// reFellowshipStaffRole excludes a "fellowship" mention that names the staff job
	// running the program rather than the fellow's own position — e.g. "Fellowship
	// Manager", "Fellowship Program Coordinator". Those are ordinary full-time/contract
	// roles, so a bare \bfellowship\b would otherwise mislabel them. The window is
	// bounded to the same line/sentence so a role word elsewhere in a long description
	// (e.g. "reports to the Program Manager") doesn't suppress a genuine fellowship.
	reFellowshipStaffRole = regexp.MustCompile(`fellowship[^\n]{0,50}\b(manager|coordinator|director|officer|administrator)\b|\b(manager|coordinator|director|officer|administrator)\b[^\n]{0,50}fellowship`)
)

// EmploymentType resolves the work arrangement from the title and description,
// returning one of vocab.EmploymentTypeValues or "" when nothing is stated. It
// never assumes full-time for an unstated posting.
func EmploymentType(title, description string) string {
	t, s := employmentText(title, description)
	switch {
	case reFellowship.MatchString(s) && !reFellowshipStaffRole.MatchString(s):
		return "fellowship"
	case reInternship.MatchString(s) || reTraineeBodyFR.MatchString(s) || frenchTraineeTitle(t) ||
		reTraineeTitlePL.MatchString(t) || reTraineeTitleBCS.MatchString(t):
		return "internship"
	}
	// A French title's own label is the advertised role's, and outranks a body that may
	// describe the employer's other roles ("Développeur en CDI" beside "notre agence
	// recrute en CDD et en intérim").
	if h := frenchHoursTitle(t); h != "" {
		return h
	}
	switch {
	case partTimeSchedule(s) || rePartTimeFR.MatchString(s) || bcsLabel(s, rePartTimeBCS):
		return "part_time"
	case reContractFR.MatchString(t):
		return "contract"
	case reFullTimeFR.MatchString(t):
		return "full_time"
	case contractSchedule(s) || reContractFR.MatchString(s) || bcsLabel(s, reContractBCS):
		return "contract"
	case fullTimeSchedule(s) || reFullTimeFR.MatchString(s) || bcsLabel(s, reFullTimeBCS):
		return "full_time"
	}
	return polishEmploymentType(t, s)
}

// polishEmploymentType reads the Polish contract words (rePartTimePL and its siblings)
// of the lowercased title t and posting s, "" when none is stated. It runs after every
// English and French reading, so a body's internship phrase ("płatny staż") cannot
// outrank an English "Full-time". A title's own share is the advertised role's; in the
// body a share counts only when no full post is offered beside it ("pełny etat lub 3/4
// etatu" is a choice, read as the employment contract).
func polishEmploymentType(t, s string) string {
	switch {
	case reTraineeBodyPL.MatchString(s):
		return "internship"
	case rePartTimePL.MatchString(t):
		return "part_time"
	case rePartTimePL.MatchString(s) && !rePolishFullShare.MatchString(s):
		return "part_time"
	case reContractPL.MatchString(s):
		return "contract"
	case reFullTimePL.MatchString(s):
		return "full_time"
	}
	return ""
}

func PartTimePayPolicyOnly(title, description string) bool {
	_, s := employmentText(title, description)
	return rePartTime.MatchString(s) && !partTimeSchedule(s)
}

func EmploymentBoilerplateKind(stored, title, description string) string {
	if stored == "part_time" {
		if PartTimePayPolicyOnly(title, description) {
			return "part_time_pay_policy"
		}
		return ""
	}
	_, s := employmentText(title, description)
	switch stored {
	case "full_time":
		if fullTimePayPolicyOnly(s) {
			return "full_time_pay_policy"
		}
		if permanentStatusOnly(s) {
			return "permanent_status"
		}
	case "contract":
		if contractBoilerplateOnly(s) {
			return "contract_boilerplate"
		}
	}
	return ""
}

func employmentText(title, description string) (string, string) {
	t := strings.ReplaceAll(strings.ToLower(title), "’", "'")
	return t, t + "\n" + strings.ReplaceAll(strings.ToLower(description), "’", "'")
}

func fullTimePayPolicyOnly(s string) bool {
	saw := false
	real := false
	eachMatch(s, reFullTime, func(at int, word string) bool {
		if word == "permanent" {
			if permanentSchedule(s, at) {
				real = true
				return true
			}
			if classPayPolicy(s, at, rePermanentPolicy) {
				saw = true
			}
			return false
		}
		if !classPayPolicy(s, at, reFullTimePolicy) {
			real = true
			return true
		}
		saw = true
		return false
	})
	return saw && !real
}

func permanentStatusOnly(s string) bool {
	saw := false
	real := false
	eachMatch(s, reFullTime, func(at int, word string) bool {
		if word != "permanent" {
			real = true
			return true
		}
		if permanentSchedule(s, at) || classPayPolicy(s, at, rePermanentPolicy) {
			real = true
			return true
		}
		saw = true
		return false
	})
	return saw && !real
}

func contractBoilerplateOnly(s string) bool {
	if contractSchedule(s) {
		return false
	}
	return reContract.MatchString(s)
}

func partTimeSchedule(s string) bool {
	found := false
	eachMatch(s, rePartTime, func(at int, _ string) bool {
		if !classPayPolicy(s, at, rePartTimePolicy) {
			found = true
			return true
		}
		return false
	})
	return found
}

func fullTimeSchedule(s string) bool {
	found := false
	eachMatch(s, reFullTime, func(at int, word string) bool {
		switch word {
		case "permanent":
			if permanentSchedule(s, at) {
				found = true
				return true
			}
		default:
			if !classPayPolicy(s, at, reFullTimePolicy) {
				found = true
				return true
			}
		}
		return false
	})
	return found
}

func eachMatch(s string, re *regexp.Regexp, fn func(at int, word string) bool) {
	from := 0
	for from <= len(s) {
		loc := re.FindStringIndex(s[from:])
		if loc == nil {
			return
		}
		at := from + loc[0]
		end := from + loc[1]
		if fn(at, s[at:end]) {
			return
		}
		if end <= from {
			from = at + 1
		} else {
			from = end
		}
	}
}

func permanentSchedule(s string, at int) bool {
	if classPayPolicy(s, at, rePermanentPolicy) {
		return false
	}
	status := rePermanentStatus.FindStringIndex(s[at:])
	return status == nil || status[0] != 0
}

func classPayPolicy(s string, at int, class *regexp.Regexp) bool {
	if !reProrate.MatchString(sentenceAround(s, at)) {
		return false
	}
	noun := class.FindStringIndex(s[at:])
	return noun != nil && noun[0] == 0
}

func sentenceAround(s string, at int) string {
	lo := strings.LastIndexAny(s[:at], ".\n")
	if lo < 0 {
		lo = 0
	} else {
		lo++
	}
	hiRel := strings.IndexAny(s[at:], ".\n")
	if hiRel < 0 {
		return s[lo:]
	}
	return s[lo : at+hiRel]
}

func contractSchedule(s string) bool {
	hits := reContract.FindAllStringIndex(s, -1)
	if len(hits) == 0 {
		return reContractShorthand.MatchString(s)
	}
	if reContractShorthand.MatchString(s) {
		return true
	}
	covers := reContractInstrument.FindAllStringIndex(s, -1)
	notJob := reTemporaryNotJob.FindAllStringIndex(s, -1)
	for _, m := range hits {
		if spanCovered(m, covers) || spanCovered(m, notJob) {
			continue
		}
		return true
	}
	return false
}

func spanCovered(span []int, covers [][]int) bool {
	for _, c := range covers {
		if span[0] >= c[0] && span[1] <= c[1] {
			return true
		}
	}
	return false
}

// frenchTraineeTitle reports whether a lowercased title names a French trainee contract.
func frenchTraineeTitle(t string) bool {
	return reTraineeTitleFR.MatchString(reEnglishStage.ReplaceAllString(t, " "))
}

// frenchHoursTitle returns the hours a lowercased French title states, "" when none.
func frenchHoursTitle(title string) string {
	m := reHoursTitleFR.FindString(title)
	switch {
	case m == "":
		return ""
	case strings.Contains(m, "plein") || strings.Contains(m, "complet"):
		return "full_time"
	}
	return "part_time"
}

// Croatian, Serbian and Bosnian contract types, which a posting states as a phrase
// ("ugovor o radu na neodređeno vrijeme", "Radni odnos: puno radno vreme", "ugovor na
// određeno s mogućnošću produljenja"). The indefinite contract is the permanent one
// (full_time, as a French CDI), the fixed-term contract and the work contract ("ugovor
// o djelu", "autorski ugovor") are contract, and "nepuno radno vrijeme" is part-time.
// The fixed-term contract outranks the permanent one, as everywhere above: "na određeno
// vrijeme, uz mogućnost zasnivanja radnog odnosa na neodređeno" is fixed-term today.
//
// A phrase followed by "ili" offers both and states neither ("na određeno ili na
// neodređeno vrijeme" in a civil-service eligibility clause, "na puno ili nepuno radno
// vrijeme", "na određeno / neodređeno vrijeme"), see bcsLabel. The trainee words count
// in the TITLE only: a body says "praksa" for good practice, "pripravnik" for a junior
// colleague's mentoring, and a civil-service notice cites the "pripravnički staž" in the
// eligibility rules it shares across all the posts it lists. "Skraćeno radno vrijeme" and "pola
// radnog vremena" are absent: both are a parent's benefit in a perks list.
var (
	rePartTimeBCS     = regexp.MustCompile(`\bnepuno radno vr(?:ij)?eme\b`)
	reContractBCS     = regexp.MustCompile(`\bna odre(?:đ|dj|d)eno\b|\bugovor(?:u|a)? (?:o d(?:j)?elu|na odre(?:đ|dj|d)eno)\b|\bautorski ugovor\b`)
	reFullTimeBCS     = regexp.MustCompile(`\bna neodre(?:đ|dj|d)eno\b|\bpuno radno vr(?:ij)?eme\b|\bstalni ugovor\b|\bstalni radni odnos\b`)
	reTraineeTitleBCS = regexp.MustCompile(`\b(?:pripravni(?:k|ca|ci)|praksa|praktikant(?:ica)?|sta(?:ž|z)ist(?:a|kinja)?)\b`)
)

// bcsLabel reports whether re matches s at a place neither preceded nor followed by
// "ili" (or) or a slash, which would offer both arrangements: "na određeno ili na
// neodređeno vrijeme", "na puno ili nepuno radno vrijeme", "Vrsta ugovora: na određeno
// vrijeme / neodređeno vrijeme".
func bcsLabel(s string, re *regexp.Regexp) bool {
	for _, m := range re.FindAllStringIndex(s, -1) {
		before := strings.TrimSuffix(strings.TrimRight(s[:m[0]], " "), " na")
		rest := strings.TrimLeft(s[m[1]:], " ")
		rest = strings.TrimPrefix(strings.TrimPrefix(rest, "vrijeme "), "vreme ")
		alternative := strings.HasSuffix(before, " ili") || strings.HasSuffix(before, "/") ||
			strings.HasPrefix(rest, "ili ") || strings.HasPrefix(rest, "/")
		if !alternative {
			return true
		}
	}
	return false
}

// EducationLevel resolves the required education from the description, returning
// one of vocab.EducationLevelValues or "" when nothing is stated. A named degree
// wins over a "no degree" phrase (a posting that says "Bachelor's or equivalent;
// no degree required for exceptional candidates" still has a degree signal). The
// actual degree matching lives in internal/dict/edulevel.ForRequirement — this
// package keeps only hardRequirementText's required/optional clause split, which
// depends on internal/job/reqextract and therefore cannot move down into dict.
func EducationLevel(description string) string {
	return edulevel.ForRequirement(hardRequirementText(description))
}
