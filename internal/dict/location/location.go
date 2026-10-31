// Package location derives a job's geography — ISO 3166-1 alpha-2 country codes
// and region codes — and a work-mode hint deterministically from the free-text
// ATS location string.
//
// It is a curated dictionary, not a geocoder: it resolves the high-frequency
// country names, ATS shorthands ("USA", "UK"), macro-region names ("Europe",
// "APAC"), and a few beacon cities that real ATS location fields use, and emits
// nothing for anything it cannot resolve (it never guesses). Region codes are
// drawn from the same controlled vocabulary the enrichment contract defines
// (vocab.RegionValues), and work modes from vocab.WorkModeValues, so the
// parser, the enrichment payload, and the search facet all speak one set of
// values.
package location

import (
	"regexp"
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/platform/stringset"
)

// Geo is the geography parsed from a location string: zero or more country codes
// and region codes, and an optional work-mode hint. Each field is empty when the
// location states nothing the parser can resolve.
type Geo struct {
	Countries []string
	Regions   []string
	Cities    []string // canonical city names for resolved beacon cities; empty when none
	WorkMode  string   // "", "remote", "hybrid", or "onsite" — only on an explicit marker
	Metros    []string // metro values ("Sacramento (US)"); dict-only, see metros.go
}

// parenMark prefixes a token that followed "(" in the location line. A parenthesis
// qualifies the place before it — "Schönebeck (Elbe)", "Halle (Saale)", "Kempten
// (Allgäu)" — and the qualifier is a river or a region that the small-places table may
// also list as a village ("Elbe" is one, in Lower Saxony). Such a token still resolves
// through the curated tables ("Berlin (Germany)"), but is never read as a small place.
const parenMark = "\x01"

// separatorReplacer normalizes every token separator to a comma in one pass so a
// single Split yields the geography tokens. The multi-character forms (" - ",
// " or ") and parentheses are included, so "Berlin (On-site)" -> "berlin",
// "on-site".
var separatorReplacer = strings.NewReplacer(
	";", ",", "/", ",", "|", ",", "(", ",", ")", ",", " - ", ",", " or ", ",",
)

// Parse maps a location string to its geography. Countries/regions are
// deduplicated and sorted; nil when nothing resolves. WorkMode is set only from
// an explicit marker, while a plain city/country yields geography with no
// WorkMode. A remote job that resolves NO geography (a bare "Remote", "WFH", …)
// states nothing about where it is open to, so it stays in the "region not
// specified" bucket (empty Regions/Countries) — never guessed into "global".
// An explicit open-anywhere word ("Anywhere", "Worldwide", "International", …)
// is a curated dictionary entry in its own right (see nameToRegion) and still
// resolves to the global region through the ordinary token loop below. A remote
// marker alongside a real place ("US Remote") keeps that place, as before.
func Parse(location string) Geo {
	return ParseIn(location, nil)
}

