package cvlifecycle

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/strelov1/freehire/internal/candidate/atscheck"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/dict/skilltag"
	"github.com/strelov1/freehire/internal/identity/promo"
	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/search/search"
)

// ErrSkip says a letter does not apply to this account right now. The step stays pending.
var ErrSkip = errors.New("cvlifecycle: letter does not apply")

// Letter is what one letter says about one account. It carries scores, fixed check wording
// and skill names from the dictionary — never a line of the CV, and never a name, address
// or contact detail, so nothing here can leak one into a mail or a log.
type Letter struct {
	Step Step
	// Overall and Potential are the ATS score now and after tailoring (letters 1 and 2).
	Overall, Potential int
	// Gaps are the biggest recoverable checks (letter 1).
	Gaps []string
	// Keywords are in-demand skills for the account's role that the CV does not show.
	Keywords []string
	// Offer is the first-month discount (letter 3 only).
	Offer *Offer
}

// Offer is a promo code that the promo service has just confirmed is usable by this account.
type Offer struct {
	Code    string
	Percent int16
}

const (
	maxGaps     = 3
	maxKeywords = 5
)

// Resumes reads the stored CV text. *resume.Store satisfies it.
type Resumes interface {
	Enabled() bool
	Text(ctx context.Context, userID int64) (string, error)
}

// Profiles reads an account's profile. *userprofile.Service satisfies it.
type Profiles interface {
	Get(ctx context.Context, userID int64) (userprofile.Profile, error)
}

// Facets counts the skills a role's postings ask for. *search.Client satisfies it.
type Facets interface {
	FacetCounts(ctx context.Context, p search.FacetParams) (search.FacetResult, error)
}

// Promos asks what a code is worth to an account. *promo.Service satisfies it.
type Promos interface {
	Preview(ctx context.Context, userID int64, code string) (int16, error)
	Discount(ctx context.Context, userID int64) (promo.Discount, error)
}

// Content composes letters. It scores a CV exactly as the site's ATS report does — the
// stored CV text, the skills found in it, the role's top skills from the search index — so
// the number in the mail is the number the account sees when it clicks through. No model.
type Content struct {
	resumes   Resumes
	profiles  Profiles
	facets    Facets
	promos    Promos
	promoCode string
}

// NewContent builds the composer. promoCode is the offer's code, from the environment; it
// is held in memory for the links and never written anywhere. Empty means no offer, which
// skips letter 3.
func NewContent(resumes Resumes, profiles Profiles, facets Facets, promos Promos, promoCode string) *Content {
	return &Content{
		resumes: resumes, profiles: profiles, facets: facets, promos: promos,
		promoCode: strings.TrimSpace(promoCode),
	}
}

// Compose implements Composer.
func (c *Content) Compose(ctx context.Context, step Step, userID int64) (Letter, error) {
	switch step {
	case StepATS, StepPotential:
		return c.score(ctx, step, userID)
	case StepOffer:
		return c.offer(ctx, userID)
	default:
		return Letter{}, fmt.Errorf("cvlifecycle: unknown step %d", step)
	}
}

func (c *Content) score(ctx context.Context, step Step, userID int64) (Letter, error) {
	if !c.resumes.Enabled() {
		return Letter{}, fmt.Errorf("%w: CV storage is off", ErrSkip)
	}
	cvText, err := c.resumes.Text(ctx, userID)
	if errors.Is(err, resume.ErrNotStored) {
		return Letter{}, fmt.Errorf("%w: no stored CV", ErrSkip)
	}
	if err != nil {
		return Letter{}, fmt.Errorf("reading the stored CV: %w", err)
	}

	profile, err := c.profiles.Get(ctx, userID)
	if err != nil && !errors.Is(err, userprofile.ErrNotFound) {
		return Letter{}, fmt.Errorf("reading the profile: %w", err)
	}
	res, err := c.facets.FacetCounts(ctx, search.FacetParams{
		Filter: search.FilterFromValues(url.Values{"category": profile.Specializations}),
		Facets: []string{"skills"},
	})
	if err != nil {
		return Letter{}, fmt.Errorf("reading the role's skills: %w", err)
	}

	cvSkills := skilltag.Parse(cvText, skilltag.WithResumeAcronyms())
	report := atscheck.Score(cvText, cvSkills, atscheck.TopRoleSkills(res.Facets["skills"], atscheck.RoleTopN))

	return letterFromReport(step, report)
}

// letterFromReport keeps a letter only when it has something true to say: letter 1 needs a
// gap or a keyword to name, letter 2 needs tailoring to be able to raise the score.
func letterFromReport(step Step, report atscheck.Report) (Letter, error) {
	letter := Letter{Step: step, Overall: report.Overall, Potential: report.Potential}
	if step == StepATS {
		letter.Gaps = report.TopGaps(maxGaps)
		letter.Keywords = labels(report.RecommendedKeywords, maxKeywords)
		if len(letter.Gaps) == 0 && len(letter.Keywords) == 0 {
			return Letter{}, fmt.Errorf("%w: nothing to improve", ErrSkip)
		}
	}
	if step == StepPotential && report.Potential <= report.Overall {
		return Letter{}, fmt.Errorf("%w: tailoring would add nothing", ErrSkip)
	}
	return letter, nil
}

func (c *Content) offer(ctx context.Context, userID int64) (Letter, error) {
	if c.promoCode == "" {
		return Letter{}, fmt.Errorf("%w: no offer code configured", ErrSkip)
	}
	// An account that already holds a discount — a redeemed code or a pending invite — is
	// not offered a second one: checkout admits a single coupon and the larger would win.
	held, err := c.promos.Discount(ctx, userID)
	if err != nil {
		return Letter{}, fmt.Errorf("reading the account's discount: %w", err)
	}
	if held.Percent > 0 {
		return Letter{}, fmt.Errorf("%w: account already has a discount", ErrSkip)
	}
	percent, err := c.promos.Preview(ctx, userID, c.promoCode)
	if errors.Is(err, promo.ErrNotUsable) || errors.Is(err, promo.ErrAlreadyRedeemed) {
		return Letter{}, fmt.Errorf("%w: code refused", ErrSkip)
	}
	if err != nil {
		return Letter{}, fmt.Errorf("previewing the offer: %w", err)
	}
	if percent <= 0 {
		return Letter{}, fmt.Errorf("%w: code is worth nothing", ErrSkip)
	}
	return Letter{Step: StepOffer, Offer: &Offer{Code: c.promoCode, Percent: percent}}, nil
}

// labels turns skill slugs into display names, keeping at most n.
func labels(slugs []string, n int) []string {
	var out []string
	for _, s := range slugs {
		if len(out) == n {
			break
		}
		if label := skilltag.Label(s); label != "" {
			out = append(out, label)
		}
	}
	return out
}
