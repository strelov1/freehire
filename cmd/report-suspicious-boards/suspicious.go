package main

import (
	"cmp"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/dict/normalize"
	"github.com/strelov1/freehire/internal/job/logodomain"
	"github.com/strelov1/freehire/internal/platform/db"
)

// The four heuristics. Each is evidence for a person, never a decision.
const (
	reasonWebsiteMismatch = "website_mismatch"          // (a) no board of the company resembles its website
	reasonNameNotBoardID  = "name_not_board_id"         // (b) the board's company name is not what its id says
	reasonSameProviderOdd = "same_provider_dissimilar"  // (c) a sibling board on the same provider and slug looks like another employer
	reasonTokenNamesOther = "token_names_other_company" // (d) the board id is another company's slug or site
	chainBoards           = 3                           // boards on one slug+provider sharing one name: a chain or franchise
	minKey                = 3                           // a folded key shorter than this matches too much to compare
	minBrand              = 5                           // a brand head shorter than this is a word, not an employer
	labelSlugs            = 10                          // a board name over more slugs than this is a label
	maxForeign            = 5                           // a brand heading boards of more employers than this is a franchise or a word
)

type company struct{ Slug, Name, Website string }

type finding struct {
	CompanySlug, CompanyName, CompanyWebsite string
	Provider, Board, BoardCompany            string
	OpenJobs                                 int32
	Reason, Action, Confidence               string
}

// genericToken is a board-id head that names a careers site, not an employer
// (recruiting2.ultipro.com, careers.acme.com).
var genericToken = map[string]bool{
	"recruiting": true, "careers": true, "career": true, "jobs": true, "job": true, "www": true,
	"apply": true, "hr": true, "portal": true, "talent": true, "join": true, "work": true, "clients": true,
}

// tokenCore is the employer-naming part of a board id, folded: the first path segment, then
// its first host label, trailing digits dropped (jackson-hewitt-1674). ok is false for an
// opaque id (a uuid, a hex or numeric key, a generic careers host), which names nobody.
func tokenCore(board string) (string, bool) {
	s := strings.ToLower(board)
	s, _, _ = strings.Cut(s, "/")
	s, _, _ = strings.Cut(s, ".")
	if uuid.Validate(s) == nil {
		return "", false // a uuid may carry few digits (abcdefab-cdef-4abc-...): its shape, not its digits, makes it opaque
	}
	core := strings.TrimRightFunc(fold(s), unicode.IsDigit)
	digits := 0
	for _, r := range core {
		if unicode.IsDigit(r) {
			digits++
		}
	}
	if digits >= 3 || len(core) < minKey || genericToken[core] {
		return "", false
	}
	return core, true
}

func fold(s string) string {
	return strings.Map(func(r rune) rune {
		if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return unicode.ToLower(r)
		}
		return -1
	}, s)
}

// websiteLabel is the registrable label of a website: exa.ai -> exa, jobs.acme.co.uk -> acme.
func websiteLabel(website string) string {
	parts := strings.Split(logodomain.Domain(website), ".")
	if len(parts) < 2 {
		return ""
	}
	i := len(parts) - 2
	if len(parts) >= 3 && len(parts[len(parts)-1]) == 2 && slices.Contains([]string{"co", "com", "org", "net", "ac", "gov", "edu"}, parts[i]) {
		i--
	}
	if i+1 < len(parts) && (parts[i] == "about" || genericToken[parts[i]]) {
		i++ // about.google, careers.acme (a brand TLD): the employer is the next label
	}
	return fold(parts[i])
}

// alike is a prefix either way, both sides long enough to mean something: xebiausa is Xebia,
// gomotive is not motive.
func alike(a, b string) bool {
	return len(a) >= minKey && len(b) >= minKey && (strings.HasPrefix(a, b) || strings.HasPrefix(b, a))
}

// nameWords are the slug words of a company name worth matching on, plus its acronym.
func nameWords(name string) (words []string, acronym string) {
	for _, w := range strings.Split(normalize.CompanySlug(name), "-") {
		if w == "" || stopWord[w] {
			continue
		}
		acronym += w[:1]
		if len(w) >= minKey {
			words = append(words, w)
		}
	}
	return words, acronym
}

var stopWord = map[string]bool{"the": true, "and": true, "of": true, "for": true, "de": true, "la": true, "du": true, "der": true, "und": true}

