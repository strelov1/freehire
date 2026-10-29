package location

import "regexp"

// Polish location lines, measured 2026-10-29 over 15,488 open postings placed in Poland
// (nofluffjobs, whatjobs-pl, erecruiter, traffit, solidjobs, smartrecruiters,
// teamtailor, recruitee, eures). The country was already read on 99.6% of them, and a
// city on 97% of the lines that name more than "Poland". What the parser missed came in
// a few shapes:
//
//   - eures writes a street address with the Polish postcode before the town
//     ("Klimczaka 2, 02-797 Warszawa, Polska, Poland"), which postalPrefixRE now cuts;
//   - a metropolitan region instead of a town ("Trójmiasto", "Tricity", "Śląsk"), read
//     here as a metro and no city, as Delhi NCR is;
//   - a voivodeship alone ("Mazowieckie", "Lesser Poland Voivodeship"), which stated no
//     country, now in foreignSubdivisionNames;
//   - Łódź without one of its diacritics ("Lódz", "Łodź"), in cityOverrides.
//
// "Lublin, US", "Warsaw, IN" and "Opole" already resolved to their own countries.

// polishMetroRegions name a metropolitan region rather than a city. They state the
// country and the metro and no city: a Trójmiasto posting may sit in Gdańsk, Gdynia or
// Sopot, and a "Śląsk" one anywhere in the Upper Silesian conurbation the metro table
// labels Katowice. The bare English "Silesia" is absent: Czech Silesia shares it. The
// English "Tricity" is Chandigarh's too ("Tricity, Chandigarh, India"), so it is read
// only in a line that also places it in Poland (polishMetroRegion).
var polishMetroRegions = map[string]string{
	"trójmiasto":         "Gdańsk (PL)",
	"trojmiasto":         "Gdańsk (PL)",
	"trójmieście":        "Gdańsk (PL)",
	"śląsk":              "Katowice (PL)",
	"slask":              "Katowice (PL)",
	"górny śląsk":        "Katowice (PL)",
	"gorny slask":        "Katowice (PL)",
	"gzm":                "Katowice (PL)",
	"metropolia gzm":     "Katowice (PL)",
	"aglomeracja śląska": "Katowice (PL)",
	"aglomeracja slaska": "Katowice (PL)",
	"upper silesia":      "Katowice (PL)",
}

// polishTricityContext is a line that places a "Tricity" in Poland: the country, the
// voivodeship or one of the three towns.
var polishTricityContext = regexp.MustCompile(`(?:^|[^\p{L}])(?:poland|polska|pl|pomorskie|pomeranian|gdańsk|gdansk|gdynia|sopot)(?:$|[^\p{L}])`)

// polishMetroRegion returns the metro a Polish metropolitan-region token names, reading
// the English "Tricity" only in a line placed in Poland.
func polishMetroRegion(tok, line string) (string, bool) {
	if m, ok := polishMetroRegions[tok]; ok {
		return m, true
	}
	if (tok == "tricity" || tok == "tri-city") && polishTricityContext.MatchString(line) {
		return "Gdańsk (PL)", true
	}
	return "", false
}
