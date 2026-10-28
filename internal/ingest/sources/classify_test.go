package sources

import (
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/dict/vocab"
)

// TestProviderKind pins the status-page taxonomy to the adapter markers: a
// board-based platform is an ATS, a boardless single-company adapter is that
// company's own careers page, a boardless many-company adapter is an aggregator,
// and an unregistered provider is "other".
func TestProviderKind(t *testing.T) {
	cases := map[string]string{
		"greenhouse":                KindATS,
		"workday":                   KindATS,
		"lever":                     KindATS,
		"apple":                     KindCompany,
		"google":                    KindCompany,
		"jobstash":                  KindAggregator,
		"nofluffjobs":               KindAggregator,
		"hackernews":                KindSocial,
		"telegram":                  KindSocial,
		"employer":                  KindCompany,
		"manual":                    KindOther,
		"definitely-not-a-provider": KindOther,
	}
	reg := Taxonomy()
	for provider, want := range cases {
		if got := ProviderKind(reg, provider); got != want {
			t.Errorf("ProviderKind(%q) = %q, want %q", provider, got, want)
		}
	}
}

func TestEmployerURLProvidersAcceptsATSAndCompanyAndRefusesAnAggregator(t *testing.T) {
	// The rule two surfaces publish attribution from, given its own test rather than being
	// checked only through whichever projector happens to call it.
	//
	// An aggregator's stored URL points at the aggregator. A surface that calls it the
	// employer's own page makes a claim we cannot vouch for, and renders it as a link a
	// person clicks expecting the employer.
	publishes := EmployerURLProviders()

	for _, provider := range []string{"greenhouse", "lever", "ashby"} {
		if !publishes[provider] {
			t.Errorf("%s is an ATS; its stored URL is the employer's own page", provider)
		}
	}
	for _, provider := range []string{"adzuna", "themuse"} {
		if publishes[provider] {
			t.Errorf("%s is an aggregator; its stored URL points at the aggregator", provider)
		}
	}
	if len(publishes) == 0 {
		t.Fatal("no provider publishes an employer URL; the taxonomy was not read")
	}
}

func TestEveryProviderKindIsASearchableSourceTypeOrOther(t *testing.T) {
	// The search facet's closed vocabulary lives in internal/dict/vocab, below this package,
	// so nothing but this test holds the two to each other. A kind outside it would be
	// written onto documents no filter value can reach.
	reg := Taxonomy()
	for provider := range reg {
		kind := ProviderKind(reg, provider)
		if kind != KindOther && !slices.Contains(vocab.SourceTypeValues, kind) {
			t.Errorf("ProviderKind(%q) = %q, which is not in vocab.SourceTypeValues", provider, kind)
		}
	}
	for _, kind := range vocab.SourceTypeValues {
		if kind == KindOther {
			t.Errorf("vocab.SourceTypeValues offers %q, which no document carries", kind)
		}
	}
}

func TestSourceTypesLeavesUnclassifiedSourcesBlank(t *testing.T) {
	typeOf := SourceTypes(Taxonomy())
	if got := typeOf("greenhouse"); got != KindATS {
		t.Errorf("greenhouse = %q, want %q", got, KindATS)
	}
	if got := typeOf("weblink"); got != "" {
		t.Errorf("weblink = %q, want blank: an unclassified source carries no source_type", got)
	}
}