// ParseIn is Parse for a line whose countries the caller already knows from a structured
// signal — an adapter that writes the town alone and the country beside it (pandapelatam's
// "Palermo"). When the line itself states no country, those countries let its bare place
// names resolve the way a stated country would: a Buenos Aires barrio, a small or a
// contested town, and the metro of a city. They are never added to Countries or Regions —
// the caller owns that signal — and a line that states its own country ignores them.
func ParseIn(location string, known []string) Geo {
	lower := flagsToCodes(strings.ToLower(location))

	// A token that opened a parenthesis is marked, so it can be told apart from a place
	// in its own right once the separators are gone: see parenMark.
	s := separatorReplacer.Replace(strings.ReplaceAll(lower, "(", "("+parenMark))
	// A line that names the United States reads a colliding code beside it as the state.
	namesUS := lineNamesUS(s)
	// Each comma-token's own location, when the line lists several ("Columbia, SC;
	// Curitiba, Brasil"): a location that names Brazil reads a Brazilian state code as the
	// state (location_br.go), one that ends in India an Indian state code between its city
	// and the country, and a province is told from a city within its own location only.
	segs := tokenLocations(lower, s)
	// The caller's countries; they stand in for the line's only where it states none (see
	// ParseIn). The Buenos Aires rule may read them inside the loop: a line it applies to
	// names nothing but Argentina.
	hint := map[string]struct{}{}
	for _, c := range known {
		hint[c] = struct{}{}
	}
	_, hintAR := hint["ar"]
	toks := strings.Split(s, ",")
	// A five-digit token is a postcode of the country the line names, not a US ZIP, when
	// the line names no part of the US (location_fr.go).
	foreignPostcodes := foreignPostcodeLine(toks, namesUS, hint)
	// Metros a metropolitan-region token states without naming a city ("Delhi NCR").
	regionMetros := map[string]struct{}{}

	countrySet := map[string]struct{}{}
	regionSet := map[string]struct{}{}
	citySet := map[string]struct{}{}
	// Reused per-token scratch sets: a token's curated geography is resolved here first
	// so the city-name agreement check sees only THIS token's country (order-independent),
	// then merged into the result.
	tokCountry := map[string]struct{}{}
	tokRegion := map[string]struct{}{}
	// prevTok is the previous comma-token, after the same stripping, and is the
	// disambiguating context resolveGeoToken needs for a colliding subdivision code
	// ("Tel Aviv, IL" vs "Chicago, IL"). Updated at the end of the loop body.
	prevTok := ""
	// cityFallbackCountries collects the countries of the unambiguous long-tail cities
	// seen, held back until the whole string has been read and applied only if nothing
	// else stated one AND they agree. See its use below the loop.
	cityFallbackCountries := map[string]struct{}{}
	// Subdivision codes the line states, and the cities it names, are collected over the
	// whole line: the state usually FOLLOWS the city ("Roseville, CA").
	statedSubdivisions := map[string]struct{}{}
	// An alias must agree with its own location's state, even when another location
	// on the line names the alias's state ("Harrison, NJ; Philadelphia, PA").
	segmentSubdivisions := map[string]map[string]struct{}{}
	recordSubdivision := func(code, segment string) {
		statedSubdivisions[code] = struct{}{}
		if segmentSubdivisions[segment] == nil {
			segmentSubdivisions[segment] = map[string]struct{}{}
		}
		segmentSubdivisions[segment][code] = struct{}{}
	}
	// pending holds the tokens that name a city only in a country the line states, resolved
	// once the whole line has been read (resolvePending).
	var pending []pendingCity
	// aliasCities holds the long-tail cities the line names by an alias of a differently
	// named place, resolved after the loop against what the line states (addAliasCity).
	var aliasCities []heldContested
	// placeToks counts the tokens left once work-mode words are cut. Whether one came before
	// matters because a province named after its capital is the province only after another
	// place (provinceAfterCity); the count, because the caller's countries name a pending or
	// contested town only in a line that names nothing else: in "Hermosillo, Sonora" the
	// second token is the state, not the small town GeoNames also calls Sonora.
	placeToks := 0
	// prevCountries are the countries the previous token stated through the curated
	// tables, withdrawn when a Brazilian state code after it places that town in Brazil.
	var prevCountries []string
	// countryVotes counts the tokens that stated each country through the curated tables,
	// so a withdrawal never removes a country another token also stated.
	countryVotes := map[string]int{}
	for i, tok := range toks {
		seg := segs[i]
		tok = strings.TrimSpace(tok)
		tok, parenthetical := strings.CutPrefix(tok, parenMark)
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		tok = stripCityPrefix(tok)
		tok = stripAreaWords(tok)
		tok = stripPostalPrefix(tok)
		if foreignPostcodes {
			// The postcode alone, or after its town ("Saint-Denis 97400, La Réunion").
			if isUSZip(tok) {
				prevTok = tok
				continue
			}
			if f := strings.Fields(tok); len(f) > 1 && isUSZip(f[len(f)-1]) {
				tok = strings.Join(f[:len(f)-1], " ")
			}
		}
		// The city of an arrondissement, and a compound town written with spaces for its
		// hyphens ("Issy les Moulineaux"); see location_fr.go.
		tok = hyphenFolded(frenchPlaceToken(tok))
		// Strip embedded work-mode words so the place still resolves ("US Remote" ->
		// "us"); a token that is ONLY a work-mode marker ("Remote", "On-site") strips
		// to "" and is skipped — its work mode is detected separately from the whole
		// string. Skipping also keeps it out of the dash-split below.
		// "Remote within Canada" -> "canada"; see stripPlacePreposition.
		tok = stripWorkmodeWords(stripPlacePreposition(tok))
		if tok == "" {
			continue
		}
		first := placeToks == 0
		placeToks++
		lastCountries := prevCountries
		prevCountries = nil
		// A Japanese address or prefecture (location_ja.go).
		if isJapaneseAddress(tok) {
			countrySet["jp"] = struct{}{}
			regionSet[countryToRegion["jp"]] = struct{}{}
			prevTok = tok
			continue
		}
		// India's export, district and office shapes become the city they name (india.go);
		// a metropolitan region ("Delhi NCR") is read before that, as the region it is.
		metro, isMetroRegion := indianMetroRegions[tok]
		if !isMetroRegion {
			tok = normalizeIndianToken(tok)
			metro, isMetroRegion = indianMetroRegions[tok]
		}
		if isMetroRegion {
			countrySet["in"] = struct{}{}
			regionSet[countryToRegion["in"]] = struct{}{}
			regionMetros[metro] = struct{}{}
			prevTok = tok
			continue
		}
		if metro, ok := polishMetroRegion(tok, lower); ok {
			countrySet["pl"] = struct{}{}
			regionSet[countryToRegion["pl"]] = struct{}{}
			regionMetros[metro] = struct{}{}
			prevTok = tok
			continue
		}
		// The conurbation, a barrio and a town of Greater Buenos Aires, in a line that places
		// them nowhere else (location_ar.go).
		if seg.onlyBuenosAires && (seg.namesArgentina || hintAR) {
			if buenosAiresMetroRegions[tok] {
				countrySet["ar"] = struct{}{}
				regionSet[countryToRegion["ar"]] = struct{}{}
				regionMetros[buenosAiresMetro] = struct{}{}
				prevTok = tok
				continue
			}
			if name := buenosAiresPlaceIn(tok, seg.namesBuenosAires); name != "" {
				citySet[name] = struct{}{}
				regionMetros[buenosAiresMetro] = struct{}{}
				prevTok = tok
				continue
			}
		}
		if indianStateBetween(tok, prevTok, seg.namesIndia) {
			countrySet["in"] = struct{}{}
			regionSet[countryToRegion["in"]] = struct{}{}
			prevTok = tok
			continue
		}
		// A Brazilian state code after a Brazilian town is the state ("Curitiba, PR"), not
		// Puerto Rico; see brazilianStateCodes.
		// So is one opening a location that names Brazil next ("RS, Brasil").
		if brazilianStateCodes[tok] && !seg.namesUS && (!first && brazilianStateAccepted(tok, prevTok, seg.namesBrazil) ||
			first && brazilianStateOpening(tok, toks, segs, i)) {
			// "Colombo, PR": the town is Paraná's Colombo, so Sri Lanka goes; "Concordia,
			// SC" is Concórdia, not the El Paso district the city table named.
			for _, c := range lastCountries {
				if countryVotes[c]--; c != "br" && countryVotes[c] == 0 {
					withdrawCountry(c, countrySet, regionSet)
				}
			}
			if ce, ok := cityDict[prevTok]; ok && ce.Country != "br" {
				if name, ok := brazilianCityAliases()[prevTok]; ok && sameName(name, prevTok) {
					delete(citySet, ce.Name)
					citySet[name] = struct{}{}
				}
			}
			countrySet["br"] = struct{}{}
			regionSet[countryToRegion["br"]] = struct{}{}
			prevTok = tok
			continue
		}
		// A code that is both a US/CA subdivision and an ISO country code is the country
		// when it closes a location opening with a city of that country ("Toronto, ON, CA",
		// "Toronto, CA, CA", "Tel Aviv, IL, IL"); see countryCodeClosingCity.
		if countryCodeClosingCity(tok, toks, segs, i) {
			countrySet[tok] = struct{}{}
			regionSet[countryToRegion[tok]] = struct{}{}
			prevTok = tok
			continue
		}
		if _, colliding := collidingSubdivisions[tok]; colliding && namesUS {
			countrySet["us"] = struct{}{}
			regionSet[countryToRegion["us"]] = struct{}{}
			// "Paris, IL, US" is the Illinois town, as "Paris, TX" is the Texan one — when
			// the US has a place of that name at all: "Jakarta, ID, United States" is no
			// Idaho town, and a list of codes ("CL, CO, MX, US") names no town.
			if !first && hasPlaceIn(prevTok, "us") {
				withdrawNamesake("us", prevTok, lastCountries, countryVotes, countrySet, regionSet)
			}
			if code, ok := statedSubdivisionCode(tok); ok {
				recordSubdivision(code, seg.text)
			}
			prevTok = tok
			continue
		}
		// "Zaragoza, AR, España", "Herisau, AR, CH": a code between a town and a different
		// country is that country's subdivision (Aragón, Appenzell), not a second country;
		// so is a US/CA/Australian code before the country closing the location ("Breda,
		// NB, Netherlands" is Noord-Brabant, not New Brunswick).
		if next := nextToken(toks, i); subdivisionBeforeCountry(tok, prevTok, next) ||
			foreignSubdivisionCode(tok, prevTok, closingToken(toks, segs, i)) {
			// The next country's own state or province ("Valencia, CA, US", "Estevan, SK,
			// Canada") is a stated subdivision, so a foreign namesake of the town goes.
			if c, ok := subdivisionToCountry[tok]; ok && c == countryOf(next) {
				if code, ok := statedSubdivisionCode(tok); ok {
					recordSubdivision(code, seg.text)
				}
				withdrawNamesake(c, prevTok, lastCountries, countryVotes, countrySet, regionSet)
			}
			prevTok = tok
			continue
		}
		// A UK postcode states the country on its own, the way a US ZIP does, and is cut
		// out so the place it follows still resolves ("london sw1a 2hb" -> "london").
		if rest, ok := stripUKPostcode(tok); ok {
			countrySet["gb"] = struct{}{}
			regionSet[countryToRegion["gb"]] = struct{}{}
			if tok = rest; tok == "" {
				continue
			}
		}
		// An Australian state code closing the token ("perth wa", the form SEEK writes with
		// no comma) states the country and is cut off, so the place before it resolves as
		// a city of its own, with its city name and metro.
		if rest, ok := stripAustralianState(tok); ok {
			countrySet["au"] = struct{}{}
			regionSet[countryToRegion["au"]] = struct{}{}
			tok = rest
		}
		// A Dutch province code closing the token ("hengelo ov", "hoorn nh") states the
		// country and is cut off, so the town before it resolves (see stripDutchProvince).
		if rest, ok := stripDutchProvince(tok); ok {
			countrySet["nl"] = struct{}{}
			regionSet[countryToRegion["nl"]] = struct{}{}
			tok = rest
		}
		// A Brazilian state code closing the token with no separator ("campinas sp").
		if rest, ok := stripBrazilianState(tok); ok && !namesUS {
			countrySet["br"] = struct{}{}
			regionSet[countryToRegion["br"]] = struct{}{}
			tok = rest
		}
		// Curated geography first, into the scratch sets (authoritative for country/region).
		clear(tokCountry)
		clear(tokRegion)
		// An Australian state is not a US/CA subdivision: recording the "wa" of "Perth, WA"
		// would key the metro lookup to Washington and find nothing in Perth.
		if _, ok := resolveSubdivision(tok, prevTok); ok && !isAustralianState(tok, prevTok) {
			if code, ok := statedSubdivisionCode(tok); ok {
				recordSubdivision(code, seg.text)
			}
		}
		resolved := resolveGeoToken(tok, prevTok, tokCountry, tokRegion)
		// A US state or Canadian province after a town the curated tables placed elsewhere
		// names that town's North American namesake (withdrawNamesake).
		if code, ok := resolveSubdivision(tok, prevTok); ok && !first {
			withdrawNamesake(code, prevTok, lastCountries, countryVotes, countrySet, regionSet)
		}
		// Recorded now, before any of the branches below (including the early
		// "continue" on a resolved token) that would otherwise skip past the
		// end-of-loop assignment and leave the next token's collision check
		// looking at a stale or empty prevTok.
		prevTok = tok
		// City facet from the generated dictionary (cmd/gen-cities). cityDict supplies the
		// canonical display NAME only — never a country/region — so an ambiguous city name
		// ("Birmingham") can never *guess* a geography here; the country/region stay the
		// curated dictionaries' job (and the LLM's, at serve time). This keeps the parser's
		// "never guesses" contract while still populating the cities facet broadly.
		if ce, ok := cityDict[tok]; ok && !regionNotCity[tok] && !provinceAfterCity(tok, seg.text, first) {
			if resolved {
				// The token is a curated place: emit the city name only when cityDict agrees
				// on its country, so a country/region token ("usa") never emits the unrelated
				// city buried in its GeoNames alternate names ("Yokkaichi").
				// An alias still waits for the rest of the line: "Calcutta, OH" is no Kolkata.
				if _, agree := tokCountry[ce.Country]; agree && !sameCityName(ce.Name, tok) {
					aliasCities = append(aliasCities, heldContested{ce: ce, tok: tok, segment: seg.text})
				} else if agree {
					citySet[ce.Name] = struct{}{}
				}
			} else {
				// A long-tail city the curated maps do not place ("Recife", "Joinville"):
				// emit its name for the facet now, and remember its country as a
				// LAST-RESORT candidate. It is applied after the whole string is read
				// and only if nothing else stated a country — see cityFallback below.
				//
				// A city named differently from the line — read through an alias — is held
				// until the line's country and state are known (addAliasCity): "León" is
				// León de los Aldama only in a line that places it in Mexico, and "Harrison,
				// NJ" is not Scranton, PA, whose old name it is.
				if !sameCityName(ce.Name, tok) {
					aliasCities = append(aliasCities, heldContested{ce: ce, tok: tok, segment: seg.text})
				} else {
					citySet[ce.Name] = struct{}{}
				}
				resolved = true
				if !ce.Contested && ce.Country != "" {
					cityFallbackCountries[ce.Country] = struct{}{}
				}
			}
		}
		if !resolved && !parenthetical {
			// Not a place either table states. It may still NAME one once the line has
			// stated a country (a small town beside its Land, "Köln-Niehl"); see pendingFor.
			if p := pendingFor(tok); p != nil {
				pending = append(pending, p)
			}
		}
		for c := range tokCountry {
			countrySet[c] = struct{}{}
			countryVotes[c]++
			prevCountries = append(prevCountries, c)
		}
		for r := range tokRegion {
			regionSet[r] = struct{}{}
		}
		if resolved {
			continue
		}
		// Dash-delimited exports carry the geography either first ("United
		// States-Utah-Roy", "TX-Houston") or last ("Nisku-Alberta-Canada"). Every
		// non-leading segment is resolved by NAME only, so a 2-letter code buried in a
		// hyphenated city name ("stoke-on-trent" -> "on") cannot misfire while a
		// country/region word ("alberta", "canada", "china") still does. The leading
		// segment gets the same name-only treatment first; a bare 2-letter code there
		// ("tx" in "TX-Houston") is accepted only when a following segment also
		// resolved — i.e. a real geographic dash-export, not a hyphenated common word
		// ("in-house", "de-witt") whose first segment merely happens to be a code.
		// Tried only after the whole token failed, so "cluj-napoca"/"nur-sultan"
		// (dictionary keys) still win as a unit.
		// A small town's own hyphens are not an export's: "Meslay-du-Maine" names no US
		// state, "Porto-Vecchio" no Portugal.
		if segs := strings.Split(tok, "-"); len(segs) > 1 && smallPlaceCandidates(tok) == nil {
			tailResolved := false
			for _, seg := range segs[1:] {
				if resolveGeoName(strings.TrimSpace(seg), countrySet, regionSet) {
					tailResolved = true
				}
			}
			lead := strings.TrimSpace(segs[0])
			if !tailResolved {
				// "TX-El Paso", "FL-Orlando": a US/CA subdivision code leading a city the
				// curated maps do not name but GeoNames places in the US or Canada. The
				// city is the confirmation, so "on-site", "co-op" and "hi-tech" — whose
				// tails are no city — still resolve nothing.
				if code, ok := subdivisionToCountry[lead]; ok && len(lead) == 2 &&
					isRecognizedUSCACity(strings.TrimSpace(strings.Join(segs[1:], "-"))) {
					countrySet[code] = struct{}{}
					if r, ok := countryToRegion[code]; ok {
						regionSet[r] = struct{}{}
					}
				}
			}
			if !resolveGeoName(lead, countrySet, regionSet) && tailResolved {
				// The dash-tail is the context that confirms a colliding lead code
				// ("il" in "IL-Cupertino") as a real US/CA subdivision, the same role
				// the preceding comma-token plays for "City, XX" — without it, tailResolved
				// having already added "us" via the tail's own city-name match would leave
				// the lead's country-code reading (Israel) alongside it, garbling the result.
				resolveGeoToken(lead, strings.Join(segs[1:], " "), countrySet, regionSet)
			}
		}
	}

	// Before the long-tail fallback below, never after: a pending city waits for a country
	// the line STATED, and a country inferred from another city is not that.
	stated := countrySet
	if len(stated) == 0 && placeToks == 1 {
		stated = hint
	}
	resolvePending(pending, stated, citySet)
	for _, h := range aliasCities {
		addAliasCity(citySet, h.ce, h.tok, stated, segmentSubdivisions[h.segment])
	}

	// Nothing in the line stated a country, so the long-tail cities get to — but only
	// with one voice. Two guards, and the change is wrong without either:
	//
	// The country must be unstated. A city is the WEAKEST geographic statement in a
	// location line, so it never contributes alongside a stated country: "Anna,
	// Illinois, United States" names a town in Russia AND the state that actually
	// places the job, and "Crossroads - London" names a US locality beside the city
	// that matters.
	//
	// The cities must agree. Each is individually unambiguous, but a line naming two
	// of them ("Recife, Benidorm") is not, and picking the first would make the answer
	// depend on word order — a guess wearing a determinism costume.
	if len(countrySet) == 0 && len(cityFallbackCountries) == 1 {
		for country := range cityFallbackCountries {
			countrySet[country] = struct{}{}
			if r, ok := countryToRegion[country]; ok {
				regionSet[r] = struct{}{}
			}
		}
	}

	// A metro needs one stated country: a line naming two cannot place a city in either
	// without guessing.
	var metros []string
	metroCountry := ""
	if len(countrySet) == 1 {
		for c := range countrySet {
			metroCountry = c
		}
	} else if len(countrySet) == 0 && len(hint) == 1 {
		for c := range hint {
			metroCountry = c
		}
	}
	if metroCountry != "" && len(citySet) > 0 {
		metroSet := map[string]struct{}{}
		for name := range citySet {
			if v, ok := metroFor(name, metroCountry, statedSubdivisions); ok {
				metroSet[v] = struct{}{}
			}
		}
		metros = stringset.Sorted(metroSet)
	}
	// A metropolitan region the line named ("Delhi NCR", "GBA", a Buenos Aires barrio) adds
	// its metro, in that metro's country only.
	if metroCountry != "" && len(regionMetros) > 0 {
		suffix := " (" + strings.ToUpper(metroCountry) + ")"
		for m := range regionMetros {
			if !strings.HasSuffix(m, suffix) {
				delete(regionMetros, m)
			}
		}
		if len(regionMetros) > 0 {
			for _, m := range metros {
				regionMetros[m] = struct{}{}
			}
			metros = stringset.Sorted(regionMetros)
		}
	}

	countries := stringset.Sorted(countrySet)
	regions := WithoutShadowedGlobal(countries, stringset.Sorted(regionSet), lower)
	mode := detectWorkMode(cutIndiaWidePhrases(lower))

	return Geo{
		Countries: countries,
		Regions:   regions,
		Cities:    stringset.Sorted(citySet),
		WorkMode:  mode,
		Metros:    metros,
	}
}

