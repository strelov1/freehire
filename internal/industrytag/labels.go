package industrytag

import "slices"

// displayNames is the canonical set and the text each value renders as. A slug
// absent from this map is not a canonical value — Canonicalize consults it to
// decide whether an unrecognized-but-slug-shaped input is a real canonical, and an
// invariant test asserts every alias target appears here.
var displayNames = map[string]string{
	"accounting":              "Accounting",
	"adtech":                  "AdTech",
	"aerospace":               "Aerospace",
	"agriculture":             "Agriculture",
	"ai":                      "AI",
	"architecture":            "Architecture",
	"automotive":              "Automotive",
	"banking":                 "Banking",
	"biotech":                 "Biotech",
	"building-materials":      "Building Materials",
	"climate-tech":            "Climate Tech",
	"construction":            "Construction",
	"consumer-goods":          "Consumer Goods",
	"crypto":                  "Crypto",
	"cybersecurity":           "Cybersecurity",
	"data-analytics":          "Data Analytics",
	"defense":                 "Defense",
	"developer-tools":         "Developer Tools",
	"digital-marketing":       "Digital Marketing",
	"ecommerce":               "E-commerce",
	"edtech":                  "EdTech",
	"education":               "Education",
	"energy":                  "Energy",
	"engineering-services":    "Engineering Services",
	"enterprise-software":     "Enterprise Software",
	"entertainment":           "Entertainment",
	"environmental-services":  "Environmental Services",
	"facilities-services":     "Facilities Services",
	"financial-services":      "Financial Services",
	"fintech":                 "Fintech",
	"food-and-beverage":       "Food & Beverage",
	"gambling":                "Gambling",
	"gaming":                  "Gaming",
	"government":              "Government",
	"health-tech":             "Health Tech",
	"healthcare":              "Healthcare",
	"higher-education":        "Higher Education",
	"home-services":           "Home Services",
	"hospitality":             "Hospitality",
	"hr-tech":                 "HR Tech",
	"industrial-automation":   "Industrial Automation",
	"insurance":               "Insurance",
	"iot":                     "IoT",
	"it-services":             "IT Services",
	"k-12-education":          "K-12 Education",
	"legal-services":          "Legal Services",
	"logistics":               "Logistics",
	"manufacturing":           "Manufacturing",
	"medical-devices":         "Medical Devices",
	"mental-health":           "Mental Health",
	"nonprofit":               "Nonprofit",
	"oil-and-gas":             "Oil & Gas",
	"pharmaceuticals":         "Pharmaceuticals",
	"property-management":     "Property Management",
	"proptech":                "PropTech",
	"public-safety":           "Public Safety",
	"real-estate":             "Real Estate",
	"recruitment":             "Recruitment",
	"regtech":                 "RegTech",
	"religious-organizations": "Religious Organizations",
	"renewable-energy":        "Renewable Energy",
	"retail":                  "Retail",
	"robotics":                "Robotics",
	"senior-care":             "Senior Care",
	"social-services":         "Social Services",
	"software-development":    "Software Development",
	"technology-consulting":   "Technology Consulting",
	"telecommunications":      "Telecommunications",
	"telehealth":              "Telehealth",
	"transportation":          "Transportation",
	"travel":                  "Travel",
	"utilities":               "Utilities",
	"wealth-management":       "Wealth Management",
	"wholesale":               "Wholesale",
}

// Label returns the display text for a canonical slug. An unknown slug returns
// itself rather than empty, so a value stored before a dictionary edit still
// renders as something readable instead of vanishing from the UI.
func Label(canonical string) string {
	if name, ok := displayNames[canonical]; ok {
		return name
	}
	return canonical
}

// Labels returns a copy of the canonical-to-display-text map. cmd/gen-contracts
// emits it for the SPA, mirroring skilltag.Labels, so the filter's option text is
// generated from this dictionary rather than retyped in TypeScript.
//
// The copy matters: the caller must not be able to mutate the package's table.
func Labels() map[string]string {
	out := make(map[string]string, len(displayNames))
	for canonical, name := range displayNames {
		out[canonical] = name
	}
	return out
}

// Canonicals returns every canonical slug, sorted — the source for the facet's
// option list, so the UI cannot drift from the dictionary.
func Canonicals() []string {
	out := make([]string, 0, len(displayNames))
	for canonical := range displayNames {
		out = append(out, canonical)
	}
	slices.Sort(out)
	return out
}
