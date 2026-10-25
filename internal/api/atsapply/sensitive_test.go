package atsapply

import "testing"

func TestIsSensitiveLabel_CatchesEveryPortedCategory(t *testing.T) {
	labels := []string{
		"What is your desired salary?",
		"What is your current compensation?",
		"Will you now or in the future require sponsorship?",
		"Do you require a visa to work in this country?",
		"Do you have work authorization for this role?",
		"Do you have the right to work in the UK?",
		"What gender do you identify as?",
		"What is your race?",
		"Please specify your ethnic background.",
		"Are you a protected veteran?",
		"Do you have a disability?",
		"This demographic data is voluntary.",
		"What is your sexual orientation?",
		"What is your religion?",
		"What is your national origin?",
		"What is your date of birth?",
		"Do you consent to genetic information disclosure?",
	}
	for _, label := range labels {
		if !isSensitiveLabel(label) {
			t.Errorf("isSensitiveLabel(%q) = false, want true", label)
		}
	}
}

// Found live (task 5.1's smoke check) against a real Greenhouse posting: "work authoriz"
// as a fixed-order phrase does not match this real, common phrasing at all — "authorization
// to work" has the words in the opposite order.
func TestIsSensitiveLabel_CatchesAuthorizationToWorkPhrasing(t *testing.T) {
	label := "Do you currently have legal authorization to work in the country in which the job you're applying for is located?"
	if !isSensitiveLabel(label) {
		t.Errorf("isSensitiveLabel(%q) = false, want true — a real live posting asked exactly this", label)
	}
}

func TestIsSensitiveLabel_LeavesOrdinaryQuestionsAlone(t *testing.T) {
	labels := []string{
		"Where did you first hear about this role?",
		"Do you have advanced proficiency in German?",
		"Why do you want to work here?",
		"What is your LinkedIn profile?",
		"Please describe a project you're proud of.",
	}
	for _, label := range labels {
		if isSensitiveLabel(label) {
			t.Errorf("isSensitiveLabel(%q) = true, want false", label)
		}
	}
}

func TestIsSensitiveLabel_IsCaseInsensitive(t *testing.T) {
	if !isSensitiveLabel("WHAT IS YOUR DESIRED SALARY?") {
		t.Error("want the sensitive check to ignore case")
	}
}

// Found on production 2026-10-25 (a Brazilian Greenhouse posting previewed end to end): every
// one of these was marked "will be drafted at submission", because the list knew only English
// stems — the model would have stated a salary, a disability status and a demographic answer
// in the candidate's name, and invented a tax id.
func TestIsSensitiveLabel_CatchesTheRealPortugueseForm(t *testing.T) {
	labels := []string{
		"Qual é o seu CPF? (Formato: XXX.XXX.XXX-XX)",
		"Qual é a sua pretensão salarial?",
		"Você é uma pessoa com deficiência?",
		"Consentimento - Diversidade e Inclusão (D&I)",
	}
	for _, label := range labels {
		if !isSensitiveLabel(label) {
			t.Errorf("isSensitiveLabel(%q) = false, want true", label)
		}
	}
}

// A government or tax identifier is a fact about the person nothing in the CV holds; a
// drafted one is a fabricated document number submitted in their name.
func TestIsSensitiveLabel_CatchesIdentityDocumentNumbers(t *testing.T) {
	labels := []string{
		"CPF", "Informe seu RG", "CNPJ (if contractor)", "What is your SSN?", "Social insurance number (SIN)",
		"Social Security Number", "National Insurance number", "Passport number",
		"Número de pasaporte", "DNI / NIE", "Tax ID", "Steuer-ID", "Номер паспорта",
		"ИНН", "СНИЛС", "PESEL", "CURP", "Aadhaar number", "IBAN for payroll",
		"Driver's license number", "Cédula de identidad",
	}
	for _, label := range labels {
		if !isSensitiveLabel(label) {
			t.Errorf("isSensitiveLabel(%q) = false, want true", label)
		}
	}
}

func TestIsSensitiveLabel_CatchesOtherLanguages(t *testing.T) {
	labels := []string{
		"Expectativa salarial", "Pretensión salarial", "Prétentions salariales",
		"Gehaltsvorstellung", "Ожидаемая зарплата", "Remuneração desejada",
		"¿Tiene alguna discapacidad?", "Schwerbehinderung", "Инвалидность",
		"Qual a sua raça/cor?", "Etnia", "Gênero", "Género", "Geschlecht",
		"Você precisa de patrocínio de visto?", "Autorização de trabalho",
		"Data de nascimento", "Fecha de nacimiento", "Geburtsdatum", "Дата рождения",
		"Religião", "Nacionalidade",
	}
	for _, label := range labels {
		if !isSensitiveLabel(label) {
			t.Errorf("isSensitiveLabel(%q) = false, want true", label)
		}
	}
}

// The short identifiers match as whole words only: "rg" sits inside "organization", "pan"
// inside "company", "sin" inside "business".
func TestIsSensitiveLabel_ShortIdentifiersMatchWholeWordsOnly(t *testing.T) {
	labels := []string{
		"Which organization referred you?",
		"Which company are you with now?",
		"Describe your business experience.",
		"Como você conheceu a nossa empresa?",
		"Qual motivo te fez querer se juntar ao nosso time?",
		"Qual é o seu perfil do Linkedin?",
		// Acronyms that are ordinary words elsewhere: the German article, Spanish
		// "without", German "never".
		"Haben Sie ein eigenes Projekt geleitet?",
		"¿Puede trabajar sin supervisión?",
		"Haben Sie nie im Team gearbeitet?",
	}
	for _, label := range labels {
		if isSensitiveLabel(label) {
			t.Errorf("isSensitiveLabel(%q) = true, want false", label)
		}
	}
}
