package main

import "testing"

func TestParseCards(t *testing.T) {
	// Two cards, the second one malformed (no title). Cards are parsed independently so a
	// markup change on one costs one posting rather than the whole page.
	html := `
<li>
  <div class="base-card" data-entity-urn="urn:li:jobPosting:4442681548">
    <a class="base-card__full-link" href="https://de.linkedin.com/jobs/view/backend-go-at-doodle-4442681548?trk=x"></a>
    <h3 class="base-search-card__title">Software Engineer, Backend (Go)</h3>
    <h4 class="base-search-card__subtitle"><a href="https://ch.linkedin.com/company/doodle-ag?trk=y">Doodle</a></h4>
  </div>
</li>
<li>
  <div class="base-card" data-entity-urn="urn:li:jobPosting:999">
    <h4 class="base-search-card__subtitle"><a href="https://linkedin.com/company/nameless">Nameless</a></h4>
  </div>
</li>
<li>
  <div class="base-card" data-entity-urn="urn:li:jobPosting:4430237612">
    <a class="base-card__full-link" href="https://nl.linkedin.com/jobs/view/pm-at-picnic-4430237612"></a>
    <h3 class="base-search-card__title">Product Manager</h3>
    <h4 class="base-search-card__subtitle"><a href="https://nl.linkedin.com/company/picnic-technologies">Picnic Technologies</a></h4>
  </div>
</li>`

	got := parseCards(html)
	if len(got) != 2 {
		t.Fatalf("parsed %d cards (%v), want 2 (the titleless one is dropped)", len(got), got)
	}
	first := got[0]
	if first.company != "Doodle" {
		t.Errorf("company = %q, want Doodle", first.company)
	}
	if first.profileURL != "https://ch.linkedin.com/company/doodle-ag" {
		t.Errorf("profileURL = %q, want the profile without its tracking query", first.profileURL)
	}
	if first.postingURL != "https://de.linkedin.com/jobs/view/backend-go-at-doodle-4442681548" {
		t.Errorf("postingURL = %q, want the canonical posting URL without its query", first.postingURL)
	}
	if got[1].company != "Picnic Technologies" {
		t.Errorf("second company = %q, want Picnic Technologies", got[1].company)
	}
}

func TestParseCardsOnEmptyMarkup(t *testing.T) {
	if got := parseCards("<html><body>nothing here</body></html>"); len(got) != 0 {
		t.Errorf("got %v, want no cards", got)
	}
}

func TestPostingExternalID(t *testing.T) {
	// The posting's own structured metadata carries its id in the employer's ATS.
	html := `<script type="application/ld+json">
	{"@context":"http://schema.org","@type":"JobPosting",
	 "identifier":{"@type":"PropertyValue","name":"Doodle","value":"teamtailor-8094978"},
	 "hiringOrganization":{"@type":"Organization","name":"Doodle"}}
	</script>`
	if got := postingExternalID(html); got != "teamtailor-8094978" {
		t.Errorf("postingExternalID = %q, want teamtailor-8094978", got)
	}
}

func TestPostingExternalIDWhenAbsent(t *testing.T) {
	cases := map[string]string{
		"no ld+json at all":  `<html><body>sign in to continue</body></html>`,
		"ld+json without id": `<script type="application/ld+json">{"@type":"JobPosting","title":"x"}</script>`,
		"unparsable ld+json": `<script type="application/ld+json">{not json</script>`,
	}
	for name, html := range cases {
		t.Run(name, func(t *testing.T) {
			if got := postingExternalID(html); got != "" {
				t.Errorf("got %q, want empty", got)
			}
		})
	}
}

func TestProfileWebsite(t *testing.T) {
	html := `<script type="application/ld+json">
	{"@context":"http://schema.org","@graph":[
	  {"@type":"Organization","name":"Doodle","sameAs":"https://doodle.com"}]}
	</script>`
	if got := profileWebsite(html); got != "https://doodle.com" {
		t.Errorf("profileWebsite = %q, want https://doodle.com", got)
	}
}

func TestProfileWebsiteWithoutGraphWrapper(t *testing.T) {
	// Some profiles publish the Organization node at the top level rather than inside @graph.
	html := `<script type="application/ld+json">{"@type":"Organization","sameAs":"https://acme.com"}</script>`
	if got := profileWebsite(html); got != "https://acme.com" {
		t.Errorf("profileWebsite = %q, want https://acme.com", got)
	}
}

func TestProfileWebsiteWhenAbsent(t *testing.T) {
	if got := profileWebsite(`<html>no structured data</html>`); got != "" {
		t.Errorf("got %q, want empty", got)
	}
}
