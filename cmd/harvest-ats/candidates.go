package main

import (
	"net/url"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/dict/normalize"
)

// hit is one (provider, board) the resolve step wants written to a seed: detected on a
// company's careers page, or, with expectID set, guessed offline.
type hit struct{ provider, slug, company, expectID string }

// guessedCandidates proposes seed entries for a company the careers walk could not resolve,
// pairing every candidate slug with every provider the posting id's shape allows. It performs
// no I/O and yields nothing without an id: the id is what makes a guessed slug safe, since
// harvest-boards keeps such a candidate only if the platform reports a live posting carrying
// it. The fan-out is at most maxCandidateSlugs slugs times two providers.
func guessedCandidates(site companySite) []hit {
	providers := providersForID(site.ExternalID)
	if len(providers) == 0 {
		return nil
	}
	slugs := candidateSlugs(site)
	out := make([]hit, 0, len(slugs)*len(providers))
	for _, p := range providers {
		for _, s := range slugs {
			out = append(out, hit{provider: p, slug: boardFor(p, s), company: site.Name, expectID: site.ExternalID})
		}
	}
	return out
}

// hostKeyedVendorDomain names, for platforms that key a board by the whole careers HOST
// (atsboard's modeHost), the vendor domain a tenant sits under. A guess for such a platform
// must be a host: its prober fetches https://<board>/jobs, so a bare slug can only answer no.
//
// Only the id-prefix platforms need an entry, since only they can be guessed offline (see
// providersForID); Greenhouse, Lever and Ashby key by bare slug. A tenant on its own custom
// domain stays underivable.
var hostKeyedVendorDomain = map[string]string{"teamtailor": "teamtailor.com"}

// boardFor shapes a guessed slug into the board key its platform actually uses.
func boardFor(provider, slug string) string {
	if domain, ok := hostKeyedVendorDomain[provider]; ok {
		return slug + "." + domain
	}
	return slug
}

// maxCandidateSlugs bounds how many board slugs one company may propose. Each guess costs a
// probe at harvest-boards, and past three the guesses add no new evidence.
const maxCandidateSlugs = 3

// careersHostPrefixes are the sub-domains a company puts its careers site on. Their labels
// are not the company, so `jobs.picnic.app` must contribute `picnic`, not `jobs`.
var careersHostPrefixes = map[string]bool{
	"jobs": true, "careers": true, "career": true, "hire": true, "hiring": true,
	"work": true, "join": true, "apply": true, "www": true,
}

// candidateSlugs proposes the board slugs a company might use: the label of its own domain,
// the slug of its platform profile (with and without a legal-form tail), and its normalized
// name. It performs no I/O.
//
// These are guesses, safe to emit only alongside an expected posting id, which harvest-boards
// checks against the platform's live postings. Results are sorted and de-duplicated so seeds
// are deterministic. The order is confidence order and the cap cuts from the tail: own domain
// label, then profile slug, then a slug folded out of the display name.
func candidateSlugs(site companySite) []string {
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		if s == "" || seen[s] || len(out) >= maxCandidateSlugs {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	add(domainLabel(site.Website))
	for _, s := range profileSlugs(site.LinkedIn) {
		add(s)
	}
	// Both spellings of the name: boards are registered hyphenated (`delivery-hero`) about as
	// often as run together (`deliveryhero`).
	//
	// BoardNameSlug rather than trimLegalForm(Slug(name)): trimming after slugging sees a tail
	// the slug already broke up ("Acme S.p.A." is `acme-s-p-a`), and BoardNameSlug repeats, so a
	// compound form comes off whole ("Atlassian Pty Ltd" → `atlassian`). trimLegalForm stays
	// for profile slugs, which have no name to go back to.
	nameSlug := normalize.BoardNameSlug(site.Name)
	add(nameSlug)
	add(strings.ReplaceAll(nameSlug, "-", ""))
	return out
}

// trimLegalForm drops a trailing company-form label from a slug (`delivery-hero-se` →
// `delivery-hero`), which boards routinely omit.
func trimLegalForm(slug string) string {
	head, tail, ok := lastSegment(slug)
	if !ok {
		return slug
	}
	if normalize.IsLegalForm(tail) || normalize.IsBoardNameTail(tail) {
		return head
	}
	return slug
}

// lastSegment splits a slug before its final hyphen, so trimLegalForm can judge that segment
// alone. ok=false for a slug with no hyphen: trimming the whole thing would leave nothing.
func lastSegment(slug string) (head, tail string, ok bool) {
	i := strings.LastIndexByte(slug, '-')
	if i <= 0 {
		return "", "", false
	}
	return slug[:i], slug[i+1:], true
}

// domainLabel returns the company-bearing label of a website's host: the registrable name with
// any careers sub-domain stripped (jobs.picnic.app → picnic, www.deliveryhero.com →
// deliveryhero). It returns "" when the URL carries no usable host.
func domainLabel(website string) string {
	if website == "" {
		return ""
	}
	if !strings.Contains(website, "//") {
		website = "https://" + website
	}
	u, err := url.Parse(website)
	if err != nil {
		return ""
	}
	labels := strings.Split(strings.ToLower(u.Hostname()), ".")
	for len(labels) > 1 && careersHostPrefixes[labels[0]] {
		labels = labels[1:]
	}
	if len(labels) < 2 {
		return ""
	}
	return normalize.Slug(labels[0])
}

// profileSlugRe captures the slug out of a company profile URL
// (https://ch.linkedin.com/company/doodle-ag → doodle-ag).
var profileSlugRe = regexp.MustCompile(`/company/([a-z0-9][a-z0-9-]*)`)

// profileSlugs returns the profile's own slug and, when it ends in a legal-form tail, the slug
// without it — boards are commonly registered under the bare name.
func profileSlugs(profileURL string) []string {
	m := profileSlugRe.FindStringSubmatch(strings.ToLower(profileURL))
	if m == nil {
		return nil
	}
	slug := m[1]
	out := []string{slug}
	// Try both: the profile keeps the form the board drops, but a board is occasionally
	// registered WITH it.
	if bare := trimLegalForm(slug); bare != slug {
		out = append(out, bare)
	}
	return out
}

var (
	// greenhouseIDRe is Greenhouse's job id: a ten-digit number.
	greenhouseIDRe = regexp.MustCompile(`^\d{10}$`)
	// uuidRe is the id shape Lever and Ashby both use for a posting.
	uuidRe = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	// prefixedIDRe is the shape that names its own platform (teamtailor-8094978).
	prefixedIDRe = regexp.MustCompile(`^([a-z][a-z0-9]+)-\d+$`)
)

// knownPrefixes are the platform names an id may carry as its own prefix.
var knownPrefixes = map[string]bool{"teamtailor": true, "greenhouse": true, "lever": true, "ashby": true}

// providersForID narrows an ATS-native posting id to the providers whose id space it could
// belong to, so a candidate slug is only proposed to boards that could hold it. A shape that
// narrows to nothing yields nothing: fanning out would spend probes for no added evidence.
// A heuristic only: it decides which boards get asked, and the id itself decides the answer.
func providersForID(externalID string) []string {
	id := strings.ToLower(strings.TrimSpace(externalID))
	switch {
	case id == "":
		return nil
	case greenhouseIDRe.MatchString(id):
		return []string{"greenhouse"}
	case uuidRe.MatchString(id):
		return []string{"ashby", "lever"}
	}
	if m := prefixedIDRe.FindStringSubmatch(id); m != nil && knownPrefixes[m[1]] {
		return []string{m[1]}
	}
	return nil
}
