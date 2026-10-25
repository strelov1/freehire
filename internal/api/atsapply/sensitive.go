package atsapply

import (
	"slices"
	"strings"
	"unicode"
)

// sensitiveTerms is a direct port of freehire-apply/internal/drafting's isSensitive list
// (a sibling, more mature paid repo — already measured against real Ashby postings there,
// not invented fresh here), with one fix a live smoke check (task 5.1) found: the ported
// "work authoriz" is a FIXED-ORDER phrase and never matches "authorization to work" — a
// real Greenhouse posting's exact wording, the more common of the two orderings in
// practice. Replaced with the standalone "authoriz", which catches "authorization"/
// "authorized" in either order and is, if anything, the more defensible term: in an
// application form's context there is no ordinary (non-sensitive) reason a question would
// use that word at all.
//
// A question matching any of these is never drafted, regardless of how confident a draft
// would be — draftable (draft.go) checks isSensitiveLabel before Drafter.Draft is ever
// invoked, so a sensitive question never reaches the model at all.
var sensitiveTerms = []string{
	"salary", "compensation", "sponsor", "visa", "authoriz", "right to work",
	"gender", "race", "ethnic", "veteran", "disab", "demographic", "sexual orientation",
	// The remaining US EEOC-adjacent categories a live posting could ask about as a
	// custom question, found missing by a PR review pass: religion, national origin,
	// date of birth (age-discrimination-adjacent), and genetic information (GINA).
	"religi", "national origin", "date of birth", "genetic",

	// The same categories in the other languages the catalogue's forms are written in. A
	// Brazilian Greenhouse form previewed on production (2026-10-25) had its salary,
	// disability and diversity questions all marked for drafting, because every stem above
	// is English: "salarial" does not contain "salary". Stems again, chosen to cover the
	// Romance forms at once — "salar" is salario/salarial/salariale, "etni" is
	// etnia/étnico/etnicidade, "diversi" is diversity/diversidade/diversité.
	"salar", "salaire", "remunera", "rémunéra", "gehalt", "зарплат", "оклад", "pretensão",
	"patrocín", "autoriza", "autoris", "permiso de trabajo", "arbeitserlaubnis",
	"разрешение на работу",
	"gênero", "género", "geschlecht", "raça", "raza", "etni", "раса", "национальност",
	"deficiên", "discapacidad", "behinderung", "handicap", "инвалид",
	"diversi", "orientação sexual", "orientación sexual",
	"nascimento", "nacimiento", "geburts", "naissance", "рождени",
	"nacionalid", "nationalit", "genétic", "genetisch",
}

// identityTerms are a person's document and account numbers: facts no CV holds, so a
// drafted one is a FABRICATED number submitted in their name — worse than any wrong opinion
// a draft could hold. Found missing on the same production preview: "Qual é o seu CPF?" was
// marked for drafting. Substring stems, like sensitiveTerms.
var identityTerms = []string{
	"social security", "seguridad social", "segurança social", "national insurance",
	"passport", "passaporte", "pasaporte", "паспорт",
	"tax id", "taxpayer", "tax number", "steuer", "identification number", "id number",
	"número de identifica", "documento de identidad", "carteira de identidade", "cédula",
	"personalausweis", "identity card", "id card", "carte d'identité",
	"driver's licen", "drivers licen", "driving licen", "carteira de motorista",
	"bank account", "conta banc", "cuenta banc", "routing number",
	"social insurance", "identidad de extranjero", "employer identification", "pan card",
	"pan number",
}

// identityTokens are the same category's acronyms, matched as WHOLE words: as substrings
// they are everywhere — "rg" in "organization", "cpf" is safe but "pis" is in "pista".
//
// An acronym that is also an ordinary word in a language the forms use stays OUT and is
// caught by its spelled-out name in identityTerms instead: "ein" is the German article,
// "sin" Spanish for "without", "nie" German for "never", "pan" Polish for "Mr". Listed, each
// would stop the drafter on every ordinary question in that language.
var identityTokens = []string{
	"cpf", "cnpj", "rg", "cnh", "pis", "ssn", "nino", "nif", "dni", "nss", "curp", "rfc",
	"pesel", "aadhaar", "itin", "iban", "bsn", "инн", "снилс",
}

// isSensitiveLabel reports whether a question's label text concerns compensation, work
// authorization/visa sponsorship, an equal-opportunity/demographic category, or a document
// or account number — the categories a candidate's answer must never be guessed or drafted
// for, only ever taken from a fact the candidate stated directly (see labelAnswerKeyFor's
// visa_sponsorship_needed case) or left to park.
func isSensitiveLabel(label string) bool {
	return containsAnyTerm(label, sensitiveTerms) || containsAnyTerm(label, identityTerms) ||
		containsAnyWord(label, identityTokens)
}

// containsAnyTerm reports whether the label contains any of the terms, case-insensitively.
// Substring, not whole-word: every term here is a stem ("authoriz" for
// "authorized"/"authorization", "disab" for "disability"/"disabled") chosen so a partial
// spelling still matches.
func containsAnyTerm(label string, terms []string) bool {
	lower := strings.ToLower(label)
	for _, term := range terms {
		if strings.Contains(lower, term) {
			return true
		}
	}
	return false
}

// containsAnyWord reports whether any of the label's words — split on everything that is
// not a letter or a digit — is one of words, case-insensitively.
func containsAnyWord(label string, words []string) bool {
	tokens := strings.FieldsFunc(strings.ToLower(label), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	return slices.ContainsFunc(tokens, func(t string) bool { return slices.Contains(words, t) })
}
