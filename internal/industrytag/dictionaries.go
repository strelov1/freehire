package industrytag

// aliases maps a normalized label (see normalize) to its canonical slug.
//
// Keys are stored in normal form, which an invariant test enforces — a key the
// normalizer can never produce would silently never match, which is the exact
// failure mode this dictionary exists to prevent.
//
// Entries fall into two kinds. Most are spelling variants that normalize already
// folds, kept here only because their source writes them that way. The valuable
// ones are the semantic merges: different words for one industry, which no amount
// of string normalization would ever join.
var aliases = map[string]string{
	"ai":                      "ai",
	"artificial-intelligence": "ai",
	"financial-services":      "financial-services",
	"food-and-beverage":       "food-and-beverage",
	"medical-devices":         "medical-devices",
	"retail":                  "retail",
}
