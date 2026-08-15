package industrytag

// aliases maps a normalized label (see normalize) to a canonical slug it is NOT
// equal to. Canonical values are not listed here: Canonicalize already accepts one
// by looking it up in displayNames, so a self-mapping would be dead weight that
// doubles the dictionary for nothing.
//
// Keys are stored in normal form, which an invariant test enforces — a key the
// normalizer can never produce would silently never match, which is the exact
// failure mode this dictionary exists to prevent.
//
// What earns an entry here is a semantic merge: different words for one industry,
// which no amount of string normalization would ever join. Pure spelling variants
// ("Financial Services" against "Financial-Services") need no entry at all —
// normalize folds them before the lookup.
var aliases = map[string]string{
	"artificial-intelligence": "ai",
}
