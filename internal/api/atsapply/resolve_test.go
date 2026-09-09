package atsapply

import (
	"testing"

	"github.com/strelov1/freehire/internal/api/candidateprofile"
	"github.com/strelov1/freehire/internal/dict/answertopic"
	"github.com/strelov1/freehire/internal/ingest/applyform"
)

func TestResolve_FillsATextFieldFromAKnownAnswer(t *testing.T) {
	fields := []MergedField{{ID: "first_name", Kind: "text", Required: true}}
	answers := map[string]string{"first_name": "Ada"}

	plan := Resolve(fields, answers, false)

	if len(plan.Fields) != 1 || plan.Fields[0].Value != "Ada" {
		t.Fatalf("plan.Fields = %+v, want first_name=Ada", plan.Fields)
	}
	if len(plan.Unmapped) != 0 {
		t.Errorf("unmapped = %+v, want none", plan.Unmapped)
	}
}

func TestResolve_ARequiredFieldWithNoKnownAnswerIsUnmapped(t *testing.T) {
	fields := []MergedField{{ID: "country", Label: "Country", Kind: "text", Required: true}}
	answers := map[string]string{}

	plan := Resolve(fields, answers, false)

	if len(plan.Fields) != 0 {
		t.Errorf("plan.Fields = %+v, want none filled", plan.Fields)
	}
	if len(plan.Unmapped) != 1 || plan.Unmapped[0].ID != "country" || !plan.Unmapped[0].Required {
		t.Fatalf("unmapped = %+v, want the required country field named", plan.Unmapped)
	}
}

func TestResolve_AnOptionalFieldWithNoKnownAnswerIsSkippedNotUnmapped(t *testing.T) {
	fields := []MergedField{{ID: "portfolio_url", Kind: "text", Required: false}}
	answers := map[string]string{}

	plan := Resolve(fields, answers, false)

	if len(plan.Fields) != 0 || len(plan.Unmapped) != 0 {
		t.Fatalf("plan = %+v, want an unanswered optional field left alone entirely", plan)
	}
}

// candidate-location is the DOM id; the answer key is "location" (the alias applies at
// resolve time too, not just at reconcile's API-matching).
func TestResolve_AppliesTheDOMToAnswerKeyAlias(t *testing.T) {
	fields := []MergedField{{ID: "candidate-location", Kind: "text", Required: true}}
	answers := map[string]string{"location": "Lisbon, Portugal"}

	plan := Resolve(fields, answers, false)

	if len(plan.Fields) != 1 || plan.Fields[0].Value != "Lisbon, Portugal" {
		t.Fatalf("plan.Fields = %+v, want candidate-location filled from the location answer", plan.Fields)
	}
}

// Ashby names its standard identity controls "_systemfield_name" / "_systemfield_email"
// rather than Greenhouse's ids — measured live 2026-09-07 against a Singular posting, which
// parked with "no known answer source" for both despite the candidate's name and email
// already being known.
func TestResolve_AppliesTheAshbySystemFieldAliases(t *testing.T) {
	fields := []MergedField{
		{ID: "_systemfield_name", Kind: "text", Required: true},
		{ID: "_systemfield_email", Kind: "text", Required: true},
	}
	answers := map[string]string{"full_name": "Ada Lovelace", "email": "ada@example.test"}

	plan := Resolve(fields, answers, false)

	if len(plan.Unmapped) != 0 {
		t.Fatalf("unmapped = %+v, want both Ashby system fields resolved", plan.Unmapped)
	}
	if len(plan.Fields) != 2 || plan.Fields[0].Value != "Ada Lovelace" || plan.Fields[1].Value != "ada@example.test" {
		t.Fatalf("plan.Fields = %+v, want _systemfield_name=Ada Lovelace and _systemfield_email=ada@example.test", plan.Fields)
	}
}

// Ashby's LinkedIn question carries a random uuid id rather than "linkedin" — the id
// answerKeyFor already covers — so only the label rule can match it.
func TestResolve_MatchesLinkedInByLabelWhenTheIDIsOpaque(t *testing.T) {
	fields := []MergedField{{ID: "6810b294-be01-4bf4-bd4a-d056ddd0d6da", Label: "LinkedIn ", Kind: "text", Required: true}}
	answers := map[string]string{"linkedin": "https://linkedin.com/in/ada"}

	plan := Resolve(fields, answers, false)

	if len(plan.Unmapped) != 0 {
		t.Fatalf("unmapped = %+v, want the LinkedIn field resolved via its label", plan.Unmapped)
	}
	if len(plan.Fields) != 1 || plan.Fields[0].Value != "https://linkedin.com/in/ada" {
		t.Fatalf("plan.Fields = %+v, want the LinkedIn field filled from the linkedin answer", plan.Fields)
	}
}

