package handler

import (
	"context"
	"errors"
	"strings"

	"github.com/gofiber/fiber/v2"

	"github.com/strelov1/freehire/internal/browsertools"
	"github.com/strelov1/freehire/internal/cv"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/resumeextract"
	"github.com/strelov1/freehire/internal/screeninganswers"
)

// autofillProfile is the canonical set of fields the browser extension writes
// into application forms — assembled server-side so the extension stays dumb.
type autofillProfile struct {
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
}

// screeningAnswersReader is the one screening-answers read autofill makes.
type screeningAnswersReader interface {
	Get(ctx context.Context, userID int64) (screeninganswers.Answers, error)
}

// baseCVReader is the one CV read autofill makes. Tailored copies are excluded by the
// query behind it (GetBaseCVByUser filters NOT is_tailored), not by anything here: a
// tailored CV is written for one vacancy and the autofill block carries none.
type baseCVReader interface {
	BaseCV(ctx context.Context, userID int64) (cv.Record, bool, error)
}

// resumeContactReader is the one résumé read autofill makes. The bool reports whether a
// structure current with the uploaded résumé exists: the store compares the structure's
// stamp against the résumé's upload time, so one derived from a superseded file reads as
// absent rather than being served.
type resumeContactReader interface {
	Structured(ctx context.Context, userID int64) (resumeextract.Structured, bool, error)
}

// accountReader supplies the address that backstops a contact block stating no email.
type accountReader interface {
	GetUserByID(ctx context.Context, id int64) (db.GetUserByIDRow, error)
}

// autofillHandlers serves the contact block the browser extension fills application forms
// with, both as a plain read and as an agent-driven run over the caller's own browser.
//
// It holds the two contact sources in precedence order (see autofillProfile) plus the
// account read, the browser-tool hub the agent run drives the caller's page through, and
// the model binding that run plans on.
type autofillHandlers struct {
	cvs      baseCVReader
	resumes  resumeContactReader
	accounts accountReader
	// screeningAnswers supplies the candidate's own screening answers. Nil reads as "no
	// screening answers configured" — the block degrades to empty fields, same as an
	// unconfigured résumé reader elsewhere in this handler layer.
	screeningAnswers screeningAnswersReader
	// browserTools is shared with /tools/ws and the assistant — the hub is per-process and
	// routes strictly within one user's channel.
	browserTools *browsertools.Hub
	// llm plans the agent-driven fill. A nil client is the LLM being unconfigured: the run
	// reports the feature is off and the deterministic read still works.
	llm llmBinding
}

func newAutofillHandlers(cvs baseCVReader, resumes resumeContactReader, accounts accountReader, screeningAnswers screeningAnswersReader, tools *browsertools.Hub, llm llmBinding) *autofillHandlers {
	return &autofillHandlers{cvs: cvs, resumes: resumes, accounts: accounts, screeningAnswers: screeningAnswers, browserTools: tools, llm: llm}
}

func (h *autofillHandlers) register(api fiber.Router, mw middleware) {
	// Canonical autofill fields (name/email/phone/location/links) for the browser
	// extension to write into application forms. keyAuth (Bearer).
	api.Get("/me/autofill-profile", mw.key, h.AutofillProfile)
	// Agent-driven autofill: the caller's own browser is driven over the browser-tool
	// wire. keyAuth admits the cookie too, same as every other mw.key route — the
	// handler itself refuses anything but the extension's own Bearer session JWT,
	// because unlike a read this one writes into a live form.
	api.Post("/me/autofill/run", mw.key, h.RunAgentAutofill)
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

// contactHeaderFromStructured projects the structured résumé's contact fields into a
// header, so the résumé can be offered to firstStatedHeader as one more source. It is the
// same five-field mapping cv.Seed makes when it seeds a CV from the same structure.
func contactHeaderFromStructured(st resumeextract.Structured) cv.Header {
	return cv.Header{
		FullName: st.FullName,
		Email:    st.Email,
		Phone:    st.Phone,
		Location: st.Location,
		Links:    st.Links,
	}
}

// buildAutofillProfile projects a CV contact header (plus the account email as a
// fallback) into the canonical autofill fields, splitting the name and sorting
// links into linkedin / github / portfolio.
func buildAutofillProfile(h cv.Header, accountEmail string) autofillProfile {
	first, last := splitName(h.FullName)
	email := h.Email
	if email == "" {
		email = accountEmail
	}
	p := autofillProfile{
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

// autofillProfile assembles a user's canonical autofill fields from the first source that
// states a contact, backstopped by their account email. Shared by the endpoint the
// extension reads and the agent that fills forms with it, so the values a person sees in a
// form and the values the agent plans on cannot diverge.
//
// The sources are ordered: the base CV first, because it is the copy the candidate
// authored and the only one the tailoring agent cannot rewrite (the edit policy denies it
// the header's name, email, phone and links); the structured résumé second, because it is
// what seeded that CV in the first place. Corrected beats derived.
func (h *autofillHandlers) autofillProfile(ctx context.Context, userID int64) (autofillProfile, error) {
	var fromCV cv.Header
	if rec, ok, err := h.cvs.BaseCV(ctx, userID); err != nil {
		return autofillProfile{}, err
	} else if ok {
		fromCV = rec.Document.Header
	}

	var fromResume cv.Header
	if st, ok, err := h.resumes.Structured(ctx, userID); err != nil {
		return autofillProfile{}, err
	} else if ok {
		fromResume = contactHeaderFromStructured(st)
	}

	account, err := h.accounts.GetUserByID(ctx, userID)
	if err != nil {
		return autofillProfile{}, err
	}

	profile := buildAutofillProfile(firstStatedHeader(fromCV, fromResume), account.Email)
	if err := applyScreeningFields(ctx, &profile, h.screeningAnswers, userID); err != nil {
		return autofillProfile{}, err
	}
	return profile, nil
}

// applyScreeningFields merges the caller's screening answers into the profile. Best-effort
// in the same shape as the CV/résumé reads' "not found" branches: no reader configured or
// no answers stated both leave the screening fields empty rather than failing the whole
// read. Any OTHER error is real and propagates.
func applyScreeningFields(ctx context.Context, p *autofillProfile, reader screeningAnswersReader, userID int64) error {
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

// AutofillProfile returns the caller's canonical autofill fields. keyAuth so the
// browser extension (Bearer) can read it. All-empty (bar the account email) when no
// source states a contact.
func (h *autofillHandlers) AutofillProfile(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	profile, err := h.autofillProfile(c.Context(), userID)
	if err != nil {
		return err
	}
	return c.JSON(fiber.Map{"data": profile})
}