// resolveGeoToken resolves one already-normalized token to a country and/or
// region, writing into the sets, and reports whether anything matched. Order: a
// country/city name, a macro-region name, a US/Canada subdivision, then a bare
// ISO 3166-1 alpha-2 country code (last, so a same-spelled subdivision wins) —
// except for a colliding subdivision code, where resolveSubdivision itself defers
// to prevTok and, unconfirmed, leaves the bare-code fallback to win instead.
// prevTok is the preceding comma-token (already stripped; "" when none), the
// context that confirms a colliding code as a real US/CA subdivision reading.
func resolveGeoToken(tok, prevTok string, countrySet, regionSet map[string]struct{}) bool {
	if tok == "" {
		return false
	}
	if code, ok := nameToCountry[tok]; ok {
		countrySet[code] = struct{}{}
		if r, ok := countryToRegion[code]; ok {
			regionSet[r] = struct{}{}
		}
		return true
	}
	if r, ok := nameToRegion[tok]; ok {
		regionSet[r] = struct{}{}
		return true
	}
	if isAustralianState(tok, prevTok) {
		countrySet["au"] = struct{}{}
		regionSet[countryToRegion["au"]] = struct{}{}
		return true
	}
	if code, ok := resolveSubdivision(tok, prevTok); ok {
		countrySet[code] = struct{}{}
		if r, ok := countryToRegion[code]; ok {
			regionSet[r] = struct{}{}
		}
		return true
	}
	if r, ok := countryToRegion[tok]; ok {
		countrySet[tok] = struct{}{}
		regionSet[r] = struct{}{}
		return true
	}
	return false
}