// A select/checkbox field's answer must match one of the platform's own offered options —
// never a value the widget does not offer, per the "never guess" rule the whole design rests
// on.
func TestResolve_MatchesAnAnswerToTheClosestOfferedOptionValue(t *testing.T) {
	fields := []MergedField{{
		ID: "authorized_countries", Kind: "select", Required: true,
		Options: []applyform.Option{{Label: "Yes", Value: "1"}, {Label: "No", Value: "0"}},
	}}
	answers := map[string]string{"authorized_countries": "Yes"}

	plan := Resolve(fields, answers, false)

	if len(plan.Fields) != 1 || plan.Fields[0].Value != "1" {
		t.Fatalf("plan.Fields = %+v, want the option's platform VALUE (1), not the label", plan.Fields)
	}
}

func TestResolve_AnAnswerMatchingNoOfferedOptionParksRatherThanGuessing(t *testing.T) {
	fields := []MergedField{{
		ID: "authorized_countries", Label: "Are you authorized?", Kind: "select", Required: true,
		Options: []applyform.Option{{Label: "Yes", Value: "1"}, {Label: "No", Value: "0"}},
	}}
	answers := map[string]string{"authorized_countries": "Sponsorship required"}

	plan := Resolve(fields, answers, false)

	if len(plan.Fields) != 0 {
		t.Errorf("plan.Fields = %+v, want nothing filled — the answer matches no offered option", plan.Fields)
	}
	if len(plan.Unmapped) != 1 || plan.Unmapped[0].ID != "authorized_countries" {
		t.Fatalf("unmapped = %+v, want the field named with its mismatch reason", plan.Unmapped)
	}
}

// A required résumé field parks when the entry carries no approved tailored CV — the only
// artifact this package can attach. answers is irrelevant: a file field is never resolved
// from the answers map, only from hasApprovedCV.
func TestResolve_ARequiredResumeFieldParksWithNoApprovedCV(t *testing.T) {
	fields := []MergedField{{ID: "resume", Label: "Resume/CV", Kind: "file", Required: true}}
	answers := map[string]string{"resume": "https://example.test/resume.pdf"}

	plan := Resolve(fields, answers, false)

	if len(plan.Fields) != 0 {
		t.Errorf("plan.Fields = %+v, want the résumé field left unfilled with no approved CV", plan.Fields)
	}
	if len(plan.Unmapped) != 1 || plan.Unmapped[0].ID != "resume" {
		t.Fatalf("unmapped = %+v, want the resume field named", plan.Unmapped)
	}
}

// The résumé field resolves (Kind stays "file"; Value is set later, by Client.Submit, once
// the CV is actually rendered) once the entry carries an approved tailored CV.
func TestResolve_ARequiredResumeFieldResolvesWithAnApprovedCV(t *testing.T) {
	fields := []MergedField{{ID: "resume", Label: "Resume/CV", Kind: "file", Required: true}}

	plan := Resolve(fields, map[string]string{}, true)

	if len(plan.Unmapped) != 0 {
		t.Fatalf("unmapped = %+v, want the résumé field resolved with an approved CV", plan.Unmapped)
	}
	if len(plan.Fields) != 1 || plan.Fields[0].ID != "resume" || plan.Fields[0].Kind != "file" {
		t.Fatalf("plan.Fields = %+v, want the resume field resolved, Kind unchanged", plan.Fields)
	}
}

// A cover-letter (or any other non-résumé) file field stays unmapped even with an approved
// CV — this path only ever closes the résumé gap, per design.md's Non-Goals.
func TestResolve_ACoverLetterFieldStaysUnmappedEvenWithAnApprovedCV(t *testing.T) {
	fields := []MergedField{{ID: "cover_letter", Label: "Cover Letter", Kind: "file", Required: true}}

	plan := Resolve(fields, map[string]string{}, true)

	if len(plan.Fields) != 0 {
		t.Errorf("plan.Fields = %+v, want the cover letter field left unfilled", plan.Fields)
	}
	if len(plan.Unmapped) != 1 || plan.Unmapped[0].ID != "cover_letter" {
		t.Fatalf("unmapped = %+v, want the cover_letter field named", plan.Unmapped)
	}
}

func TestResolve_FullyResolvedReportsNoUnmapped(t *testing.T) {
	fields := []MergedField{
		{ID: "first_name", Kind: "text", Required: true},
		{ID: "portfolio_url", Kind: "text", Required: false},
	}
	answers := map[string]string{"first_name": "Ada"}

	plan := Resolve(fields, answers, false)

	if !plan.FullyResolved() {
		t.Errorf("FullyResolved() = false, want true — the only required field is answered")
	}
}

