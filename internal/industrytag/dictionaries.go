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
	"artificial-intelligence":         "ai",
	"behavioral-health":               "mental-health",
	"biotechnology":                   "biotech",
	"blockchain":                      "crypto",
	"clean-energy":                    "renewable-energy",
	"clean-tech":                      "climate-tech",
	"cryptocurrency":                  "crypto",
	"data-management-and-analytics":   "data-analytics",
	"defi":                            "crypto",
	"digital-advertising":             "adtech",
	"digital-health":                  "health-tech",
	"e-commerce":                      "ecommerce",
	"edtech":                          "e-learning",
	"finance":                         "financial-services",
	"food-and-beverages":              "food-and-beverage",
	"game-development":                "gaming",
	"generative-ai":                   "ai",
	"government-administration":       "government",
	"government-services":             "government",
	"healthcare-it":                   "health-tech",
	"healthcare-services":             "healthcare",
	"healthcare-technology":           "health-tech",
	"hr-technology":                   "hr-tech",
	"human-resources":                 "hr-tech",
	"human-resources-technology":      "hr-tech",
	"information-technology":          "it-services",
	"insurtech":                       "insurance",
	"it":                              "it-services",
	"it-consulting":                   "technology-consulting",
	"life-sciences":                   "biotech",
	"logistics-and-supply-chain":      "logistics",
	"martech":                         "adtech",
	"mental-health-care":              "mental-health",
	"mental-health-services":          "mental-health",
	"non-profit":                      "nonprofit",
	"non-profit-organizations":        "nonprofit",
	"nonprofit-organizations":         "nonprofit",
	"online-education":                "e-learning",
	"online-learning":                 "e-learning",
	"pharmaceuticals":                 "pharmaceutical",
	"primary-and-secondary-education": "k-12-education",
	"public-administration":           "government",
	"restaurants":                     "food-and-beverage",
	"senior-living":                   "senior-care",
	"social-assistance":               "social-services",
	"software":                        "software-development",
	"solar-energy":                    "renewable-energy",
	"staffing-and-recruiting":         "recruitment",
	"supply-chain-management":         "logistics",
	"talent-acquisition":              "recruitment",
	"telemedicine":                    "telehealth",
	"web3":                            "crypto",
	"workforce-management":            "hr-tech",
}
