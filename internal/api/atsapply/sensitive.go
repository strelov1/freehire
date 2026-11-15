package atsapply

import (
	"slices"
	"strings"
	"unicode"
)

// sensitiveTerms is ported from freehire-apply/internal/drafting's isSensitive list, with
// "work authoriz" replaced by the standalone "authoriz", which also matches "authorization
// to work" (the common ordering) and has no ordinary non-sensitive use in a form.
//
// A question matching any term is never drafted: draftable (draft.go) checks
// isSensitiveLabel before Drafter.Draft, so it never reaches the model.
var sensitiveTerms = []string{
	"salary", "compensation", "sponsor", "visa", "authoriz", "right to work",
	"gender", "race", "ethnic", "veteran", "disab", "demographic", "sexual orientation",
	// The remaining US EEOC-adjacent categories: religion, national origin, date of birth,
	// genetic information (GINA).
	"religi", "national origin", "date of birth", "genetic",

	// The same categories in other languages the catalogue's forms use; the English stems
	// above miss them ("salarial" does not contain "salary"). Stems chosen to cover the
	// Romance forms at once: "salar" is salario/salarial/salariale, "etni" is
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
// drafted one is a fabricated number submitted in their name. Substring stems, like
// sensitiveTerms.
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

// identityTokens are the same category's acronyms, matched as whole words because as
// substrings they are everywhere ("rg" in "organization", "pis" in "pista").
//
// An acronym that is also an ordinary word in a form language stays out and is caught by
// its spelled-out name in identityTerms: "ein" (German article), "sin" (Spanish
// "without"), "nie" (German "never"), "pan" (Polish "Mr").
var identityTokens = []string{
	"cpf", "cnpj", "rg", "cnh", "pis", "ssn", "nino", "nif", "dni", "nss", "curp", "rfc",
	"pesel", "aadhaar", "itin", "iban", "bsn", "инн", "снилс",
}

// isSensitiveLabel reports whether a label concerns compensation, work authorization/visa
// sponsorship, a demographic category, or a document/account number: answers that are
// never guessed or drafted, only taken from a fact the candidate stated (see
// labelAnswerKeyFor's visa_sponsorship_needed case) or left to park.
func isSensitiveLabel(label string) bool {
	return containsAnyTerm(label, sensitiveTerms) || containsAnyTerm(label, identityTerms) ||
		containsAnyWord(label, identityTokens)
}

// containsAnyTerm reports whether the label contains any term, case-insensitively.
// Substring, not whole-word: terms are stems ("authoriz", "disab") so partial spellings
// match.
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
