package main

import (
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/normalize"
)

// hit is one (provider, board) the resolve step wants written to a seed: either detected on a
// company's careers page, or — with expectID set — guessed offline for a company whose pages
// yielded nothing.
type hit struct{ provider, slug, company, expectID string }

// guessedCandidates proposes seed entries for a company the careers walk could not resolve,
// pairing every candidate slug with every provider the posting id's shape allows. It performs
// no I/O and yields nothing without an id, because the id is the only thing that makes a
// guessed slug safe to propose — harvest-boards keeps such a candidate only if the platform
// reports a live posting carrying it.
//
// The fan-out is bounded by construction: at most maxCandidateSlugs slugs times the providers
// one id shape can belong to (never more than two).
func guessedCandidates(site companySite) []hit {
	providers := providersForID(site.ExternalID)
	if len(providers) == 0 {
		return nil
	}
	slugs := candidateSlugs(site)
	out := make([]hit, 0, len(slugs)*len(providers))
	for _, p := range providers {
		for _, s := range slugs {
			out = append(out, hit{provider: p, slug: s, company: site.Name, expectID: site.ExternalID})
		}
	}
	return out
}

// maxCandidateSlugs bounds how many board slugs one company may propose. The slugs are
// guesses; the point of bounding them is that each one costs a probe at harvest-boards, and
// past three the guesses stop being informed by anything (a fourth spelling of a name we
// already tried three ways is not new evidence).
const maxCandidateSlugs = 3

// boardNameTails are the tails a platform profile slug carries while the board does not, BEYOND
// the corporate forms normalize.IsLegalForm knows. They are here rather than in the shared list
// because they are not legal forms and must not re-key the catalogue: "group" is part of a brand
// name, and "spa" collides with the literal word. Over-trimming is safe here and only here —
// this produces board-id GUESSES, where an extra candidate costs one lookup and a missing one
// loses the board.
var boardNameTails = []string{"group", "spa"}

// careersHostPrefixes are the sub-domains a company puts its careers site on. Their labels
// are not the company, so `jobs.picnic.app` must contribute `picnic`, not `jobs`.
var careersHostPrefixes = map[string]bool{
	"jobs": true, "careers": true, "career": true, "hire": true, "hiring": true,
	"work": true, "join": true, "apply": true, "www": true,
}

// candidateSlugs proposes the board slugs a company might use, drawn from what we already know
// about it: the label of its own domain, the slug of its platform profile (with and without a
// legal-form tail), and its normalized name. It performs no I/O.
//
// These are guesses and are only safe to emit alongside an expected posting id, which
// harvest-boards checks against the platform's live postings — a slug that happens to be some
// other employer's real board is then rejected on evidence rather than accepted on a
// resemblance. Results are sorted and de-duplicated so a run's seeds are deterministic.
// The order is the order of confidence, and the cap cuts from the tail: a company's own domain
// label is the best guess, its profile slug next, and a slug folded out of its display name
// last — so a bounded list keeps the guesses most likely to be right.
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
	// often as run together (`deliveryhero`), and neither is derivable from the other.
	nameSlug := trimLegalForm(normalize.Slug(site.Name))
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
	if normalize.IsLegalForm(tail) || slices.Contains(boardNameTails, tail) {
		return head
	}
	return slug
}

// lastSegment splits a slug before its final hyphen, so trimLegalForm can judge that segment
// on its own. Returns ok=false for a slug with no hyphen — there is no tail to consider, and
// trimming the whole thing would leave nothing to guess a board from.
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
	// Both spellings are worth trying: the profile keeps the form the board drops, but a
	// board is occasionally registered WITH it, so the trimmed slug is an extra candidate
	// rather than a replacement.
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
// belong to, so a candidate slug is only proposed to boards that could plausibly hold it.
//
// A shape that narrows to nothing yields nothing: fanning a guess across every provider would
// spend probes proportional to the provider count for no added evidence. The narrowing is a
// heuristic and is never trusted on its own — it only decides which boards get asked, and the
// id itself decides the answer.
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
