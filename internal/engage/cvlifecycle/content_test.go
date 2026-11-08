package cvlifecycle

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"

	"github.com/strelov1/freehire/internal/candidate/atscheck"
	"github.com/strelov1/freehire/internal/candidate/resume"
	"github.com/strelov1/freehire/internal/identity/promo"
	"github.com/strelov1/freehire/internal/identity/userprofile"
	"github.com/strelov1/freehire/internal/search/search"
)

// The CV carries lines that must never reach a letter.
const secretCV = `JANE EXAMPLE
jane.example@example.test
+1 555 0100
Experience
Worked on ZZ-PRIVATE-PROJECT with Go and Kubernetes.`

type fakeResumes struct {
	enabled bool
	text    string
	err     error
}

func (r fakeResumes) Enabled() bool                               { return r.enabled }
func (r fakeResumes) Text(context.Context, int64) (string, error) { return r.text, r.err }

type fakeProfiles struct {
	profile userprofile.Profile
	err     error
}

func (p fakeProfiles) Get(context.Context, int64) (userprofile.Profile, error) {
	return p.profile, p.err
}

type fakeFacets struct {
	params search.FacetParams
	skills map[string]int64
	err    error
}

func (f *fakeFacets) FacetCounts(_ context.Context, p search.FacetParams) (search.FacetResult, error) {
	f.params = p
	return search.FacetResult{Facets: map[string]map[string]int64{"skills": f.skills}}, f.err
}

type fakePromos struct {
	percent     int16
	previewErr  error
	held        promo.Discount
	discountErr error
	previewed   []string
}

func (p *fakePromos) Preview(_ context.Context, _ int64, code string) (int16, error) {
	p.previewed = append(p.previewed, code)
	return p.percent, p.previewErr
}

func (p *fakePromos) Discount(context.Context, int64) (promo.Discount, error) {
	return p.held, p.discountErr
}

func newContent(resumes fakeResumes, facets *fakeFacets, promos *fakePromos, code string) *Content {
	profiles := fakeProfiles{profile: userprofile.Profile{Specializations: []string{"backend"}}}
	return NewContent(resumes, profiles, facets, promos, code)
}

func TestContent_ScoreLettersUseTheSitesInputs(t *testing.T) {
	facets := &fakeFacets{skills: map[string]int64{"go": 90, "kubernetes": 80, "rust": 70}}
	c := newContent(fakeResumes{enabled: true, text: secretCV}, facets, &fakePromos{}, "")

	first, err := c.Compose(context.Background(), StepATS, 7)
	if err != nil {
		t.Fatalf("step 1: %v", err)
	}
	if got := facets.params.Filter; got == nil || fmt.Sprint(got) != fmt.Sprint(search.FilterFromValues(url.Values{"category": {"backend"}})) {
		t.Errorf("the role filter was %v, want the profile's specializations", got)
	}
	if len(facets.params.Facets) != 1 || facets.params.Facets[0] != "skills" {
		t.Errorf("facets asked for %v", facets.params.Facets)
	}
	if first.Step != StepATS || first.Overall <= 0 || first.Potential < first.Overall {
		t.Errorf("letter 1 = %+v", first)
	}
	if len(first.Gaps) == 0 || len(first.Gaps) > 3 {
		t.Errorf("gaps = %v, want one to three", first.Gaps)
	}
	if !contains(first.Keywords, "Rust") {
		t.Errorf("keywords = %v, want the in-demand skill the CV lacks", first.Keywords)
	}

	second, err := c.Compose(context.Background(), StepPotential, 7)
	if err != nil {
		t.Fatalf("step 2: %v", err)
	}
	if second.Overall != first.Overall || second.Potential != first.Potential || second.Gaps != nil {
		t.Errorf("letter 2 = %+v, want the same score and no gap list", second)
	}
}

func TestContent_LetterCarriesNothingFromTheCV(t *testing.T) {
	c := newContent(fakeResumes{enabled: true, text: secretCV}, &fakeFacets{skills: map[string]int64{"rust": 1}}, &fakePromos{}, "")
	for _, step := range []Step{StepATS, StepPotential} {
		l, err := c.Compose(context.Background(), step, 7)
		if err != nil {
			t.Fatal(err)
		}
		dump := strings.ToLower(fmt.Sprintf("%+v", l))
		for _, secret := range []string{"jane", "jane.example", "555", "zz-private"} {
			if strings.Contains(dump, secret) {
				t.Errorf("step %d letter carries %q from the CV: %s", step, secret, dump)
			}
		}
	}
}

func TestContent_MissingProfileScoresAgainstTheWholeMarket(t *testing.T) {
	c := NewContent(fakeResumes{enabled: true, text: secretCV}, fakeProfiles{err: userprofile.ErrNotFound}, &fakeFacets{}, &fakePromos{}, "")
	if _, err := c.Compose(context.Background(), StepATS, 7); err != nil {
		t.Fatalf("a missing profile must not stop the letter: %v", err)
	}
}