// filler is what a board id may carry before the employer's name without naming another
// employer: jobsatphamily, yourgesture, beparallel (and a site: aboutamazon.com).
var filler = []string{"about", "jobsat", "careersat", "workat", "joinus", "join", "your", "team", "hire", "life", "the", "get", "my", "we", "be", "at"}

// matchesWords reports whether key (a folded board id or website label) starts with the name,
// a word of it or its acronym, after any filler.
func matchesWords(key, name string) bool {
	if alike(key, normalize.CompanyKey(name)) {
		return true
	}
	words, acronym := nameWords(name)
	for _, k := range keyForms(key) {
		if len(acronym) >= 2 && (strings.HasPrefix(k, acronym) || (len(k) >= 2 && strings.HasPrefix(acronym, k))) {
			return true
		}
		for _, w := range words {
			if strings.HasPrefix(k, w) || alike(k, w) {
				return true
			}
		}
	}
	return false
}

func keyForms(key string) []string {
	forms := []string{key}
	for _, f := range filler {
		if rest, ok := strings.CutPrefix(key, f); ok && len(rest) >= minKey {
			forms = append(forms, rest)
		}
	}
	return forms
}

// latin is whether a name is written mostly in ASCII letters; a romanised Japanese or Cyrillic
// name never spells its board id, so comparing them only produces noise.
func latin(name string) bool {
	ascii, other := 0, 0
	for _, r := range name {
		switch {
		case r < unicode.MaxASCII && unicode.IsLetter(r):
			ascii++
		case unicode.IsLetter(r):
			other++
		}
	}
	return ascii > 2*other
}

// namedProviders are the providers whose board ids name the employer on at least half the
// rows. On the rest (paycom's hex keys, UKG Ready's numeric tenants, a job-search slice) an id
// is no evidence, so the id-based heuristics skip them. Measured from the rows themselves, so
// a new adapter needs no list.
func namedProviders(rows []db.ListOpenJobsByCompanyBoardRow) map[string]bool {
	total, named := map[string]int{}, map[string]int{}
	for _, r := range rows {
		core, ok := tokenCore(r.Board)
		if !ok || !latin(r.BoardCompany) {
			continue
		}
		total[r.Provider]++
		if matchesWords(core, r.BoardCompany) {
			named[r.Provider]++
		}
	}
	out := map[string]bool{}
	for p, n := range total {
		out[p] = 2*named[p] >= n
	}
	return out
}