func TestResolve_NotFullyResolvedWhenAnyUnmappedExists(t *testing.T) {
	fields := []MergedField{{ID: "country", Kind: "text", Required: true}}
	plan := Resolve(fields, map[string]string{}, false)

	if plan.FullyResolved() {
		t.Error("FullyResolved() = true, want false — a required field has no answer")
	}
}

func TestIsCoverLetterTextField_RecognizesTheKnownID(t *testing.T) {
	if !isCoverLetterTextField(MergedField{ID: "cover_letter_text", Label: "", Kind: "textarea"}) {
		t.Error("want a cover_letter_text field recognized by id alone")
	}
}

func TestIsCoverLetterTextField_RecognizesAnOpaqueIDByLabel(t *testing.T) {
	if !isCoverLetterTextField(MergedField{ID: "question_98765", Label: "Cover Letter", Kind: "textarea"}) {
		t.Error("want an opaque-id field with a Cover Letter label recognized via the label fallback")
	}
}

func TestIsCoverLetterTextField_AnUnrelatedFreeTextFieldIsNotRecognized(t *testing.T) {
	if isCoverLetterTextField(MergedField{ID: "question_11111", Label: "Why do you want to work here?", Kind: "textarea"}) {
		t.Error("want an unrelated free-text question not recognized as a cover-letter field")
	}
}

func TestIsCoverLetterTextField_RecognizesALabelWithATrailingQualifier(t *testing.T) {
	if !isCoverLetterTextField(MergedField{ID: "question_22222", Label: "Cover Letter (optional)", Kind: "textarea"}) {
		t.Error("want a label that opens with the phrase recognized regardless of what follows")
	}
}

// Found by code review: a substring match on the label would fire on any question that
// merely MENTIONS a cover letter, not just one asking for one — and since this field's
// answer is used verbatim, that would submit the candidate's full cover letter as the
// literal answer to an unrelated question.
func TestIsCoverLetterTextField_ALabelThatOnlyMentionsACoverLetterIsNotRecognized(t *testing.T) {
	if isCoverLetterTextField(MergedField{ID: "question_33333", Label: "If you don't have a cover letter, please explain why", Kind: "text"}) {
		t.Error("want a label that only mentions a cover letter in passing not recognized as a cover-letter field")
	}
}

// The candidate's own stored salary expectation answers an employer's salary question,
// whatever they called it.
//
// Measured on production 2026-09-08: queue entry 3 parked on "What is your desired salary?"
// while screening_answers held 5000 USD/year for that very candidate. Greenhouse gives a
// custom question an opaque numeric id, so answerKeyFor can never reach it and only a label
// rule can — and there was none for salary, though there was one for visa sponsorship right
// beside it.
//
// The three phrasings are the shapes real boards use for the same question. A rule that
// only matched the exact wording of the one posting we happened to look at would be the
// same gap again, one phrasing narrower.
func TestResolve_MatchesASalaryQuestionByLabelWhateverItIsCalled(t *testing.T) {
	answers := map[string]string{"desired_salary": "5000 USD per year"}

	for _, label := range []string{
		"What is your desired salary?",
		"Salary expectations",
		"Desired compensation (USD)",
	} {
		t.Run(label, func(t *testing.T) {
			fields := []MergedField{{ID: "question_19869712004", Label: label, Kind: "text", Required: true}}

			plan := Resolve(fields, answers, false)

			if len(plan.Unmapped) != 0 {
				t.Fatalf("unmapped = %+v, want the salary question answered from the candidate's own figure", plan.Unmapped)
			}
			if len(plan.Fields) != 1 || plan.Fields[0].Value != "5000 USD per year" {
				t.Fatalf("plan.Fields = %+v, want the stored salary", plan.Fields)
			}
		})
	}
}

// The rule must not reach a question ABOUT pay that is not asking for the candidate's own
// figure. Current pay is a different fact (candidate_survey holds it separately and
// deliberately), and answering it with a desired figure would misreport them to an
// employer.
func TestResolve_DoesNotAnswerACurrentSalaryQuestionWithTheDesiredOne(t *testing.T) {
	answers := map[string]string{"desired_salary": "5000 USD per year"}
	fields := []MergedField{{ID: "question_1", Label: "What is your current salary?", Kind: "text", Required: true}}

	plan := Resolve(fields, answers, false)

	if len(plan.Fields) != 0 {
		t.Fatalf("plan.Fields = %+v, want the current-salary question left unanswered", plan.Fields)
	}
	if len(plan.Unmapped) != 1 {
		t.Fatalf("unmapped = %+v, want the current-salary question reported as unanswered", plan.Unmapped)
	}
}