// resolveGeoName resolves a token by place NAME only — a country/city name, a
// macro-region name, or a full (len>2) US/Canada subdivision name. It deliberately
// skips bare 2-letter codes (subdivision or ISO), so it is safe to run on every
// non-leading dash segment of a hyphenated city ("stoke-on-trent") without "on"
// or "in" misfiring.
func resolveGeoName(tok string, countrySet, regionSet map[string]struct{}) bool {
	if code, ok := nameToCountry[tok]; ok {
		countrySet[code] = struct{}{}
		if r, ok := countryToRegion[code]; ok {
			regionSet[r] = struct{}{}
		}
		return true
	}
	if r, ok := nameToRegion[tok]; ok {
		regionSet[r] = struct{}{}
		return true
	}
	if len(tok) > 2 {
		if code, ok := subdivisionToCountry[tok]; ok {
			countrySet[code] = struct{}{}
			if r, ok := countryToRegion[code]; ok {
				regionSet[r] = struct{}{}
			}
			return true
		}
	}
	return false
}

// cityMarkerPrefixes are the "city" abbreviations that Cyrillic-writing sources
// prepend to a bare city name — Russian "г Москва" / "город Самара", Ukrainian
// "м. Львів" / "місто Київ". Stripped from a token before lookup so the city
// resolves; checked longest-first so "город " wins over "г ". A city whose name
// merely starts with "г" or "м" ("Грозный", "Мурманск") is untouched — every
// prefix ends in a separator the name doesn't.
var cityMarkerPrefixes = []string{"город ", "місто ", "г. ", "м. ", "г.", "м.", "г ", "м "}