// suspicious runs the four heuristics over the first-party boards feeding each company.
func suspicious(rows []db.ListOpenJobsByCompanyBoardRow, companies []company) []finding {
	rows = employerRows(rows)
	bySlug := map[string]company{}
	siteOwner := map[string]string{} // website label -> the one company stating it, "" if shared
	for _, c := range companies {
		bySlug[c.Slug] = c
		if l := websiteLabel(c.Website); l != "" {
			if _, taken := siteOwner[l]; taken {
				siteOwner[l] = "" // two companies state the site: it names neither
			} else {
				siteOwner[l] = c.Slug
			}
		}
	}
	feeds := map[string][]db.ListOpenJobsByCompanyBoardRow{}
	slugOwner := map[string]string{} // folded slug -> slug
	for _, r := range rows {
		feeds[r.CompanySlug] = append(feeds[r.CompanySlug], r)
		slugOwner[normalize.FoldSlug(r.CompanySlug)] = r.CompanySlug
	}
	for _, c := range companies {
		slugOwner[normalize.FoldSlug(c.Slug)] = c.Slug
	}
	named := namedProviders(rows)
	brands := brandHeads(rows, named)

	var out []finding
	add := func(r db.ListOpenJobsByCompanyBoardRow, reason, action, confidence string) {
		c := bySlug[r.CompanySlug]
		out = append(out, finding{
			CompanySlug: r.CompanySlug, CompanyName: c.Name, CompanyWebsite: c.Website,
			Provider: r.Provider, Board: r.Board, BoardCompany: r.BoardCompany, OpenJobs: r.OpenJobs,
			Reason: reason, Action: action, Confidence: confidence,
		})
	}

	for slug, boards := range feeds {
		c := bySlug[slug]
		label := websiteLabel(c.Website)
		ownKey := normalize.FoldSlug(slug)
		sameName := map[string]int{} // provider + board name -> boards
		for _, r := range boards {
			sameName[r.Provider+"\x00"+r.BoardCompany]++
		}

		// (a) the company states a website no other company states (smartrecruiters.com or
		// xing.com as a "website" names nobody) and not one of its boards resembles it: by its
		// id (on a provider whose ids name employers) or by the name the board row states.
		if label != "" && siteOwner[label] != "" {
			resembled := false
			for _, r := range boards {
				core, ok := tokenCore(r.Board)
				byID := ok && named[r.Provider] && alike(label, core)
				byName := matchesWords(label, r.BoardCompany)
				if byID || byName {
					resembled = true
					break
				}
			}
			if !resembled {
				conf := "medium"
				if len(boards) >= 5 || resemblesAfterFiller(label, boards) {
					conf = "low" // a group under a parent's site, or gopigment.com for Pigment
				}
				for _, r := range boards {
					add(r, reasonWebsiteMismatch,
						"check this board against "+logodomain.Domain(c.Website)+"; rename a namesake off the slug (add-board --rename) or correct the company website", conf)
				}
			}
		}

		for _, r := range boards {
			core, ok := tokenCore(r.Board)
			if !ok || !named[r.Provider] {
				continue
			}
			chain := sameName[r.Provider+"\x00"+r.BoardCompany] >= chainBoards

			// (b) the board's stated name is not what its id names. Reported when the id starts
			// with ANOTHER employer's name (xebiacee named "Poland and Eastern Europe": Xebia is a
			// brand other boards carry). An id that merely shares nothing with its name is mostly
			// an abbreviation or a parent's tenant (nrd-careers, quickchek) and is not reported.
			if !chain && latin(r.BoardCompany) && !matchesWords(core, r.BoardCompany) {
				// Postings already on a slug under that brand harm nothing (a franchisee board
				// named after its operator); only those landing on another company are reported.
				if brand := brandHead(r.Provider, core, brands); brand != "" && !strings.HasPrefix(ownKey, brand) {
					add(r, reasonNameNotBoardID,
						"board id "+r.Board+" starts with the brand '"+brand+"', not \""+r.BoardCompany+"\"; find the board's real employer and add-board --rename", "high")
				}
			}

			// (c) a sibling on the same provider looks like nobody else on this slug.
			if !chain {
				siblings := 0
				similar := alike(core, ownKey) || alike(core, normalize.CompanyKey(c.Name)) ||
					alike(core, label) || alike(core, normalize.CompanyKey(r.BoardCompany))
				for _, s := range boards {
					if s.Provider != r.Provider || s.Board == r.Board {
						continue
					}
					if _, ok := tokenCore(s.Board); !ok {
						continue
					}
					siblings++
				}
				if siblings > 0 && !similar && !alikeAny(core, boards, r) {
					add(r, reasonSameProviderOdd,
						"verify "+r.Provider+"/"+r.Board+" is the same employer as its "+strconv.Itoa(siblings)+" sibling board(s) on "+slug+"; rename it off the slug if not", "low")
				}
			}

			// (d) the board id is another company's slug or the label of a website only it
			// states, and this slug is that id or extends it: ashby/exa feeding exa-labs while exa
			// exists, greenhouse/gomotive feeding gomotive while motive states gomotive.com. A
			// shared word alone (woodward, viva) is not reported.
			if strings.HasPrefix(ownKey, core) {
				other, conf := siteOwner[core], "high"
				if core == ownKey && ownSite(label, core) {
					other = "" // the slug states its own site (amazon at aboutamazon.com): the id is its own
				}
				if (other == "" || other == slug) && len(core) > minKey {
					// A three-letter id (lex4, dev2) names too many employers to mean one.
					other, conf = slugOwner[core], "medium"
				}
				// A twin that folds to this slug is cmd/merge-companies' backlog, not a namesake.
				if other != "" && other != slug && normalize.FoldSlug(other) != ownKey {
					add(r, reasonTokenNamesOther,
						"board id names company "+other+"; if one employer, propose a 'same' merge "+slug+" -> "+other+" (rename "+other+"'s namesake boards off it first)", conf)
				}
			}
		}
	}
	slices.SortFunc(out, func(a, b finding) int {
		if d := cmp.Compare(b.OpenJobs, a.OpenJobs); d != 0 {
			return d
		}
		return cmp.Or(cmp.Compare(a.CompanySlug, b.CompanySlug), cmp.Compare(a.Board, b.Board), cmp.Compare(a.Reason, b.Reason))
	})
	return out
}

