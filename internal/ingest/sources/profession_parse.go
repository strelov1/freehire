package sources

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/dict/location"
	"golang.org/x/net/html"
)

// professionBodySections are the page's body sections, in the order the platform renders them.
// They are selected by element id rather than by heading, because the heading is the employer's
// own words ("Feladatok", "A pozíció leírása", "Responsibilities", ...). #c_info and #contact_info
// are absent on purpose: the first is the employer's standing blurb repeated across its postings,
// the second is where a postal address or a recruiter's mail lands.
var professionBodySections = []string{"tasks", "requirements", "other", "offer", "workplace_extras"}

// professionBody assembles the posting's body from the page's own sections, each under the
// heading its box states. The ld+json description has its list markup flattened, so it is not read.
func professionBody(root *html.Node) string {
	var b strings.Builder
	for _, id := range professionBodySections {
		section := firstByID(root, id)
		if section == nil {
			continue
		}
		body := strings.TrimSpace(innerHTML(section))
		if body == "" {
			continue
		}
		if heading := professionSectionHeading(section); heading != "" {
			fmt.Fprintf(&b, "<h3>%s</h3>", html.EscapeString(heading))
		}
		b.WriteString(body)
	}
	return b.String()
}

// professionSectionHeading returns the heading the section's box container states, or ""
// when the page renders none. The heading is a sibling of the section rather than a
// child, so it is reached by walking up to the box and back down to its <h2>.
func professionSectionHeading(section *html.Node) string {
	for n := section.Parent; n != nil; n = n.Parent {
		if n.Type != html.ElementNode || !HasClass(n, "box-container") {
			continue
		}
		if h2 := firstByTag(n, "h2"); h2 != nil {
			return strings.Join(strings.Fields(textContent(h2)), " ")
		}
		return ""
	}
	return ""
}

// professionWorkModeBadges maps the badge the page prints ahead of the address onto the work
// arrangement. It is the only thing that separates a hybrid posting from a remote one; the ld+json
// spends a single TELECOMMUTE flag on both. "Országos lefedettség" ("nationwide coverage") names
// no arrangement and no place, so it resolves to neither.
var professionWorkModeBadges = map[string]string{
	"Hibrid":               "hybrid",
	"Távmunka / Remote":    "remote",
	"Országos lefedettség": "",
}

// professionDigits matches a run of digits: a postal code or a street number.
var professionDigits = regexp.MustCompile(`\d+`)

// professionPlace splits the page's address line into the work mode its badge states and the
// place the location dictionary can read. A line with no badge is an ordinary sited posting.
//
// The digits go because the postal code glued to the city inside one comma-token stops the
// dictionary: it resolves "Budapest" and nothing in "1087 Budapest, Kerepesi út 21.". The street
// name is left in place, since removing it would mean guessing which token is the city.
func professionPlace(addressLine string) (workMode, place string) {
	line := strings.Join(strings.Fields(addressLine), " ")
	badge, rest, separated := strings.Cut(line, "•")
	badge, rest = strings.TrimSpace(badge), strings.TrimSpace(rest)
	mode, isBadge := professionWorkModeBadges[badge]
	switch {
	case isBadge && separated:
		return mode, professionStripDigits(rest)
	case isBadge:
		return mode, ""
	}
	return "onsite", professionStripDigits(line)
}

// professionStripDigits removes digit runs and tidies the whitespace they leave behind.
func professionStripDigits(place string) string {
	return strings.TrimSpace(strings.Join(strings.Fields(professionDigits.ReplaceAllString(place, " ")), " "))
}

// professionCountries reads the country the platform states, in one of two mutually exclusive
// places: a sited posting carries jobLocation.address.addressCountry ("HU"), a not-strictly-onsite
// one carries applicantLocationRequirements, naming the country in Hungarian or English. Both go
// through the shared dictionary, so an unknown name (a posting sited abroad, "Külföld") yields
// nothing rather than a guess.
func professionCountries(ld professionLDPosting) []string {
	if code := location.NormalizeCountry(ld.JobLocation.Address.Country); code != "" {
		return []string{code}
	}
	var out []string
	for _, req := range ld.ApplicantLocationRequirements {
		if code := location.NormalizeCountry(req.Name); code != "" && !slices.Contains(out, code) {
			out = append(out, code)
		}
	}
	return out
}