// noiseTokenWords are dropped from a geography token so an embedded place still
// resolves: work-mode words ("US Remote" -> "us") and site suffixes ("San
// Francisco Office" / "... HQ" -> "san francisco"). Matched as whole
// space-separated words; the work mode is detected separately (detectWorkMode).
var noiseTokenWords = map[string]struct{}{
	"remote": {}, "hybrid": {}, "onsite": {}, "on-site": {},
	"office": {}, "hq": {}, "headquarters": {},
}

// siteWords are the noiseTokenWords that name a site rather than a work mode.
var siteWords = map[string]bool{"office": true, "hq": true, "headquarters": true}

// siteDescriptors say which KIND of site the word after them is, so they go with it: a
// school district's "Central Office" is its administration building, not Central,
// Louisiana, and a "Branch Office" is not Branch, Arkansas. Cutting the site word alone
// left the descriptor to be read as a town.
var siteDescriptors = map[string]bool{
	"central": true, "main": true, "head": true, "home": true, "district": true,
	"regional": true, "corporate": true, "branch": true, "field": true, "front": true,
	"back": true, "satellite": true, "administrative": true, "admin": true,
}

// trimSiteDescriptors drops the run of siteDescriptors closing words ("central regional"),
// stopping at a town whose own name ends in one: "Olive Branch Office" is Olive Branch.
func trimSiteDescriptors(words []string) []string {
	for n := len(words); n > 0 && siteDescriptors[words[n-1]]; n-- {
		if n > 1 && knownAsWritten(strings.Join(words[:n], " ")) {
			return words[:n]
		}
		words = words[:n-1]
	}
	return words
}

// stripWorkmodeWords drops any noise words from a token, returning the rest
// (possibly ""). "us remote" -> "us"; "remote" -> ""; "san francisco office" ->
// "san francisco"; "central office" -> "" (see siteDescriptors).
func stripWorkmodeWords(tok string) string {
	fields := strings.Fields(tok)
	kept := fields[:0]
	for _, f := range fields {
		if _, drop := noiseTokenWords[f]; drop {
			if siteWords[f] {
				kept = trimSiteDescriptors(kept)
			}
			continue
		}
		kept = append(kept, f)
	}
	return strings.Join(kept, " ")
}

// placePrepositions introduce the area a remote line is open to ("Remote within Canada",
// "Based in Germany", "Remote across LATAM").
var placePrepositions = map[string]bool{
	"in": true, "within": true, "from": true, "across": true, "throughout": true,
}

// placeLeadWords are the words that may stand before such a preposition — a work mode or a
// phrase introducing a place ("Anywhere in Europe", "Work from anywhere in Canada", "Fully
// remote, based in"). Any other word is a place of its own ("Kirchheim in Schwaben") and
// keeps the token whole.
var placeLeadWords = map[string]bool{
	"remote": true, "hybrid": true, "onsite": true, "on-site": true, "fully": true,
	"anywhere": true, "based": true, "located": true, "only": true, "work": true,
	"working": true, "from": true,
}

