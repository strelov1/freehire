package location

import (
	"strings"
)

// ParseIn is Parse for a line whose countries the caller already knows from a structured
// signal — an adapter that writes the town alone and the country beside it (pandapelatam's
// "Palermo"). When the line itself states no country, those countries let its bare place
// names resolve the way a stated country would: a Buenos Aires barrio, a small or a
// contested town, and the metro of a city. They are never added to Countries or Regions —
// the caller owns that signal — and a line that states its own country ignores them.
func ParseIn(location string, known []string) Geo {
	p := newParseState(location, known)
	for i, raw := range p.toks {
		if tok, parenthetical, ok := p.cleanToken(raw); ok {
			p.readToken(i, tok, parenthetical)
		}
	}
	return p.finish()
}

// parseState is what ParseIn accumulates while it reads a line's comma-tokens in order.
type parseState struct {
	lower string
	// namesUS: a line that names the United States reads a colliding code beside it as the state.
	namesUS bool
	// Each comma-token's own location, when the line lists several ("Columbia, SC;
	// Curitiba, Brasil"): a location that names Brazil reads a Brazilian state code as the
	// state (location_br.go), one that ends in India an Indian state code between its city
	// and the country, and a province is told from a city within its own location only.
	segs []tokenLocation
	// The caller's countries; they stand in for the line's only where it states none (see
	// ParseIn). The Buenos Aires rule may read them inside the loop: a line it applies to
	// names nothing but Argentina.
	hint   map[string]struct{}
	hintAR bool
	toks   []string
	// A five-digit token is a postcode of the country the line names, not a US ZIP, when
	// the line names no part of the US (location_fr.go).
	foreignPostcodes bool
	// Metros a metropolitan-region token states without naming a city ("Delhi NCR").
	regionMetros map[string]struct{}

	countrySet map[string]struct{}
	regionSet  map[string]struct{}
	citySet    map[string]struct{}
	// Reused per-token scratch sets: a token's curated geography is resolved here first
	// so the city-name agreement check sees only THIS token's country (order-independent),
	// then merged into the result.
	tokCountry map[string]struct{}
	tokRegion  map[string]struct{}
	// prevTok is the previous comma-token, after the same stripping, and is the
	// disambiguating context resolveGeoToken needs for a colliding subdivision code
	// ("Tel Aviv, IL" vs "Chicago, IL").
	prevTok string
	// cityFallbackCountries collects the countries of the unambiguous long-tail cities
	// seen, held back until the whole string has been read and applied only if nothing
	// else stated one AND they agree. See applyCityFallback.
	cityFallbackCountries map[string]struct{}
	// Subdivision codes the line states, and the cities it names, are collected over the
	// whole line: the state usually FOLLOWS the city ("Roseville, CA").
	statedSubdivisions map[string]struct{}
	// An alias must agree with its own location's state, even when another location
	// on the line names the alias's state ("Harrison, NJ; Philadelphia, PA").
	segmentSubdivisions map[string]map[string]struct{}
	// pending holds the tokens that name a city only in a country the line states, resolved
	// once the whole line has been read (resolvePending).
	pending []pendingCity
	// aliasCities holds the long-tail cities the line names by an alias of a differently
	// named place, resolved after the loop against what the line states (addAliasCity).
	aliasCities []heldContested
	// placeToks counts the tokens left once work-mode words are cut. Whether one came before
	// matters because a province named after its capital is the province only after another
	// place (provinceAfterCity); the count, because the caller's countries name a pending or
	// contested town only in a line that names nothing else: in "Hermosillo, Sonora" the
	// second token is the state, not the small town GeoNames also calls Sonora.
	placeToks int
	// prevCountries are the countries the previous token stated through the curated
	// tables, withdrawn when a Brazilian state code after it places that town in Brazil.
	prevCountries []string
	// countryVotes counts the tokens that stated each country through the curated tables,
	// so a withdrawal never removes a country another token also stated.
	countryVotes map[string]int
}

