package sources

import "testing"

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
