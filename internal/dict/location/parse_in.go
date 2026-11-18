package location

import (
	"strings"

	"github.com/strelov1/freehire/internal/platform/stringset"
)

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
	// Keep aliases separate until their metros are resolved: two segments can name
	// different places called Franklin, each with its own metro.
	aliasesBySegment := map[string]map[string]struct{}{}
	for _, h := range aliasCities {
		if aliasesBySegment[h.segment] == nil {
			aliasesBySegment[h.segment] = map[string]struct{}{}
		}
		addAliasCity(aliasesBySegment[h.segment], h.ce, h.tok, stated, segmentSubdivisions[h.segment])
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
	metroSet := map[string]struct{}{}
	if metroCountry != "" {
		for name := range citySet {
			if v, ok := metroFor(name, metroCountry, statedSubdivisions); ok {
				metroSet[v] = struct{}{}
			}
		}
	}
	for segment, names := range aliasesBySegment {
		for name := range names {
			citySet[name] = struct{}{}
			if metroCountry != "" {
				if v, ok := metroFor(name, metroCountry, segmentSubdivisions[segment]); ok {
					metroSet[v] = struct{}{}
				}
			}
		}
	}
	metros := stringset.Sorted(metroSet)
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