func newParseState(location string, known []string) *parseState {
	lower := flagsToCodes(strings.ToLower(location))
	// A token that opened a parenthesis is marked, so it can be told apart from a place
	// in its own right once the separators are gone: see parenMark.
	s := separatorReplacer.Replace(strings.ReplaceAll(lower, "(", "("+parenMark))
	p := &parseState{
		lower:                 lower,
		namesUS:               lineNamesUS(s),
		segs:                  tokenLocations(lower, s),
		hint:                  map[string]struct{}{},
		toks:                  strings.Split(s, ","),
		regionMetros:          map[string]struct{}{},
		countrySet:            map[string]struct{}{},
		regionSet:             map[string]struct{}{},
		citySet:               map[string]struct{}{},
		tokCountry:            map[string]struct{}{},
		tokRegion:             map[string]struct{}{},
		cityFallbackCountries: map[string]struct{}{},
		statedSubdivisions:    map[string]struct{}{},
		segmentSubdivisions:   map[string]map[string]struct{}{},
		countryVotes:          map[string]int{},
	}
	for _, c := range known {
		p.hint[c] = struct{}{}
	}
	_, p.hintAR = p.hint["ar"]
	p.foreignPostcodes = foreignPostcodeLine(p.toks, p.namesUS, p.hint)
	return p
}

// stateCountry adds a country the line states, with its region.
func (p *parseState) stateCountry(c string) {
	p.countrySet[c] = struct{}{}
	p.regionSet[countryToRegion[c]] = struct{}{}
}

func (p *parseState) recordSubdivision(code, segment string) {
	p.statedSubdivisions[code] = struct{}{}
	if p.segmentSubdivisions[segment] == nil {
		p.segmentSubdivisions[segment] = map[string]struct{}{}
	}
	p.segmentSubdivisions[segment][code] = struct{}{}
}

