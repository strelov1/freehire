// Package candidateprofile assembles the fields a candidate has already stated (identity,
// contact, screening answers) from their CV, structured résumé, account and screening answers.
//
// The extension autofill and cmd/auto-apply both read Assemble, so their values cannot diverge.
// Nothing here drafts or infers: an unset field stays unset.
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

	// Screening answers (internal/screeninganswers), formatted for autofill. Independent of the
	// identity fields: one store, no precedence. Empty when unstated, never guessed.
	AuthorizedCountries   string `json:"authorized_countries"`
	VisaSponsorshipNeeded string `json:"visa_sponsorship_needed"`
	DesiredSalary         string `json:"desired_salary"`
	NoticePeriod          string `json:"notice_period"`
	WillingToRelocate     string `json:"willing_to_relocate"`
	Age18OrOlder          string `json:"age_18_or_older"`
	PrivacyConsent        string `json:"privacy_consent"`

	// BankAnswers is topic → answer for questions answered on earlier applications. Not a fixed
	// field because employer questions are not a fixed set.
	BankAnswers map[string]string `json:"-"`
}

// Fields flattens Profile into the keyed values a form-filler grounds its plan in, keyed as
// /me/autofill-profile returns them.
//
// Banked answers are deliberately NOT here (see FieldsWithBankedAnswers): this map is
// JSON-marshalled into the autofillagent prompt, so the bank would send every past free-text
// answer (demographic ones included) to the model and widen autofillagent.groundedValue's
// anti-fabrication gate, which admits any value sharing a word-run with any profile value.
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
		"privacy_consent":         p.PrivacyConsent,
	}
}

// FieldsWithBankedAnswers is Fields plus the banked answers. An application FORM is resolved
// against it (atsapply.Resolve), which keeps the bank off every other path.
//
// Banked answers ride under a "topic:" prefix that must match atsapply's bankAnswerKeyPrefix
// (no shared constant: atsapply is a higher layer). atsapply's
// TestResolve_ReadsTheKeysProfileFieldsActuallyWrites fails on drift.
func (p Profile) FieldsWithBankedAnswers() map[string]string {
	fields := p.Fields()
	for topic, answer := range p.BankAnswers {
		fields["topic:"+topic] = answer
	}
	return fields
}

// CVReader is the one base-CV read Assemble makes. The query (GetBaseCVByUser filters NOT
// is_tailored) excludes tailored copies, which are written for one vacancy.
type CVReader interface {
	BaseCV(ctx context.Context, userID int64) (cv.Record, bool, error)
}

// ResumeReader is the one résumé read Assemble makes. The bool reports whether a structure
// current with the uploaded résumé exists; one from a superseded file reads as absent.
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

// BankReader supplies the candidate's banked answers by topic. Only answers they gave:
// answerbank.Store.Sendable enforces that.
type BankReader interface {
	Sendable(ctx context.Context, userID int64) (map[string]string, error)
}

// Assembler holds the sources Assemble reads, in precedence order.
type Assembler struct {
	cvs      CVReader
	resumes  ResumeReader
	accounts AccountReader
	// screeningAnswers is nil-able: unconfigured leaves the screening fields empty.
	screeningAnswers ScreeningAnswersReader
	// bank is nil-able: unconfigured means no banked answers.
	bank BankReader
	// profiles is nil-able: the job-search profile's base, read only when no contact source
	// states a location (WithProfileLocation).
	profiles ProfileReader
}

// NewAssembler builds an Assembler over its sources. screeningAnswers and bank may be nil.
func NewAssembler(cvs CVReader, resumes ResumeReader, accounts AccountReader, screeningAnswers ScreeningAnswersReader, bank BankReader) *Assembler {
	return &Assembler{cvs: cvs, resumes: resumes, accounts: accounts, screeningAnswers: screeningAnswers, bank: bank}
}

// Assemble builds a candidate's Profile from the first source that states a contact,
// backstopped by their account email.
//
// Sources are ordered: the base CV first (the candidate's corrected copy; the edit policy
// denies the tailoring agent the header), then the structured résumé that seeded it.
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
	// The header answers for the whole block, so a CV with a name but no location would hide the
	// résumé's location; ask the résumé for that field before the search profile.
	if profile.Location == "" {
		profile.Location = fromResume.Location
	}
	if profile.Location == "" {
		base, err := a.baseLocation(ctx, userID)
		if err != nil {
			return Profile{}, err
		}
		profile.Location = base
	}
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

// statesContact reports whether a header carries any contact value. A source that states
// nothing is passed over, so an empty CV does not silence a résumé.
func statesContact(h cv.Header) bool {
	return h.FullName != "" || h.Email != "" || h.Phone != "" || h.Location != "" || len(h.Links) > 0
}

// firstStatedHeader returns the first header that states any contact value, or the zero header.
//
// The chosen header answers for the WHOLE block, never merged across sources: the CV holds the
// corrected fields, so a merge would restore a value the candidate deleted.
func firstStatedHeader(headers ...cv.Header) cv.Header {
	for _, h := range headers {
		if statesContact(h) {
			return h
		}
	}
	return cv.Header{}
}

// ContactHeaderFromStructured projects the résumé's contact fields into a header, the same
// mapping cv.Seed makes.
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

// applyScreeningFields merges the caller's screening answers into the profile. No reader or no
// stored answers leaves the fields empty; any other error propagates.
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
	p.PrivacyConsent = fields["privacy_consent"]
	return nil
}
