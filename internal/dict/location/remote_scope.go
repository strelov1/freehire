package location

import (
	"html"
	"regexp"
	"slices"
	"strings"

	"golang.org/x/text/language"
	"golang.org/x/text/language/display"

	"github.com/strelov1/freehire/internal/platform/stringset"
)

var (
	scopeBlockTag = regexp.MustCompile(`(?i)</?(?:p|div|li|br|h[1-6])\b[^>]*>`)
	scopeSpace    = regexp.MustCompile(`[\t\r \x{00a0}]+`)
	placeListJoin = strings.NewReplacer(" and ", ",", " or ", ",")
)

func descriptionScopeText(desc string) string {
	desc = scopeBlockTag.ReplaceAllString(desc, "\n")
	return scopeSpace.ReplaceAllString(strings.ToLower(html.UnescapeString(stripTags(desc))), " ")
}

func remoteScopeFromLabels(text string) (countries, regions []string) {
	for line := range strings.SplitSeq(text, "\n") {
		if strings.Contains(line, " - remote") {
			if c, r := remoteScopeLabel(line); len(r) > 0 {
				return c, r
			}
		}
	}
	return nil, nil
}

// remoteScopeLabel reads a complete location label, including lists such as
// "United States - Remote, EMEA - Remote". Treat the entire line as a label
// only when every entry is a known country or region followed by the remote marker.
func remoteScopeLabel(line string) (countries, regions []string) {
	fields, _, _ := strings.Cut(line, " please note that ")
	label, value, hasLabel := strings.Cut(fields, ":")
	if hasLabel {
		switch strings.Trim(label, " *") {
		case "headquarters":
			// Some boards put remote locations under this heading. Require
			// a list of explicitly remote entries before using it as scope.
			if !strings.Contains(value, ",") {
				return nil, nil
			}
		case "location", "locations":
		default:
			return nil, nil
		}
		fields = strings.TrimLeft(value, " *")
	}
	countrySet := map[string]struct{}{}
	regionSet := map[string]struct{}{}
	for _, field := range strings.Split(fields, ",") {
		place, ok := strings.CutSuffix(strings.TrimSpace(field), " - remote")
		if !ok {
			return nil, nil
		}
		rs := normalizeRegion(place)
		if len(rs) == 0 || slices.Contains(rs, globalRegion) {
			return nil, nil
		}
		if c := NormalizeCountry(place); c != "" {
			countrySet[c] = struct{}{}
		}
		for _, r := range rs {
			regionSet[r] = struct{}{}
		}
	}
	return stringset.Sorted(countrySet), stringset.Sorted(regionSet)
}

var countryExclusion = regexp.MustCompile(
	`\b(?:(?:unable to|cannot|can['’]t|(?:do|does|will) not|not able to) (?:accept|consider) (?:applications from )?|not open to |excluding )` +
		`(?:candidates|applicants) (?:based|located|residing) in\b|` +
		`\b(?:remote (?:worldwide|globally)|hiring worldwide) *,? (?:except in|excluding)\b`)

// ExcludedCountriesFromDescription reads explicit refusals tied to where
// applicants live, including exceptions to a worldwide scope. Country lists
// must resolve in full so a city followed by its country is never an exclusion
// of the whole country.
func ExcludedCountriesFromDescription(desc string) []string {
	lower := strings.ToLower(desc)
	applicantDenial := containsAny(lower, []string{"candidates", "applicants"}) &&
		containsAny(lower, []string{"unable", "not", "cannot", "can't", "can’t", "excluding"})
	worldwideException := containsAny(lower, []string{"worldwide", "globally"}) &&
		containsAny(lower, []string{"except", "excluding"})
	if !applicantDenial && !worldwideException {
		return nil
	}
	return excludedCountries(descriptionScopeText(lower))
}

func excludedCountries(text string) []string {
	var countries []string
	for _, match := range countryExclusion.FindAllStringIndex(text, -1) {
		start, end := match[0], match[1]
		if negatedSentence(sentenceAround(text[:start], start, start)) {
			continue
		}
		var list []string
		valid := true
		clause := strings.TrimSuffix(strings.TrimSpace(clauseAfterPhrase(text, end)), " for this role")
		for tok := range strings.SplitSeq(placeListJoin.Replace(clause), ",") {
			tok = strings.TrimPrefix(strings.TrimSpace(tok), "the ")
			if tok == "" {
				continue
			}
			if c := namedCountry(tok); c != "" {
				list = append(list, c)
			} else {
				valid = false
				break
			}
		}
		if valid {
			countries = append(countries, list...)
		}
	}
	slices.Sort(countries)
	return slices.Compact(countries)
}

