package sources

import (
	"context"
	"golang.org/x/net/html"
	"testing"
)

func TestLinkedInCompanyWebsiteReadsOnlyOrganizationMetadata(t *testing.T) {
	for _, tc := range []struct{ body, want string }{
		{`<script type="application/ld+json">{"@type":"Organization","sameAs":["https://www.linkedin.com/company/acme/","https://acme.example/"]}</script>`, "https://acme.example/"},
		{`<script type="application/ld+json">{"@type":"Organization","sameAs":"https://www.gistworld.com/"}</script>`, "https://www.gistworld.com/"},
		{`<script type="application/ld+json">{"@graph":[{"@type":"Organization","sameAs":["https://acme.example/"]}]}</script>`, "https://acme.example/"},
		{`<script type="application/ld+json">{"@type":"WebPage","sameAs":"https://unrelated.example/"}</script>`, ""},
		{`<script type="application/ld+json">{"@type":"Organization","sameAs":"javascript:alert(1)"}</script>`, ""},
	} {
		if got := LinkedInCompanyWebsite(parseHTML(t, tc.body)); got != tc.want {
			t.Fatalf("website = %q, want %q", got, tc.want)
		}
	}
}

type companyProfileResponse struct {
	root  *html.Node
	final string
	calls int
}

func (g *companyProfileResponse) GetHTMLResolved(context.Context, string) (*html.Node, string, error) {
	g.calls++
	return g.root, g.final, nil
}
func TestLinkedInCompanyWebsiteValidatesIdentityAndSharesTheRequestBudget(t *testing.T) {
	const profile = "https://www.linkedin.com/company/acme/"
	for _, final := range []string{profile, "https://www.linkedin.com/authwall", "https://outside.example/company/acme/", "http://www.linkedin.com/company/acme/"} {
		getter := &companyProfileResponse{root: parseHTML(t, `<script type="application/ld+json">{"@type":"Organization","sameAs":"https://acme.example/"}</script>`), final: final}
		gate := &recordingWaiter{}
		source := linkedIn{http: getter, pace: gate}
		if _, err := source.CompanyWebsite(context.Background(), "https://outside.example/company/acme/"); err == nil || getter.calls != 0 || gate.calls != 0 {
			t.Fatal("invalid profile consumed a request")
		}
		website, err := source.CompanyWebsite(context.Background(), profile)
		if (err == nil) != (final == profile) || getter.calls != 1 || gate.calls != 1 {
			t.Fatalf("profile result: website=%s error=%v requests=%d budget=%d", website, err, getter.calls, gate.calls)
		}
		if err == nil && website != "https://acme.example/" {
			t.Fatalf("wrong employer website: %s", website)
		}
	}
}