// A banked answer reaches a question no rule and no id could match — the bank's whole
// purpose. The key is the question's own topic, so the wording the employer used does not
// have to be the wording the candidate answered.
func TestResolve_AnsweredFromTheBankByTopic(t *testing.T) {
	answers := map[string]string{"topic:which state do you currently reside in": "Santa Catarina"}
	fields := []MergedField{{
		ID: "question_4005041004", Label: "Which state do you currently reside in?",
		Kind: "text", Required: true,
	}}

	plan := Resolve(fields, answers, false)

	if len(plan.Unmapped) != 0 {
		t.Fatalf("unmapped = %+v, want the question answered from the bank", plan.Unmapped)
	}
	if len(plan.Fields) != 1 || plan.Fields[0].Value != "Santa Catarina" {
		t.Fatalf("plan.Fields = %+v, want the banked answer", plan.Fields)
	}
}

// A typed fact still wins. It is validated and structured; the bank's copy is free text,
// and two sources answering one question must resolve the same way every time rather than
// by whichever was read first.
func TestResolve_ATypedFactOutranksABankedAnswer(t *testing.T) {
	answers := map[string]string{
		"desired_salary":           "5000 USD per year",
		"topic:salary_expectation": "whatever you think is fair",
	}
	fields := []MergedField{{ID: "question_1", Label: "What is your desired salary?", Kind: "text", Required: true}}

	plan := Resolve(fields, answers, false)

	if len(plan.Fields) != 1 || plan.Fields[0].Value != "5000 USD per year" {
		t.Fatalf("plan.Fields = %+v, want the typed fact to win", plan.Fields)
	}
}

// The seam: what Profile.Fields() writes must be what resolveOne reads.
//
// The prefix is two separate literals in two packages that cannot share a constant
// (candidateprofile importing atsapply would invert the layering). Every other test here
// hands Resolve a map it built itself with the prefix already applied — so all of them
// would still pass if the two literals drifted apart, while the feature silently filled
// nothing. This is the only test that would fail.
func TestResolve_ReadsTheKeysProfileFieldsActuallyWrites(t *testing.T) {
	profile := candidateprofile.Profile{
		BankAnswers: map[string]string{"which state do you currently reside in": "Santa Catarina"},
	}

	fields := []MergedField{{
		ID: "question_4005041004", Label: "Which state do you currently reside in?",
		Kind: "text", Required: true,
	}}

	plan := Resolve(fields, profile.Fields(), false)

	if !plan.FullyResolved() {
		t.Fatalf("unmapped = %+v — Profile.Fields() and resolveOne disagree about the banked-answer key prefix", plan.Unmapped)
	}
	if plan.Fields[0].Value != "Santa Catarina" {
		t.Errorf("value = %q, want the banked answer", plan.Fields[0].Value)
	}
}

// The feature, asserted as one story: a required question parks, the candidate answers it,
// and the next resolve fills it — even though the second employer words it differently.
//
// This is the test that would have caught the whole class of bug this feature exists for.
// Everything else here checks a piece.
func TestResolve_AnAnsweredQuestionStopsBlockingLaterApplications(t *testing.T) {
	firstEmployer := []MergedField{{
		ID: "question_4005041004", Label: "Which state do you currently reside in?",
		Kind: "text", Required: true,
	}}
	noAnswersYet := map[string]string{}

	before := Resolve(firstEmployer, noAnswersYet, false)
	if len(before.Unmapped) != 1 {
		t.Fatalf("unmapped = %+v, want the question to park before it is answered", before.Unmapped)
	}
	if before.FullyResolved() {
		t.Fatal("FullyResolved() is true with a required question unanswered")
	}

	// The candidate answers it. answertopic.Of is what the server applies on save; the key
	// here is what that produces.
	banked := map[string]string{"topic:which state do you currently reside in": "Santa Catarina"}

	secondEmployer := []MergedField{{
		ID: "question_99887766", Label: "  Which state do you currently reside in?  ",
		Kind: "text", Required: true,
	}}

	after := Resolve(secondEmployer, banked, false)
	if !after.FullyResolved() {
		t.Fatalf("unmapped = %+v, want a different employer's phrasing answered from the bank", after.Unmapped)
	}
	if after.Fields[0].Value != "Santa Catarina" {
		t.Errorf("value = %q, want the banked answer", after.Fields[0].Value)
	}
}