// cleanToken strips a raw comma-token down to the place it names. ok is false for a token
// that names no place (empty, a bare postcode, only work-mode words).
func (p *parseState) cleanToken(tok string) (_ string, parenthetical, ok bool) {
	tok = strings.TrimSpace(tok)
	tok, parenthetical = strings.CutPrefix(tok, parenMark)
	tok = strings.TrimSpace(tok)
	if tok == "" {
		return "", false, false
	}
	tok = stripCityPrefix(tok)
	tok = stripAreaWords(tok)
	tok = stripPostalPrefix(tok)
	if p.foreignPostcodes {
		// The postcode alone, or after its town ("Saint-Denis 97400, La Réunion").
		if isUSZip(tok) {
			p.prevTok = tok
			return "", false, false
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
	return tok, parenthetical, tok != ""
}

// readToken reads the i-th comma-token, already cleaned: each stage either consumes it or
// passes it on to the next.
func (p *parseState) readToken(i int, tok string, parenthetical bool) {
	seg := p.segs[i]
	first := p.placeToks == 0
	p.placeToks++
	lastCountries := p.prevCountries
	p.prevCountries = nil
	tok, handled := p.recognizeRegionalPlace(tok, seg)
	if handled ||
		p.recognizeBrazilianState(i, tok, seg, first, lastCountries) ||
		p.recognizeSubdivisionCode(i, tok, seg, first, lastCountries) {
		return
	}
	tok, ok := p.cutClosingCodes(tok)
	if !ok || p.resolveToken(tok, seg, first, lastCountries, parenthetical) {
		return
	}
	p.resolveDashExport(tok)
}

// recognizeRegionalPlace reads the country-specific shapes that name a place on their own.
// It returns the token as normalized so far, and whether it consumed it.
func (p *parseState) recognizeRegionalPlace(tok string, seg tokenLocation) (string, bool) {
	// A Japanese address or prefecture (location_ja.go).
	if isJapaneseAddress(tok) {
		p.stateCountry("jp")
		p.prevTok = tok
		return tok, true
	}
	// India's export, district and office shapes become the city they name (india.go);
	// a metropolitan region ("Delhi NCR") is read before that, as the region it is.
	metro, isMetroRegion := indianMetroRegions[tok]
	if !isMetroRegion {
		tok = normalizeIndianToken(tok)
		metro, isMetroRegion = indianMetroRegions[tok]
	}
	if isMetroRegion {
		p.stateCountry("in")
		p.regionMetros[metro] = struct{}{}
		p.prevTok = tok
		return tok, true
	}
	if metro, ok := polishMetroRegion(tok, p.lower); ok {
		p.stateCountry("pl")
		p.regionMetros[metro] = struct{}{}
		p.prevTok = tok
		return tok, true
	}
	// The conurbation, a barrio and a town of Greater Buenos Aires, in a line that places
	// them nowhere else (location_ar.go).
	if seg.onlyBuenosAires && (seg.namesArgentina || p.hintAR) {
		if buenosAiresMetroRegions[tok] {
			p.stateCountry("ar")
			p.regionMetros[buenosAiresMetro] = struct{}{}
			p.prevTok = tok
			return tok, true
		}
		if name := buenosAiresPlaceIn(tok, seg.namesBuenosAires); name != "" {
			p.citySet[name] = struct{}{}
			p.regionMetros[buenosAiresMetro] = struct{}{}
			p.prevTok = tok
			return tok, true
		}
	}
	if indianStateBetween(tok, p.prevTok, seg.namesIndia) {
		p.stateCountry("in")
		p.prevTok = tok
		return tok, true
	}
	return tok, false
}

// recognizeBrazilianState consumes a Brazilian state code after a Brazilian town.
func (p *parseState) recognizeBrazilianState(i int, tok string, seg tokenLocation, first bool, lastCountries []string) bool {
	// A Brazilian state code after a Brazilian town is the state ("Curitiba, PR"), not
	// Puerto Rico; see brazilianStateCodes.
	// So is one opening a location that names Brazil next ("RS, Brasil").
	accepted := brazilianStateCodes[tok] && !seg.namesUS && (!first && brazilianStateAccepted(tok, p.prevTok, seg.namesBrazil) ||
		first && brazilianStateOpening(tok, p.toks, p.segs, i))
	if !accepted {
		return false
	}
	// "Colombo, PR": the town is Paraná's Colombo, so Sri Lanka goes; "Concordia,
	// SC" is Concórdia, not the El Paso district the city table named.
	for _, c := range lastCountries {
		if p.countryVotes[c]--; c != "br" && p.countryVotes[c] == 0 {
			withdrawCountry(c, p.countrySet, p.regionSet)
		}
	}
	if ce, ok := cityDict[p.prevTok]; ok && ce.Country != "br" {
		if name, ok := brazilianCityAliases()[p.prevTok]; ok && sameName(name, p.prevTok) {
			delete(p.citySet, ce.Name)
			p.citySet[name] = struct{}{}
		}
	}
	p.stateCountry("br")
	p.prevTok = tok
	return true
}

// recognizeSubdivisionCode consumes a two-letter code whose reading (country or which
// country's subdivision) its neighbours decide.
func (p *parseState) recognizeSubdivisionCode(i int, tok string, seg tokenLocation, first bool, lastCountries []string) bool {
	// A code that is both a US/CA subdivision and an ISO country code is the country
	// when it closes a location opening with a city of that country ("Toronto, ON, CA",
	// "Toronto, CA, CA", "Tel Aviv, IL, IL"); see countryCodeClosingCity.
	if countryCodeClosingCity(tok, p.toks, p.segs, i) {
		p.stateCountry(tok)
		p.prevTok = tok
		return true
	}
	if _, colliding := collidingSubdivisions[tok]; colliding && p.namesUS {
		p.stateCountry("us")
		// "Paris, IL, US" is the Illinois town, as "Paris, TX" is the Texan one — when
		// the US has a place of that name at all: "Jakarta, ID, United States" is no
		// Idaho town, and a list of codes ("CL, CO, MX, US") names no town.
		if !first && hasPlaceIn(p.prevTok, "us") {
			withdrawNamesake("us", p.prevTok, lastCountries, p.countryVotes, p.countrySet, p.regionSet)
		}
		if code, ok := statedSubdivisionCode(tok); ok {
			p.recordSubdivision(code, seg.text)
		}
		p.prevTok = tok
		return true
	}
	// "Zaragoza, AR, España", "Herisau, AR, CH": a code between a town and a different
	// country is that country's subdivision (Aragón, Appenzell), not a second country;
	// so is a US/CA/Australian code before the country closing the location ("Breda,
	// NB, Netherlands" is Noord-Brabant, not New Brunswick).
	if next := nextToken(p.toks, i); subdivisionBeforeCountry(tok, p.prevTok, next) ||
		foreignSubdivisionCode(tok, p.prevTok, closingToken(p.toks, p.segs, i)) {
		// The next country's own state or province ("Valencia, CA, US", "Estevan, SK,
		// Canada") is a stated subdivision, so a foreign namesake of the town goes.
		if c, ok := subdivisionToCountry[tok]; ok && c == countryOf(next) {
			if code, ok := statedSubdivisionCode(tok); ok {
				p.recordSubdivision(code, seg.text)
			}
			withdrawNamesake(c, p.prevTok, lastCountries, p.countryVotes, p.countrySet, p.regionSet)
		}
		p.prevTok = tok
		return true
	}
	return false
}

// cutClosingCodes cuts a postcode or a state/province code closing the token, stating its
// country. ok is false when nothing is left of the token.
func (p *parseState) cutClosingCodes(tok string) (_ string, ok bool) {
	// A UK postcode states the country on its own, the way a US ZIP does, and is cut
	// out so the place it follows still resolves ("london sw1a 2hb" -> "london").
	if rest, ok := stripUKPostcode(tok); ok {
		p.stateCountry("gb")
		if tok = rest; tok == "" {
			return "", false
		}
	}
	// An Australian state code closing the token ("perth wa", the form SEEK writes with
	// no comma) states the country and is cut off, so the place before it resolves as
	// a city of its own, with its city name and metro.
	if rest, ok := stripAustralianState(tok); ok {
		p.stateCountry("au")
		tok = rest
	}
	// A Dutch province code closing the token ("hengelo ov", "hoorn nh") states the
	// country and is cut off, so the town before it resolves (see stripDutchProvince).
	if rest, ok := stripDutchProvince(tok); ok {
		p.stateCountry("nl")
		tok = rest
	}
	// A Brazilian state code closing the token with no separator ("campinas sp").
	if rest, ok := stripBrazilianState(tok); ok && !p.namesUS {
		p.stateCountry("br")
		tok = rest
	}
	return tok, true
}

// resolveToken resolves the token through the curated tables and the city dictionary, and
// reports whether either placed it.
func (p *parseState) resolveToken(tok string, seg tokenLocation, first bool, lastCountries []string, parenthetical bool) bool {
	// Curated geography first, into the scratch sets (authoritative for country/region).
	clear(p.tokCountry)
	clear(p.tokRegion)
	// An Australian state is not a US/CA subdivision: recording the "wa" of "Perth, WA"
	// would key the metro lookup to Washington and find nothing in Perth.
	if _, ok := resolveSubdivision(tok, p.prevTok); ok && !isAustralianState(tok, p.prevTok) {
		if code, ok := statedSubdivisionCode(tok); ok {
			p.recordSubdivision(code, seg.text)
		}
	}
	resolved := resolveGeoToken(tok, p.prevTok, p.tokCountry, p.tokRegion)
	// A US state or Canadian province after a town the curated tables placed elsewhere
	// names that town's North American namesake (withdrawNamesake).
	if code, ok := resolveSubdivision(tok, p.prevTok); ok && !first {
		withdrawNamesake(code, p.prevTok, lastCountries, p.countryVotes, p.countrySet, p.regionSet)
	}
	// Recorded now, before the city and dash-export stages, so the next token's
	// collision check never looks at a stale or empty prevTok.
	p.prevTok = tok
	// City facet from the generated dictionary (cmd/gen-cities). cityDict supplies the
	// canonical display NAME only — never a country/region — so an ambiguous city name
	// ("Birmingham") can never *guess* a geography here; the country/region stay the
	// curated dictionaries' job (and the LLM's, at serve time). This keeps the parser's
	// "never guesses" contract while still populating the cities facet broadly.
	if ce, ok := cityDict[tok]; ok && !regionNotCity[tok] && !provinceAfterCity(tok, seg.text, first) {
		p.addCity(ce, tok, seg.text, resolved)
		resolved = true
	}
	if !resolved && !parenthetical {
		// Not a place either table states. It may still NAME one once the line has
		// stated a country (a small town beside its Land, "Köln-Niehl"); see pendingFor.
		if pc := pendingFor(tok); pc != nil {
			p.pending = append(p.pending, pc)
		}
	}
	for c := range p.tokCountry {
		p.countrySet[c] = struct{}{}
		p.countryVotes[c]++
		p.prevCountries = append(p.prevCountries, c)
	}
	for r := range p.tokRegion {
		p.regionSet[r] = struct{}{}
	}
	return resolved
}

// addCity adds the city cityDict names for tok; curated says whether the curated tables
// already placed tok.
func (p *parseState) addCity(ce cityEntry, tok, segment string, curated bool) {
	if curated {
		// The token is a curated place: emit the city name only when cityDict agrees
		// on its country, so a country/region token ("usa") never emits the unrelated
		// city buried in its GeoNames alternate names ("Yokkaichi").
		// An alias still waits for the rest of the line: "Calcutta, OH" is no Kolkata.
		if _, agree := p.tokCountry[ce.Country]; agree && !sameCityName(ce.Name, tok) {
			p.aliasCities = append(p.aliasCities, heldContested{ce: ce, tok: tok, segment: segment})
		} else if agree {
			p.citySet[ce.Name] = struct{}{}
		}
		return
	}
	// A long-tail city the curated maps do not place ("Recife", "Joinville"):
	// emit its name for the facet now, and remember its country as a
	// LAST-RESORT candidate. It is applied after the whole string is read
	// and only if nothing else stated a country — see applyCityFallback.
	//
	// A city named differently from the line — read through an alias — is held
	// until the line's country and state are known (addAliasCity): "León" is
	// León de los Aldama only in a line that places it in Mexico, and "Harrison,
	// NJ" is not Scranton, PA, whose old name it is.
	if !sameCityName(ce.Name, tok) {
		p.aliasCities = append(p.aliasCities, heldContested{ce: ce, tok: tok, segment: segment})
	} else {
		p.citySet[ce.Name] = struct{}{}
	}
	if !ce.Contested && ce.Country != "" {
		p.cityFallbackCountries[ce.Country] = struct{}{}
	}
}

// resolveDashExport reads a token nothing else placed as a dash-delimited export.
//
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
func (p *parseState) resolveDashExport(tok string) {
	parts := strings.Split(tok, "-")
	if len(parts) <= 1 || smallPlaceCandidates(tok) != nil {
		return
	}
	tailResolved := false
	for _, part := range parts[1:] {
		if resolveGeoName(strings.TrimSpace(part), p.countrySet, p.regionSet) {
			tailResolved = true
		}
	}
	lead := strings.TrimSpace(parts[0])
	if !tailResolved {
		// "TX-El Paso", "FL-Orlando": a US/CA subdivision code leading a city the
		// curated maps do not name but GeoNames places in the US or Canada. The
		// city is the confirmation, so "on-site", "co-op" and "hi-tech" — whose
		// tails are no city — still resolve nothing.
		if code, ok := subdivisionToCountry[lead]; ok && len(lead) == 2 &&
			isRecognizedUSCACity(strings.TrimSpace(strings.Join(parts[1:], "-"))) {
			p.countrySet[code] = struct{}{}
			if r, ok := countryToRegion[code]; ok {
				p.regionSet[r] = struct{}{}
			}
		}
	}
	if !resolveGeoName(lead, p.countrySet, p.regionSet) && tailResolved {
		// The dash-tail is the context that confirms a colliding lead code
		// ("il" in "IL-Cupertino") as a real US/CA subdivision, the same role
		// the preceding comma-token plays for "City, XX" — without it, tailResolved
		// having already added "us" via the tail's own city-name match would leave
		// the lead's country-code reading (Israel) alongside it, garbling the result.
		resolveGeoToken(lead, strings.Join(parts[1:], " "), p.countrySet, p.regionSet)
	}
}
