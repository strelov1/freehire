package industrytag

import "slices"

// displayNames is the canonical set and the text each value renders as. A slug
// absent from this map is not a canonical value — Canonicalize consults it to
// decide whether an unrecognized-but-slug-shaped input is a real canonical, and an
// invariant test asserts every alias target appears here.
var displayNames = map[string]string{
	"accounting":                     "Accounting",
	"adtech":                         "AdTech",
	"aerospace":                      "Aerospace",
	"aerospace-and-defense":          "Aerospace and Defense",
	"agriculture":                    "Agriculture",
	"ai":                             "AI",
	"automotive":                     "Automotive",
	"automotive-retail":              "Automotive Retail",
	"automotive-services":            "Automotive Services",
	"banking":                        "Banking",
	"biotech":                        "Biotech",
	"building-materials":             "Building Materials",
	"business-intelligence":          "Business Intelligence",
	"civil-engineering":              "Civil Engineering",
	"climate-tech":                   "Climate Tech",
	"cloud-computing":                "Cloud Computing",
	"construction":                   "Construction",
	"consumer-goods":                 "Consumer Goods",
	"consumer-services":              "Consumer Services",
	"crypto":                         "Crypto",
	"custom-software-development":    "Custom Software Development",
	"cybersecurity":                  "Cybersecurity",
	"data-analytics":                 "Data Analytics",
	"defense":                        "Defense",
	"developer-tools":                "Developer Tools",
	"digital-marketing":              "Digital Marketing",
	"digital-media":                  "Digital Media",
	"digital-transformation":         "Digital Transformation",
	"e-learning":                     "E-Learning",
	"ecommerce":                      "E-commerce",
	"education":                      "Education",
	"elderly-care":                   "Elderly Care",
	"energy":                         "Energy",
	"engineering-services":           "Engineering Services",
	"enterprise-software":            "Enterprise Software",
	"entertainment":                  "Entertainment",
	"environmental-services":         "Environmental Services",
	"facilities-services":            "Facilities Services",
	"financial-services":             "Financial Services",
	"fintech":                        "Fintech",
	"food-and-beverage":              "Food & Beverage",
	"gaming":                         "Gaming",
	"government":                     "Government",
	"health-and-wellness":            "Health and Wellness",
	"health-tech":                    "Health Tech",
	"healthcare":                     "Healthcare",
	"higher-education":               "Higher Education",
	"home-health-care":               "Home Health Care",
	"home-improvement":               "Home Improvement",
	"home-services":                  "Home Services",
	"hospitality":                    "Hospitality",
	"hospitals":                      "Hospitals",
	"hr-tech":                        "HR Tech",
	"individual-and-family-services": "Individual and Family Services",
	"industrial-automation":          "Industrial Automation",
	"industrial-machinery":           "Industrial Machinery",
	"industrial-manufacturing":       "Industrial Manufacturing",
	"insurance":                      "Insurance",
	"iot":                            "IoT",
	"it-services":                    "IT Services",
	"k-12-education":                 "K-12 Education",
	"legal-services":                 "Legal Services",
	"logistics":                      "Logistics",
	"machine-learning":               "Machine Learning",
	"management-consulting":          "Management Consulting",
	"manufacturing":                  "Manufacturing",
	"marketing-services":             "Marketing Services",
	"medical-devices":                "Medical Devices",
	"medical-services":               "Medical Services",
	"mental-health":                  "Mental Health",
	"mobile-apps":                    "Mobile Apps",
	"nonprofit":                      "Nonprofit",
	"oil-and-gas":                    "Oil and Gas",
	"pharmaceutical":                 "Pharmaceutical",
	"professional-services":          "Professional Services",
	"property-management":            "Property Management",
	"proptech":                       "PropTech",
	"public-safety":                  "Public Safety",
	"real-estate":                    "Real Estate",
	"recruitment":                    "Recruitment",
	"religious-organizations":        "Religious Organizations",
	"renewable-energy":               "Renewable Energy",
	"retail":                         "Retail",
	"retail-technology":              "Retail Technology",
	"risk-management":                "Risk Management",
	"saas":                           "SaaS",
	"senior-care":                    "Senior Care",
	"social-services":                "Social Services",
	"software-development":           "Software Development",
	"sustainability":                 "Sustainability",
	"technology-consulting":          "Technology Consulting",
	"telecommunications":             "Telecommunications",
	"telehealth":                     "Telehealth",
	"transportation":                 "Transportation",
	"transportation-and-logistics":   "Transportation and Logistics",
	"utilities":                      "Utilities",
	"wealth-management":              "Wealth Management",
	"web-development":                "Web Development",
	"wholesale":                      "Wholesale",
	"workforce-development":          "Workforce Development",
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