func TestContent_ScoreInputsThatAreUnavailableAreNotLetters(t *testing.T) {
	boom := errors.New("backend down")
	cases := map[string]struct {
		c        *Content
		wantSkip bool
	}{
		"storage off":        {newContent(fakeResumes{}, &fakeFacets{}, &fakePromos{}, ""), true},
		"no stored CV":       {newContent(fakeResumes{enabled: true, err: resume.ErrNotStored}, &fakeFacets{}, &fakePromos{}, ""), true},
		"CV unreadable":      {newContent(fakeResumes{enabled: true, err: boom}, &fakeFacets{}, &fakePromos{}, ""), false},
		"search down":        {newContent(fakeResumes{enabled: true, text: secretCV}, &fakeFacets{err: boom}, &fakePromos{}, ""), false},
		"profile unreadable": {NewContent(fakeResumes{enabled: true, text: secretCV}, fakeProfiles{err: boom}, &fakeFacets{}, &fakePromos{}, ""), false},
	}
	for name, tc := range cases {
		_, err := tc.c.Compose(context.Background(), StepATS, 7)
		if err == nil || errors.Is(err, ErrSkip) != tc.wantSkip {
			t.Errorf("%s: err = %v, want skip=%v", name, err, tc.wantSkip)
		}
	}
}

func TestLetterFromReport_OnlySaysWhatIsTrue(t *testing.T) {
	perfect := atscheck.Report{Overall: 100, Potential: 100}
	for _, step := range []Step{StepATS, StepPotential} {
		if _, err := letterFromReport(step, perfect); !errors.Is(err, ErrSkip) {
			t.Errorf("step %d for a perfect CV: err = %v, want a skip", step, err)
		}
	}
	room := atscheck.Report{Overall: 60, Potential: 80, RecommendedKeywords: []string{"go"}}
	if l, err := letterFromReport(StepPotential, room); err != nil || l.Potential != 80 {
		t.Errorf("step 2 with room to grow: %+v, %v", l, err)
	}
}

func TestContent_UnknownStepIsAnError(t *testing.T) {
	if _, err := (&Content{}).Compose(context.Background(), Step(9), 7); err == nil || errors.Is(err, ErrSkip) {
		t.Errorf("err = %v, want a real error", err)
	}
}

func TestContent_OfferStatesWhatThePromoCheckReturned(t *testing.T) {
	promos := &fakePromos{percent: 23}
	c := newContent(fakeResumes{}, &fakeFacets{}, promos, "  zzoffer ")
	l, err := c.Compose(context.Background(), StepOffer, 7)
	if err != nil {
		t.Fatalf("offer: %v", err)
	}
	if l.Offer == nil || l.Offer.Percent != 23 || l.Offer.Code != "zzoffer" {
		t.Fatalf("offer = %+v, want the previewed percent and the configured code", l.Offer)
	}
	if len(promos.previewed) != 1 {
		t.Errorf("previewed %v, want one check at send time", promos.previewed)
	}
}

func TestContent_OfferIsSkippedWhenTheCheckRefuses(t *testing.T) {
	cases := map[string]struct {
		code   string
		promos *fakePromos
	}{
		"code unset":       {"", &fakePromos{percent: 15}},
		"code unusable":    {"ZZOFFER", &fakePromos{previewErr: promo.ErrNotUsable}},
		"already redeemed": {"ZZOFFER", &fakePromos{previewErr: promo.ErrAlreadyRedeemed}},
		"worth nothing":    {"ZZOFFER", &fakePromos{percent: 0}},
		"holds a discount": {"ZZOFFER", &fakePromos{percent: 15, held: promo.Discount{Percent: 15, Source: promo.SourceInvite}}},
	}
	for name, tc := range cases {
		l, err := newContent(fakeResumes{}, &fakeFacets{}, tc.promos, tc.code).Compose(context.Background(), StepOffer, 7)
		if !errors.Is(err, ErrSkip) || l.Offer != nil {
			t.Errorf("%s: letter %+v err %v, want a skip", name, l, err)
		}
		if err != nil && tc.code != "" && strings.Contains(err.Error(), tc.code) {
			t.Errorf("%s: the code is in the error text: %v", name, err)
		}
	}
}

func TestContent_OfferInfrastructureErrorsAreNotSkips(t *testing.T) {
	boom := errors.New("db down")
	for name, promos := range map[string]*fakePromos{
		"discount unreadable": {discountErr: boom},
		"preview unreadable":  {previewErr: boom},
	} {
		_, err := newContent(fakeResumes{}, &fakeFacets{}, promos, "ZZOFFER").Compose(context.Background(), StepOffer, 7)
		if err == nil || errors.Is(err, ErrSkip) {
			t.Errorf("%s: err = %v, want a real error", name, err)
		}
	}
}

func contains(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