// The location dictionary includes cities and subdivisions as aliases for
// their countries. A country exclusion needs the country itself.
func namedCountry(name string) string {
	c := NormalizeCountry(name)
	if c == "" {
		return ""
	}
	region, err := language.ParseRegion(strings.ToUpper(c))
	if err == nil && strings.EqualFold(name, display.English.Regions().Name(region)) {
		return c
	}
	switch name {
	case "us", "usa", "uk":
		return c
	}
	return ""
}

var worldwideArrangement = regexp.MustCompile(
	`\b(?:(?:this|the) (?:role|position|job) is (?:fully |100% )?remote (?:and open )?(?:worldwide|globally|from anywhere in the world)` +
		`|this is a (?:fully |100% )?remote (?:role|position|job) open (?:worldwide|to (?:candidates|applicants) anywhere in the world)` +
		`|we (?:accommodate|offer|support) (?:fully |100% )?remote work *,? with (?:teammates|employees|colleagues) living (?:around the globe|around the world|worldwide))\b`)

var worldwideConflict = regexp.MustCompile(
	`\b(?:must (?:live|reside|be|be located|be based) in|open to [^.!?\n]{1,55} (?:applicants|candidates) only|` +
		`(?:remote|hiring) only (?:in|within|from)|remote (?:in|within|from) [^.!?\n]{1,65} only|` +
		`(?:on[- ]?site|in[- ]office) (?:only|required|role|position)|fully (?:on[- ]?site|in[- ]office)|` +
		`must (?:be|work) (?:on[- ]?site|from (?:our|the) office)|work from (?:our|the) office|` +
		`days (?:per week |a week )?(?:in (?:(?:our|the) )?office|on[- ]?site)|hybrid (?:role|position|schedule|arrangement)|` +
		`except (?:in|for)|excluding)\b`)

// WorldwideRemoteFromDescription reports a worldwide work arrangement stated
// in the body. A distributed team or a temporary work-from-anywhere perk
// alone doesn't assert the scope of the role.
func WorldwideRemoteFromDescription(desc string) bool {
	lower := strings.ToLower(desc)
	if !containsAny(lower, []string{"worldwide", "globally", "the globe", "the world"}) || !strings.Contains(lower, "remote") {
		return false
	}
	text := descriptionScopeText(desc)
	for _, match := range worldwideArrangement.FindAllStringIndex(text, -1) {
		if negatedSentence(sentenceAround(text, match[0], match[1])) ||
			travelPerkQualified(text, match[0], match[1]) {
			continue
		}
		c, r := RegionScopeFromDescription(desc)
		return len(c)+len(r) == 0 && !worldwideConflict.MatchString(text) &&
			len(excludedCountries(text)) == 0 && !RemoteContradicted(desc) && !hasEligibilityRestriction(text)
	}
	return false
}

func hasEligibilityRestriction(text string) bool {
	return slices.ContainsFunc(eligibilityRules, func(rule eligibilityRule) bool { return ruleAsserted(text, rule) })
}

// IsRemoteOfficeList reports whether the location line lists only a standalone
// remote marker and recognizable city names. A country, region, or qualified
// remote marker can express hiring scope and must retain precedence.
func IsRemoteOfficeList(line string, cities []string) bool {
	if len(cities) < 2 {
		return false
	}
	remote := false
	for _, field := range strings.Split(separatorReplacer.Replace(strings.ToLower(line)), ",") {
		field = strings.TrimSpace(field)
		if field == "remote" {
			remote = true
			continue
		}
		if !slices.Contains(cities, cityDict[field].Name) || namedCountry(field) != "" {
			return false
		}
	}
	return remote
}

// WithoutExcludedGeography removes excluded countries and the regions supported
// only by those countries. A macro-region named separately in the location
// line remains a valid scope even when it contains an excluded country.
func WithoutExcludedGeography(countries, regions, excluded []string, line string) ([]string, []string) {
	if len(excluded) == 0 {
		return countries, regions
	}
	var removedRegions []string
	kept := slices.DeleteFunc(slices.Clone(countries), func(c string) bool {
		if !slices.Contains(excluded, c) {
			return false
		}
		removedRegions = append(removedRegions, countryToRegion[c])
		return true
	})
	regions = slices.DeleteFunc(slices.Clone(regions), func(r string) bool {
		if r == globalRegion {
			return true
		}
		if !slices.Contains(removedRegions, r) ||
			slices.ContainsFunc(kept, func(c string) bool { return countryToRegion[c] == r }) {
			return false
		}
		for tok := range strings.SplitSeq(separatorReplacer.Replace(strings.ToLower(line)), ",") {
			tok = strings.TrimSpace(tok)
			if nameToRegion[tok] == r || slices.Contains(regionAliases[tok], r) {
				return false
			}
		}
		return true
	})
	return kept, regions
}