// The bank must never answer a work-authorization question. "Are you authorized to work in
// the country in which this position is located?" is ONE topic across every posting worded
// that way, so a "Yes" banked from a US posting would be re-asserted, in the candidate's
// name, on a Brazilian one — and at submit time, where the candidate never sees it.
// labelAnswerKeyFor's own doc comment states this invariant; the bank is bound by it too.
func TestResolve_NeverAnswersAWorkAuthorizationQuestionFromTheBank(t *testing.T) {
	for _, label := range []string{
		"Are you legally authorized to work in the country in which this position is located?",
		"Do you now or in the future require sponsorship to work in the United States?",
		"Do you have the right to work in the UK?",
		"Will you require a visa?",
	} {
		topic, ok := bankTopicKeyForTest(t, label)
		answers := map[string]string{topic: "Yes"}
		if !ok {
			t.Fatalf("the fold refused %q — this test would pass for the wrong reason", label)
		}
		fields := []MergedField{{ID: "question_1", Label: label, Kind: "text", Required: true}}

		plan := Resolve(fields, answers, false)

		if len(plan.Fields) != 0 {
			t.Errorf("Resolve(%q) filled %+v — a work-authorization question needs this posting's own country, which nothing here has", label, plan.Fields)
		}
		if len(plan.Unmapped) != 1 {
			t.Errorf("Resolve(%q) unmapped = %+v, want the question reported as unanswered", label, plan.Unmapped)
		}
	}
}

// The refusal is the work-authorization SUBSET of sensitiveTerms, not the whole list.
// Salary is on that list and the salary case is the feature's headline; demographic
// questions are on it too and are the candidate's own answer to give. Only authorization is
// unanswerable here, because only it depends on a country nothing in this package holds.
func TestMatchBankAnswerKey_RefusesOnlyTheWorkAuthorizationSubset(t *testing.T) {
	refused := []string{
		"Are you legally authorized to work in the country in which this position is located?",
		"Do you now or in the future require sponsorship to work in the United States?",
		"Do you have the right to work in the UK?",
		"Will you require a visa?",
	}
	for _, label := range refused {
		if key, ok := matchBankAnswerKey(MergedField{Label: label}); ok {
			t.Errorf("matchBankAnswerKey(%q) = %q, true; want a refusal", label, key)
		}
	}

	kept := []string{
		"What is your desired salary?",
		"Compensation expectations",
		"Are you a protected veteran?",
		"Which state do you currently reside in?",
	}
	for _, label := range kept {
		if _, ok := matchBankAnswerKey(MergedField{Label: label}); !ok {
			t.Errorf("matchBankAnswerKey(%q) refused — only work authorization is refused here", label)
		}
	}
}

// bankTopicKeyForTest is the answers-map key a question is banked under, built the way the
// server does on save.
func bankTopicKeyForTest(t *testing.T, question string) (string, bool) {
	t.Helper()
	topic, ok := answertopic.Of(question)
	return bankAnswerKeyPrefix + topic, ok
}

// The empty-label seam, asserted across the two functions that have to agree about it.
//
// MergedField.Label is empty for any DOM-rendered field the platform's schema never
// declared (reconcile.go), and Greenhouse's `country` is exactly that field — required on
// nearly every posting and, by resolve.go's own admission, the single most common reason an
// application parks. The review screen shows the candidate an input titled "country"
// (PreviewAnswers substitutes the id), they answer it, and the server banks it under the
// topic of the text they read. If the resolver keys on the raw label instead, that answer is
// stored under a key nothing can recall and the field parks again forever.
func TestPreviewAndResolveAgreeOnALabellessField(t *testing.T) {
	field := MergedField{ID: "country", Kind: "text", Required: true}

	preview := PreviewAnswers([]MergedField{field}, map[string]string{}, false)
	if len(preview.Pending) != 1 {
		t.Fatalf("preview.Pending = %+v, want the labelless field reported as pending", preview.Pending)
	}
	// What the candidate is shown, and therefore what the save route receives as the
	// question text.
	shown := preview.Pending[0].Label
	if shown == "" {
		t.Fatal("the review screen would render an input with no title at all")
	}

	topic, ok := bankTopicKeyForTest(t, shown)
	if !ok {
		t.Fatalf("the server would refuse to bank %q — nothing the candidate types can be saved", shown)
	}

	plan := Resolve([]MergedField{field}, map[string]string{topic: "Brazil"}, false)

	if !plan.FullyResolved() {
		t.Fatalf("unmapped = %+v — the answer was banked under %q and the resolver looks somewhere else", plan.Unmapped, topic)
	}
	if plan.Fields[0].Value != "Brazil" {
		t.Errorf("value = %q, want the banked answer", plan.Fields[0].Value)
	}
}