// stripPlacePreposition returns the country or region name after a lead-in preposition
// ("remote within canada" -> "canada", "anywhere in the us" -> "us"), or tok unchanged.
// Only a name the dictionary resolves is taken — the preposition is cut to read a stated
// place, never to guess one ("in zzyzx", "in the world" stay as they are). A bare "IN"
// opening the token is also Indiana's code ("IN Lebanon", "IN Angola"), so with nothing
// before it a name the US has a town of is left alone.
func stripPlacePreposition(tok string) string {
	fields := strings.Fields(tok)
	for k, f := range fields {
		if !placePrepositions[f] {
			continue
		}
		lead := fields[:k]
		if !allLeadWords(lead) {
			return tok
		}
		bareIn := len(lead) == 0 && f == "in"
		// The name as written first ("The Woodlands" is a Texan town), then without the
		// article ("the United States").
		name := stripWorkmodeWords(strings.Join(fields[k+1:], " "))
		if namesPlace(name, bareIn) {
			return name
		}
		if rest, ok := strings.CutPrefix(name, "the "); ok && namesPlace(rest, bareIn) {
			return rest
		}
	}
	return tok
}

// allLeadWords reports whether every word is a placeLeadWords entry.
func allLeadWords(words []string) bool {
	for _, w := range words {
		if !placeLeadWords[w] {
			return false
		}
	}
	return true
}

// namesPlace reports whether name is a country, city or region name. A US state or
// Canadian province is left out: resolveSubdivision already reads one closing a token
// ("Remote in California"), and cut loose it would also name a town ("Oregon"). A city of
// the main table counts under its own name only, never an alternate one ("market" is
// GeoNames' alias of Marquette). After a bare "IN", which is also Indiana's code, only a
// curated name the US has no town of counts: "IN Lebanon", "IN Munster" are Indiana towns.
func namesPlace(name string, bareIn bool) bool {
	if isSubdivisionName(name) {
		return false
	}
	if bareIn {
		_, country := nameToCountry[name]
		_, region := nameToRegion[name]
		return (country || region) && !hasPlaceIn(name, "us")
	}
	if _, ok := nameToCountry[name]; ok {
		return true
	}
	if ce, ok := cityDict[name]; ok && sameName(ce.Name, name) {
		return true
	}
	_, ok := nameToRegion[name]
	return ok
}

// areaPrefixes name the area AROUND a place ("Großraum Hannover", "Region Stuttgart",
// "Raum München", "Standort Erkheim", "Kanton Bern"); areaSuffixes do the same from the
// other side ("Stuttgart und Umgebung"). The place is what remains. French "Canton" is
// not here: it is a city in Massachusetts and Ohio ("Canton MA"). English "Greater" is
// deliberately not here: "Greater London" and "Greater Manchester" are subdivision names in
// their own right (foreignSubdivisionNames), and stripping would hand "Manchester" to the
// contested city table.
var (
	areaPrefixes = []string{"großraum ", "grossraum ", "metropolregion ", "region ", "raum ",
		"umkreis ", "standort ", "kanton ", "regio ", "omgeving ", "okolica ", "v okolici "}
	areaSuffixes = []string{" und umgebung", " und umland", " en omgeving", " e.o.",
		" in okolica", " z okolico", " z bližnjo okolico", " in bližnja okolica",
		" in širša okolica", " z širšo okolico"}
)

// stripAreaWords removes one areaPrefixes word and one areaSuffixes phrase from a token.
func stripAreaWords(tok string) string {
	for _, p := range areaPrefixes {
		if rest, ok := strings.CutPrefix(tok, p); ok {
			tok = strings.TrimSpace(rest)
			break
		}
	}
	for _, s := range areaSuffixes {
		if rest, ok := strings.CutSuffix(tok, s); ok {
			return strings.TrimSpace(rest)
		}
	}
	return tok
}

// postalPrefixRE matches a continental postal code written BEFORE its town: German and
// Austrian/Swiss style, optionally behind a country letter ("85462 Eitting", "CH-3400
// Burgdorf", "A-1010 Wien", "D-80331 München"), or Polish ("02-797 Warszawa"). The code states no country on its own —
// four and five digits are half of Europe's formats — so it is only cut away for the town
// to resolve. A US ZIP comes after its state and is untouched (isUSZip).
var postalPrefixRE = regexp.MustCompile(`^(?:[a-z]{1,2}-)?(?:\d{4,5}|\d{2}-\d{3})\s+(\pL.*)$`)

// dutchPostcodeRE matches the Dutch postcode before its town: four digits and two
// letters ("1012 AB Amsterdam", "3011AA Rotterdam"). Like postalPrefixRE it states no
// country; it is cut away so the town resolves.
var dutchPostcodeRE = regexp.MustCompile(`^\d{4}\s?[a-z]{2}\s+(\pL.*)$`)

// stripPostalPrefix returns the town after a leading postal code, or the token unchanged.
func stripPostalPrefix(tok string) string {
	// The Dutch form first: postalPrefixRE would take its letters for the town.
	if m := dutchPostcodeRE.FindStringSubmatch(tok); m != nil {
		return m[1]
	}
	if m := postalPrefixRE.FindStringSubmatch(tok); m != nil {
		return m[1]
	}
	return tok
}

// stripCityPrefix removes a leading Russian city marker from an already-lowercased,
// trimmed token, returning the bare city name (or the token unchanged).
func stripCityPrefix(tok string) string {
	for _, p := range cityMarkerPrefixes {
		if rest, ok := strings.CutPrefix(tok, p); ok {
			return strings.TrimSpace(rest)
		}
	}
	return tok
}

