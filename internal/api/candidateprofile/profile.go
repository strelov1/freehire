// Package candidateprofile assembles the canonical set of fields a candidate has already
// stated — identity, contact details, and their own screening answers — from their CV,
// their structured résumé, their account, and internal/screeninganswers.
//
// It exists so a value the browser extension fills a form with and a value cmd/auto-apply
// resolves a form against can never diverge: both read this package's Assemble, not two
// separate re-derivations of the same precedence rules. It answers only what the candidate
// has already told the system — nothing here drafts or infers an answer, so an unset field
// stays unset rather than guessed.
package candidateprofile

import (
	"context"
	"errors"
	"strings"

	"github.com/strelov1/freehire/internal/candidate/cv"
	"github.com/strelov1/freehire/internal/candidate/resumeextract"
	"github.com/strelov1/freehire/internal/ingest/screeninganswers"
	"github.com/strelov1/freehire/internal/platform/db"
)

// Profile is the canonical set of fields assembled for a candidate.
type Profile struct {
	FullName  string `json:"full_name"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Email     string `json:"email"`
	Phone     string `json:"phone"`
	Location  string `json:"location"`
	LinkedIn  string `json:"linkedin"`
	GitHub    string `json:"github"`
	Portfolio string `json:"portfolio"`

	// The candidate's own screening answers (internal/screeninganswers), formatted for
	// display/autofill. Independent of the identity fields above: there is exactly one
	// screening-answers store, so unlike CV vs. résumé there is no precedence to resolve.
	// Empty when the caller has stated nothing — never guessed.
	AuthorizedCountries   string `json:"authorized_countries"`
	VisaSponsorshipNeeded string `json:"visa_sponsorship_needed"`
	DesiredSalary         string `json:"desired_salary"`
	NoticePeriod          string `json:"notice_period"`
	WillingToRelocate     string `json:"willing_to_relocate"`
	Age18OrOlder          string `json:"age_18_or_older"`

	// BankAnswers is topic → answer for the questions this candidate has answered on
	// earlier applications. Not a fixed field like the ones above, because the questions
	// employers author are not a fixed set — that is the whole reason the bank exists.
	BankAnswers map[string]string `json:"-"`
}

// Fields flattens Profile into the keyed values a form-filler grounds its plan in — the
// fixed set above and nothing else, keyed exactly as /me/autofill-profile returns them.
//
// The banked answers are deliberately NOT here; FieldsWithBankedAnswers is what includes
// them. This map is what internal/api/handler's agent autofill hands to
// internal/ai/autofillagent, which JSON-marshals the whole of it into a model prompt and
// then uses it as the grounding set — so adding the bank here would send every free-text
// answer the candidate ever typed for an employer (on Greenhouse that includes the
// demographic and veteran/disability questions) to the model on every run, and would widen
// autofillagent.groundedValue's anti-fabrication gate by the size of the bank: that gate
// accepts any planned value sharing a word-run with ANY profile value, so a model could
// fabricate an answer to one question and have it admitted because the words happen to
// appear in an unrelated banked answer.
func (p Profile) Fields() map[string]string {
	return map[string]string{
		"full_name":  p.FullName,
		"first_name": p.FirstName,
		"last_name":  p.LastName,
		"email":      p.Email,
		"phone":      p.Phone,
		"location":   p.Location,
		"linkedin":   p.LinkedIn,
		"github":     p.GitHub,
		"portfolio":  p.Portfolio,

		"authorized_countries":    p.AuthorizedCountries,
		"visa_sponsorship_needed": p.VisaSponsorshipNeeded,
		"desired_salary":          p.DesiredSalary,
		"notice_period":           p.NoticePeriod,
		"willing_to_relocate":     p.WillingToRelocate,
		"age_18_or_older":         p.Age18OrOlder,
	}
}

// FieldsWithBankedAnswers is Fields plus the candidate's banked answers, and is what an
// application FORM is resolved against — internal/api/atsapply.Resolve, reached through
// cmd/auto-apply's own autoapply.AnswerSource. Those two are the only consumers the design
// names, and the split is what keeps the bank off every other path (see Fields).
//
// Banked answers ride in the same map under a prefix, so a caller that resolves an answer
// never has to know which source stated it. The prefix must match internal/api/atsapply's
// own bankAnswerKeyPrefix — a topic is a folded question and could otherwise collide with a
// fixed key. The two cannot share a constant (atsapply is layer 8, this block sits below
// it), so that comment and this one are the only thing holding them together;
// atsapply's TestResolve_ReadsTheKeysProfileFieldsActuallyWrites is the only test that
// fails if they drift.
func (p Profile) FieldsWithBankedAnswers() map[string]string {
	fields := p.Fields()
	for topic, answer := range p.BankAnswers {
		fields["topic:"+topic] = answer
	}
	return fields
}

// CVReader is the one base-CV read Assemble makes. Tailored copies are excluded by the
// query behind it (GetBaseCVByUser filters NOT is_tailored), not by anything here: a
// tailored CV is written for one vacancy and this block carries none.
type CVReader interface {
	BaseCV(ctx context.Context, userID int64) (cv.Record, bool, error)
}

// ResumeReader is the one résumé read Assemble makes. The bool reports whether a structure
// current with the uploaded résumé exists: the store compares the structure's stamp against
// the résumé's upload time, so one derived from a superseded file reads as absent rather
// than being served.
type ResumeReader interface {
	Structured(ctx context.Context, userID int64) (resumeextract.Structured, bool, error)
}

// AccountReader supplies the address that backstops a contact block stating no email.
type AccountReader interface {
	GetUserByID(ctx context.Context, id int64) (db.GetUserByIDRow, error)
}

// ScreeningAnswersReader supplies the candidate's own screening answers.
type ScreeningAnswersReader interface {
	Get(ctx context.Context, userID int64) (screeninganswers.Answers, error)
}

// BankReader supplies the candidate's own banked screening answers, keyed by topic. Only
// answers they themselves gave — internal/candidate/answerbank.Store.Sendable is what
// enforces that, not this interface.
type BankReader interface {
	Sendable(ctx context.Context, userID int64) (map[string]string, error)
}

// Assembler holds the sources Assemble reads, in precedence order.
type Assembler struct {
	cvs      CVReader
	resumes  ResumeReader
	accounts AccountReader
	// screeningAnswers is nil-able: "no screening answers configured" degrades the
	// screening fields to empty, same as an unconfigured résumé reader.
	screeningAnswers ScreeningAnswersReader
	// bank is nil-able: "no answer bank configured" degrades to no banked answers, the
	// same way an unconfigured screening-answers reader degrades those fields to empty.
	bank BankReader
}

// NewAssembler builds an Assembler over its sources. screeningAnswers and bank may be nil.
func NewAssembler(cvs CVReader, resumes ResumeReader, accounts AccountReader, screeningAnswers ScreeningAnswersReader, bank BankReader) *Assembler {
	return &Assembler{cvs: cvs, resumes: resumes, accounts: accounts, screeningAnswers: screeningAnswers, bank: bank}
}

// Assemble builds a candidate's Profile from the first source that states a contact,
// backstopped by their account email.
//
// The sources are ordered: the base CV first, because it is the copy the candidate authored
// and the only one the tailoring agent cannot rewrite (the edit policy denies it the
// header's name, email, phone and links); the structured résumé second, because it is what
// seeded that CV in the first place. Corrected beats derived.
func (a *Assembler) Assemble(ctx context.Context, userID int64) (Profile, error) {
	var fromCV cv.Header
	if rec, ok, err := a.cvs.BaseCV(ctx, userID); err != nil {
		return Profile{}, err
	} else if ok {
		fromCV = rec.Document.Header
	}

	var fromResume cv.Header
	if st, ok, err := a.resumes.Structured(ctx, userID); err != nil {
		return Profile{}, err
	} else if ok {
		fromResume = ContactHeaderFromStructured(st)
	}

	account, err := a.accounts.GetUserByID(ctx, userID)
	if err != nil {
		return Profile{}, err
	}

	profile := BuildProfile(firstStatedHeader(fromCV, fromResume), account.Email)
	if err := applyScreeningFields(ctx, &profile, a.screeningAnswers, userID); err != nil {
		return Profile{}, err
	}
	if a.bank != nil {
		banked, err := a.bank.Sendable(ctx, userID)
		if err != nil {
			return Profile{}, err
		}
		profile.BankAnswers = banked
	}
	return profile, nil
}

// statesContact reports whether a header carries any contact value at all. A source that
// exists but states nothing is passed over rather than allowed to answer, so a CV created
// empty does not silence a résumé that has the values.
func statesContact(h cv.Header) bool {
	return h.FullName != "" || h.Email != "" || h.Phone != "" || h.Location != "" || len(h.Links) > 0
}

// firstStatedHeader returns the first header that states any contact value, or the zero
// header when none does.
//
// The chosen header answers for the WHOLE block: fields are never merged across sources.
// The résumé structure is what seeds a CV (cv.Seed), so the two sources hold the same
// fields with the CV holding the corrected ones — a merge would restore precisely the
// value the candidate deleted from their CV.
func firstStatedHeader(headers ...cv.Header) cv.Header {
	for _, h := range headers {
		if statesContact(h) {
			return h
		}
	}
	return cv.Header{}
}

// ContactHeaderFromStructured projects the structured résumé's contact fields into a
// header, so the résumé can be offered to firstStatedHeader as one more source. It is the
// same five-field mapping cv.Seed makes when it seeds a CV from the same structure.
func ContactHeaderFromStructured(st resumeextract.Structured) cv.Header {
	return cv.Header{
		FullName: st.FullName,
		Email:    st.Email,
		Phone:    st.Phone,
		Location: st.Location,
		Links:    st.Links,
	}
}

// BuildProfile projects a CV contact header (plus the account email as a fallback) into
// the canonical Profile fields, splitting the name and sorting links into
// linkedin / github / portfolio.
func BuildProfile(h cv.Header, accountEmail string) Profile {
	first, last := splitName(h.FullName)
	email := h.Email
	if email == "" {
		email = accountEmail
	}
	p := Profile{
		FullName:  h.FullName,
		FirstName: first,
		LastName:  last,
		Email:     email,
		Phone:     h.Phone,
		Location:  h.Location,
	}
	for _, link := range h.Links {
		switch l := strings.ToLower(link); {
		case strings.Contains(l, "linkedin.com") && p.LinkedIn == "":
			p.LinkedIn = link
		case strings.Contains(l, "github.com") && p.GitHub == "":
			p.GitHub = link
		case p.Portfolio == "":
			p.Portfolio = link
		}
	}
	return p
}

func splitName(full string) (first, last string) {
	parts := strings.Fields(full)
	switch len(parts) {
	case 0:
		return "", ""
	case 1:
		return parts[0], ""
	default:
		return parts[0], strings.Join(parts[1:], " ")
	}
}

// applyScreeningFields merges the caller's screening answers into the profile. Best-effort
// in the same shape as the CV/résumé reads' "not found" branches: no reader configured or
// no answers stated both leave the screening fields empty rather than failing the whole
// read. Any OTHER error is real and propagates.
func applyScreeningFields(ctx context.Context, p *Profile, reader ScreeningAnswersReader, userID int64) error {
	if reader == nil {
		return nil
	}
	answers, err := reader.Get(ctx, userID)
	if err != nil {
		if errors.Is(err, screeninganswers.ErrNotFound) {
			return nil
		}
		return err
	}
	fields := answers.AutofillFields()
	p.AuthorizedCountries = fields["authorized_countries"]
	p.VisaSponsorshipNeeded = fields["visa_sponsorship_needed"]
	p.DesiredSalary = fields["desired_salary"]
	p.NoticePeriod = fields["notice_period"]
	p.WillingToRelocate = fields["willing_to_relocate"]
	p.Age18OrOlder = fields["age_18_or_older"]
	return nil
}