// brandHeads are, per provider, the employer names that head board ids as brands: on at
// least half the boards whose id starts with the name, the row names that employer too.
// Xebia heads xebiausa ("Xebia"), xebiadach ("Xebia DACH") and xebiacee ("Poland and Eastern
// Europe"), so it is a brand; "grupo" or "carrieres" head ids whose rows name someone else.
func brandHeads(rows []db.ListOpenJobsByCompanyBoardRow, named map[string]bool) map[string]bool {
	names := map[string]bool{}
	for _, r := range rows {
		if k := normalize.CompanyKey(r.BoardCompany); len(k) >= minBrand && named[r.Provider] {
			names[r.Provider+"\x00"+k] = true
		}
	}
	total, own := map[string]int{}, map[string]int{}
	foreign := map[string]map[string]bool{} // slugs a head's boards feed that do not start with it
	for _, r := range rows {
		core, ok := tokenCore(r.Board)
		if !ok || !named[r.Provider] {
			continue
		}
		key := normalize.CompanyKey(r.BoardCompany)
		for n := minBrand; n <= len(core); n++ {
			head := r.Provider + "\x00" + core[:n]
			if names[head] {
				total[head]++
				if strings.HasPrefix(key, core[:n]) {
					own[head]++
				} else if !strings.HasPrefix(normalize.FoldSlug(r.CompanySlug), core[:n]) {
					if foreign[head] == nil {
						foreign[head] = map[string]bool{}
					}
					foreign[head][r.CompanySlug] = true
				}
			}
		}
	}
	out := map[string]bool{}
	for h, n := range total {
		// A franchise or a generic word (clubpilates, grupo) heads boards of many other employers.
		out[h] = own[h] >= 1 && 2*own[h] >= n && len(foreign[h]) <= maxForeign
	}
	return out
}

// brandHead is the longest brand the id starts with, shorter than the id itself, or "".
func brandHead(provider, core string, brands map[string]bool) string {
	for n := len(core) - 1; n >= minBrand; n-- {
		if brands[provider+"\x00"+core[:n]] {
			return core[:n]
		}
	}
	return ""
}

// employerRows drops rows whose board name is a label, not an employer: one name over many
// slugs (a job-search slice "Bundesagentur für Arbeit", a staffing umbrella) says nothing
// about the company each posting lands on.
func employerRows(rows []db.ListOpenJobsByCompanyBoardRow) []db.ListOpenJobsByCompanyBoardRow {
	slugs := map[string]map[string]bool{}
	for _, r := range rows {
		k := r.Provider + "\x00" + r.BoardCompany
		if slugs[k] == nil {
			slugs[k] = map[string]bool{}
		}
		slugs[k][r.CompanySlug] = true
	}
	var out []db.ListOpenJobsByCompanyBoardRow
	for _, r := range rows {
		if len(slugs[r.Provider+"\x00"+r.BoardCompany]) <= labelSlugs {
			out = append(out, r)
		}
	}
	return out
}

// resemblesAfterFiller is whether a board name matches the website label once a marketing
// prefix is cut from the label: gopigment for Pigment (and gomotive for the Motive namesakes,
// which is why this lowers confidence instead of clearing the row).
func resemblesAfterFiller(label string, boards []db.ListOpenJobsByCompanyBoardRow) bool {
	for _, f := range []string{"go", "get", "try", "join", "use", "hello", "we", "my", "the", "by", "ask"} {
		rest, ok := strings.CutPrefix(label, f)
		if !ok || len(rest) < minKey {
			continue
		}
		for _, r := range boards {
			if matchesWords(rest, r.BoardCompany) {
				return true
			}
		}
	}
	return false
}

// ownSite is whether a website label, or the label past a filler prefix, resembles the board
// id: aboutamazon for amazon. An unrelated site the slug states does not make the id its own.
func ownSite(label, core string) bool {
	if label == "" {
		return false
	}
	for _, k := range keyForms(label) {
		if alike(k, core) {
			return true
		}
	}
	return false
}

// alikeAny is whether core shares a 4-letter head with another board feeding the slug.
func alikeAny(core string, boards []db.ListOpenJobsByCompanyBoardRow, self db.ListOpenJobsByCompanyBoardRow) bool {
	for _, s := range boards {
		if s == self {
			continue
		}
		if sc, ok := tokenCore(s.Board); ok && commonPrefix(core, sc) >= 4 {
			return true
		}
	}
	return false
}

func commonPrefix(a, b string) int {
	n := 0
	for n < len(a) && n < len(b) && a[n] == b[n] {
		n++
	}
	return n
}