// resolveSubdivision resolves a US-state / Canadian-province token to its ISO
// country code, covering the "City, ST ZIP" and "City, Province" ATS formats. It
// tries, in order: a direct match ("tx", "texas", "ontario"); a trailing US ZIP
// preceded by a state code ("tx 76135" -> "tx"); a bare trailing code in a
// multi-word token ("austin tx"); and a standalone US ZIP ("94105") as a us
// signal. It returns ("", false) for anything it cannot resolve — it never
// guesses past the curated subdivision table.
//
// A direct or trailing-code match that lands on a collidingSubdivisions code (a
// code that also spells a curated country, e.g. "il") is accepted only when
// prevTok — the city portion, either the preceding comma-token for a direct match
// or the token's own leading words for a trailing-code match — names a recognized
// US/CA place (subdivisionAccepted); otherwise it is rejected here so the caller
// falls through to the bare-country-code reading ("Tel Aviv, IL" -> Israel, not
// Illinois). The ZIP-preceded branch is never gated: a ZIP code is a US signal on
// its own, so "il 60601" is unambiguous regardless of context.
func resolveSubdivision(tok, prevTok string) (string, bool) {
	if code, ok := subdivisionToCountry[tok]; ok {
		if !subdivisionAccepted(tok, prevTok) {
			return "", false
		}
		return code, true
	}
	fields := strings.Fields(tok)
	switch len(fields) {
	case 0:
		return "", false
	case 1:
		if isUSZip(fields[0]) {
			return "us", true
		}
		return "", false
	}
	last := fields[len(fields)-1]
	if isUSZip(last) {
		if code, ok := subdivisionToCountry[fields[len(fields)-2]]; ok {
			return code, true
		}
		return "us", true
	}
	if code, ok := subdivisionToCountry[last]; ok {
		leading := strings.Join(fields[:len(fields)-1], " ")
		if !subdivisionAccepted(last, leading) {
			return "", false
		}
		return code, true
	}
	return "", false
}

// australianStateCodes are the postal abbreviations of Australia's states and territories.
// The value says whether the code is Australia's alone: "nsw" or "qld" can only mean an
// Australian state, while "wa" is also Washington, "nt" the Northwest Territories, "sa"
// Saudi Arabia's ISO code and "act" an English word.
var australianStateCodes = map[string]bool{
	"nsw": true, "vic": true, "qld": true, "tas": true,
	"wa": false, "sa": false, "nt": false, "act": false,
}

// isAustralianState reports whether code, stated after place, is an Australian state:
// the comma form passes the preceding token as the place ("Perth, WA"), the no-comma form
// the token's own leading words (stripAustralianState). A code only Australia uses needs
// nothing more than a place before it; a shared one needs that place to be a city the dictionary
// already puts in Australia, so "Seattle, WA" stays Washington and "Yellowknife, NT" stays
// Canadian.
func isAustralianState(code, place string) bool {
	only, ok := australianStateCodes[code]
	if !ok || place == "" {
		return false
	}
	return only || isRecognizedCityOf(place, "au")
}

// stripAustralianState cuts an Australian state code off the end of a multi-word token
// ("perth wa" -> "perth", "bondi junction nsw" -> "bondi junction"), under the same rule
// isAustralianState applies with the leading words as the place.
func stripAustralianState(tok string) (string, bool) {
	fields := strings.Fields(tok)
	if len(fields) < 2 {
		return tok, false
	}
	place := strings.Join(fields[:len(fields)-1], " ")
	if !isAustralianState(fields[len(fields)-1], place) {
		return tok, false
	}
	return place, true
}

// subdivisionAccepted reports whether a matched subdivision code should win over
// its identically-spelled country-code reading. A code outside
// collidingSubdivisions is unambiguous and always accepted; a colliding one is
// accepted when cityTok is isRecognizedUSCACity — or, for a stateFirstSubdivisions
// code, when there is a city at all and the dictionary does not place it in the
// colliding country itself.
func subdivisionAccepted(code, cityTok string) bool {
	if _, ambiguous := collidingSubdivisions[code]; !ambiguous {
		return true
	}
	if isRecognizedUSCACity(cityTok) {
		return true
	}
	// A US town below the main city table's floor ("Atkins, AR", "Mountain Home, AR"):
	// the state, unless the colliding country has a place of that name too.
	if _, us := smallPlaceCandidates(cityTok)["us"]; us && !hasPlaceIn(cityTok, code) {
		return true
	}
	if _, stateFirst := stateFirstSubdivisions[code]; stateFirst && cityTok != "" {
		return !isRecognizedCityOf(cityTok, code)
	}
	return false
}

// isRecognizedCityOf reports whether tok names a place the parser already knows is
// in the given country — the same two lookups isRecognizedUSCACity makes. It is what
// keeps "Casablanca, MA" Moroccan once MA defaults to Massachusetts.
func isRecognizedCityOf(tok, country string) bool {
	if code, ok := nameToCountry[tok]; ok {
		return code == country
	}
	if ce, ok := cityDict[tok]; ok {
		return ce.Country == country
	}
	return false
}

// isRecognizedUSCACity reports whether tok names a place the parser already knows
// is in the US or Canada — checked directly against nameToCountry and cityDict
// (not through resolveGeoToken), so a long-tail GeoNames beacon like "Baton Rouge"
// counts just as much as a curated one like "Minneapolis". This is the signal that
// disambiguates a colliding subdivision code from its identically-spelled country
// code: "Baton Rouge, LA" stays Louisiana because "baton rouge" resolves here to
// us, while "Vientiane, LA" does not, so "la" falls back to the country code Laos.
func isRecognizedUSCACity(tok string) bool {
	if code, ok := nameToCountry[tok]; ok {
		return code == "us" || code == "ca"
	}
	if ce, ok := cityDict[tok]; ok {
		return ce.Country == "us" || ce.Country == "ca"
	}
	return false
}

// ukPostcodePattern matches a full UK postcode as a whole word: an outward code (area
// letters, district digit, optional sub-district) and an inward code (a digit and two
// letters, never C, I, K, M, O or V). Only the FULL form is read — an outward code alone
// ("sw1", "b2") is indistinguishable from an ordinary token.
var ukPostcodePattern = regexp.MustCompile(`\b([a-z]{1,2})\d[a-z\d]? \d[abd-hjlnp-uw-z]{2}\b`)

