// Package formeremployer resolves the places a person has worked at — free-text company
// names in their experience bank — to the catalogue's company slugs, for the "Hiring again"
// search filter (`former_employers=true` on /jobs/search).
//
// The bank stores a company as the string a CV or a chat printed, never as a slug, so this
// is a name lookup, and a name lookup is wrong in a way nobody downstream can see: the feed
// would show a stranger company's postings under a familiar name. That is worse than an
// empty feed, so every rule here refuses rather than guesses. A name resolves only when:
//
//   - it names an employer at all — "Freelance", "Self-employed", "Confidential" and the
//     freelance marketplaces a person sold work through are refused (nonEmployers);
//   - the catalogue has settled it: the alias registry names ONE canonical slug for the
//     name's folded key, or, with no registry entry, exactly ONE company carrying jobs folds
//     to it. Two slugs sharing a fold (`vk` / `v-k`) is a name the catalogue has not
//     decided, and choosing either is the guess this refuses — the same rule
//     cmd/publish-logo-domains applies to a name two companies disagree about;
//   - a SHORT name is corroborated by place: a key of at most weakKeyRunes letters ("CARE",
//     "NTI", "e&") is where namesakes concentrate, so the banked role's location must name a
//     country the company is hiring in. Measured on prod 2026-10-31, every wrong match in a
//     hand-checked sample of uncorroborated resolutions was one of these.
//
// The key is normalize.CompanyKey — the fold the alias registry is keyed by and the one
// ingest applies to a posting's company — so a bank name and a posting name that would
// land on one company at ingest land on it here too. There is no second resolver.
package formeremployer

import (
	"slices"
	"unicode/utf8"

	"github.com/strelov1/freehire/internal/dict/location"
	"github.com/strelov1/freehire/internal/dict/normalize"
)

// weakKeyRunes is the longest folded key that needs a place to corroborate it. Measured on
// prod 2026-10-31 over 6,661 resolved bank names: the wrong matches a hand check found
// ("e&" on a Danish "E ApS", "CARE Pvt. Ltd." on the CARE charity, "Comfy", "Engen",
// "Saga", "Over") all fold to five letters or fewer, while longer names were right. The
// big short brands (IBM, EY, UBS, KPMG) still resolve whenever the role states a country
// they hire in, which most banked roles that state a place do.
const weakKeyRunes = 5

// nonEmployers are the bank entries that name no employer: self-employment in the
// languages the bank holds most, placeholders for an undisclosed one, and the freelance
// marketplaces a person sold work THROUGH — a former Upwork freelancer is not a former
// Upwork employee, and Upwork's own vacancies are not a return. Written as the names a CV
// prints and folded at init, so the list cannot drift from the key it is compared with.
var nonEmployerNames = []string{
	"Freelance", "Freelancer", "Freelancing", "Self-Employed", "Self Employment",
	"Independent", "Independent Contractor", "Independent Consultant", "Independent Projects",
	"Independiente", "Autónomo", "Autonomo", "Indépendant", "Freiberuflich", "Selbstständig",
	"Selbständig", "Фриланс", "Фрилансер", "Самозанятый", "Индивидуальный предприниматель",
	"ИП", "Confidential", "Confidential Company", "Undisclosed", "NDA", "Stealth",
	"Stealth Startup", "Stealth Mode Startup", "Startup", "Personal Projects",
	"Personal Project", "Various", "Various Clients", "Contract", "Contractor", "Consultant",
	"Consulting", "Company", "Group", "Private", "Private Company",
	"Upwork", "Fiverr", "Toptal", "Freelancer.com", "Kwork", "Workana", "PeoplePerHour",
}

var nonEmployers = foldAll(nonEmployerNames)

func foldAll(names []string) map[string]bool {
	out := make(map[string]bool, len(names))
	for _, n := range names {
		if k := normalize.CompanyKey(n); k != "" {
			out[k] = true
		}
	}
	return out
}

// Employment is one banked role: the company name and the location as stored.
type Employment struct {
	Company  string
	Location string
}

// Company is a catalogue company carrying searchable jobs.
type Company struct {
	Slug string
	// Countries are the ISO codes of the company's open jobs (companies.countries).
	Countries []string
}

// Key is the folded key a banked company name is looked up by, or "" for a name that is
// never looked up — empty after folding, or not an employer.
func Key(name string) string {
	k := normalize.CompanyKey(name)
	if nonEmployers[k] {
		return ""
	}
	return k
}

// Keys is the distinct lookup keys of a person's employments, sorted.
func Keys(emps []Employment) []string {
	var keys []string
	for _, e := range emps {
		if k := Key(e.Company); k != "" && !slices.Contains(keys, k) {
			keys = append(keys, k)
		}
	}
	slices.Sort(keys)
	return keys
}

// Resolve returns the company slugs a person's employments name unambiguously, sorted and
// distinct.
//
// canons maps a folded key to the canonical slugs the alias registry holds for it — more
// than one is a registry the writer was meant to keep at one, and is refused like any other
// contest. companies are the hiring companies that fold to one of the keys or ARE one of the
// canons (db.ListHiringCompaniesByFoldedSlug); a company absent from it carries no
// searchable job, which leaves its name nothing to show rather than something to guess at.
func Resolve(emps []Employment, canons map[string][]string, companies []Company) []string {
	bySlug := make(map[string]Company, len(companies))
	byFold := make(map[string][]Company, len(companies))
	for _, c := range companies {
		bySlug[c.Slug] = c
		f := normalize.FoldSlug(c.Slug)
		byFold[f] = append(byFold[f], c)
	}

	var out []string
	for _, e := range emps {
		key := Key(e.Company)
		if key == "" {
			continue
		}
		c, ok := settled(key, canons, bySlug, byFold)
		if !ok || slices.Contains(out, c.Slug) {
			continue
		}
		if utf8.RuneCountInString(key) <= weakKeyRunes && !sharesCountry(e.Location, c.Countries) {
			continue
		}
		out = append(out, c.Slug)
	}
	slices.Sort(out)
	return out
}

// settled is the one hiring company a key names, or false when the catalogue names none or
// has not decided between several. The registry, when it speaks, is the answer: it is the
// recorded decision about which spelling is the employer, and a not-yet-merged twin the
// fold also finds is exactly what that decision retires.
func settled(key string, canons map[string][]string, bySlug map[string]Company, byFold map[string][]Company) (Company, bool) {
	if cs := canons[key]; len(cs) > 0 {
		if len(cs) > 1 {
			return Company{}, false
		}
		c, ok := bySlug[cs[0]]
		return c, ok
	}
	if cs := byFold[key]; len(cs) == 1 {
		return cs[0], true
	}
	return Company{}, false
}

// sharesCountry reports whether a banked role's location names a country the company is
// hiring in. A location that names no country corroborates nothing.
func sharesCountry(loc string, hiring []string) bool {
	for _, c := range location.Parse(loc).Countries {
		if slices.Contains(hiring, c) {
			return true
		}
	}
	return false
}