// ukPostcodeAreas are the postcode areas of the United Kingdom. GY, JE and IM (Guernsey,
// Jersey, the Isle of Man) are left out: they are Crown dependencies with their own ISO
// codes, not gb. BF (forces mail) names no place.
var ukPostcodeAreas = map[string]struct{}{
	"ab": {}, "al": {}, "b": {}, "ba": {}, "bb": {}, "bd": {}, "bh": {}, "bl": {}, "bn": {},
	"br": {}, "bs": {}, "bt": {}, "ca": {}, "cb": {}, "cf": {}, "ch": {}, "cm": {}, "co": {},
	"cr": {}, "ct": {}, "cv": {}, "cw": {}, "da": {}, "dd": {}, "de": {}, "dg": {}, "dh": {},
	"dl": {}, "dn": {}, "dt": {}, "dy": {}, "e": {}, "ec": {}, "eh": {}, "en": {}, "ex": {},
	"fk": {}, "fy": {}, "g": {}, "gl": {}, "gu": {}, "ha": {}, "hd": {}, "hg": {}, "hp": {},
	"hr": {}, "hs": {}, "hu": {}, "hx": {}, "ig": {}, "ip": {}, "iv": {}, "ka": {}, "kt": {},
	"kw": {}, "ky": {}, "l": {}, "la": {}, "ld": {}, "le": {}, "ll": {}, "ln": {}, "ls": {},
	"lu": {}, "m": {}, "me": {}, "mk": {}, "ml": {}, "n": {}, "ne": {}, "ng": {}, "nn": {},
	"np": {}, "nr": {}, "nw": {}, "ol": {}, "ox": {}, "pa": {}, "pe": {}, "ph": {}, "pl": {},
	"po": {}, "pr": {}, "rg": {}, "rh": {}, "rm": {}, "s": {}, "sa": {}, "se": {}, "sg": {},
	"sk": {}, "sl": {}, "sm": {}, "sn": {}, "so": {}, "sp": {}, "sr": {}, "ss": {}, "st": {},
	"sw": {}, "sy": {}, "ta": {}, "td": {}, "tf": {}, "tn": {}, "tq": {}, "tr": {}, "ts": {},
	"tw": {}, "ub": {}, "w": {}, "wa": {}, "wc": {}, "wd": {}, "wf": {}, "wn": {}, "wr": {},
	"ws": {}, "wv": {}, "yo": {}, "ze": {},
}

// stripUKPostcode reports whether tok carries a UK postcode and returns tok without it.
// An inward code that reads as an ordinal ("b2 2nd floor") is not a postcode: the letters
// are legal, so only the shape tells them apart, and precision wins over the rare real
// postcode ending in ST, ND, RD or TH.
func stripUKPostcode(tok string) (string, bool) {
	if !strings.ContainsAny(tok, "0123456789") {
		return tok, false
	}
	for _, m := range ukPostcodePattern.FindAllStringSubmatchIndex(tok, -1) {
		if _, ok := ukPostcodeAreas[tok[m[2]:m[3]]]; !ok {
			continue
		}
		switch tok[m[1]-2 : m[1]] {
		case "st", "nd", "rd", "th":
			continue
		}
		return strings.Trim(strings.Join(strings.Fields(tok[:m[0]]+" "+tok[m[1]:]), " "), "."), true
	}
	return tok, false
}

// isUSZip reports whether s is a US ZIP code: five digits, optionally followed by
// a "-" and the four-digit ZIP+4 extension ("76135" or "76135-1234").
func isUSZip(s string) bool {
	switch len(s) {
	case 5:
		return allDigits(s)
	case 10:
		return s[5] == '-' && allDigits(s[:5]) && allDigits(s[6:])
	default:
		return false
	}
}

func allDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// workModeMarkers maps a work mode to the substrings that signal it, checked in
// priority order: hybrid (most specific) beats a remote marker in the same
// string, and an explicit onsite marker is the last resort. A location with no
// marker yields "" — onsite is never assumed from a bare city.
var workModeMarkers = []struct {
	mode    string
	markers []string
}{
	// The Portuguese and Spanish words ("Remoto, Remoto, BR", "São Paulo - SP - Presencial")
	// are spelled with and without the accent; a location line is short, so the bare
	// stem cannot meet the "soporte remoto" sense a description has.
	{"hybrid", []string{"hybrid", "гибрид", "híbrid", "hibrid"}},
	{"remote", []string{"remote", "work from home", "wfh", "anywhere", "worldwide", "distributed", "удал", "remoto", "remota"}},
	// "WFO" is India's "work from office".
	{"onsite", []string{"on-site", "onsite", "on site", "in office", "in-office", "work from office", "wfo", "presencial"}},
}

// detectWorkMode scans the whole lowercased location for a work-mode marker,
// independent of tokenization so a marker embedded in a token ("Berlin
// (On-site)") is still found. A Recruitee remote label (remoteLabels) counts only as the
// whole line.
func detectWorkMode(lower string) string {
	if remoteLabels[strings.TrimSpace(lower)] {
		return "remote"
	}
	for _, wm := range workModeMarkers {
		for _, m := range wm.markers {
			if strings.Contains(lower, m) {
				return wm.mode
			}
		}
	}
	return ""
}

// statedSubdivisionCode returns the postal code a token that resolveSubdivision accepted
// states: the token itself ("ca"), its last field ("san francisco ca"), or the field
// before a trailing ZIP ("tx 76135").
func statedSubdivisionCode(tok string) (string, bool) {
	if code, ok := subdivisionCode[tok]; ok {
		return code, true
	}
	fields := strings.Fields(tok)
	if n := len(fields); n > 0 && isUSZip(fields[n-1]) {
		fields = fields[:n-1]
	}
	if len(fields) == 0 {
		return "", false
	}
	code, ok := subdivisionCode[fields[len(fields)-1]]
	return code, ok
}

// withdrawNamesake withdraws the countries the town before a US state or Canadian province
// stated (lastCountries), when the subdivision's country code is not among them: "Paris,
// TX", "London, ON", "Dublin, CA, US" name the North American namesake. A country another
// token also stated stays, and so does a town that is itself a province ("Saint-Laurent,
// Quebec, CA").
func withdrawNamesake(code, town string, lastCountries []string, votes map[string]int, countrySet, regionSet map[string]struct{}) {
	if len(lastCountries) == 0 || slices.Contains(lastCountries, code) || isSubdivisionName(town) {
		return
	}
	for _, c := range lastCountries {
		if votes[c]--; votes[c] == 0 {
			withdrawCountry(c, countrySet, regionSet)
		}
	}
}
