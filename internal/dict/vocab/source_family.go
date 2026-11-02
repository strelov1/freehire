package vocab

import "strings"

// SourceFamilies folds adapters that are one brand's regional splits into the single
// name a person picks in the source filter. WhatJobs runs one adapter per country
// market (`whatjobs`, `whatjobs-br`, …), each its own `source` value because each is a
// separate publisher account; to a visitor they are one site. Choosing the family name
// selects every member, and the source facet reports the members as one bucket.
//
// The members are listed, not matched by prefix, because the search index has no
// prefix filter. internal/ingest/sources checks the list against its registry, so a
// market added there without being added here fails a test instead of staying a
// separate entry in the picker.
var SourceFamilies = map[string][]string{
	"whatjobs": {
		"whatjobs",
		"whatjobs-ae",
		"whatjobs-ar",
		"whatjobs-at",
		"whatjobs-au",
		"whatjobs-be",
		"whatjobs-bh",
		"whatjobs-br",
		"whatjobs-ca",
		"whatjobs-ch",
		"whatjobs-cl",
		"whatjobs-co",
		"whatjobs-cr",
		"whatjobs-de",
		"whatjobs-dk",
		"whatjobs-do",
		"whatjobs-dz",
		"whatjobs-ec",
		"whatjobs-eg",
		"whatjobs-es",
		"whatjobs-fi",
		"whatjobs-fr",
		"whatjobs-gr",
		"whatjobs-gt",
		"whatjobs-hk",
		"whatjobs-hu",
		"whatjobs-id",
		"whatjobs-ie",
		"whatjobs-in",
		"whatjobs-it",
		"whatjobs-ke",
		"whatjobs-kw",
		"whatjobs-lu",
		"whatjobs-ma",
		"whatjobs-mx",
		"whatjobs-my",
		"whatjobs-ng",
		"whatjobs-nl",
		"whatjobs-no",
		"whatjobs-nz",
		"whatjobs-om",
		"whatjobs-pa",
		"whatjobs-pe",
		"whatjobs-ph",
		"whatjobs-pk",
		"whatjobs-pl",
		"whatjobs-pr",
		"whatjobs-pt",
		"whatjobs-py",
		"whatjobs-qa",
		"whatjobs-ru",
		"whatjobs-sa",
		"whatjobs-se",
		"whatjobs-sg",
		"whatjobs-sv",
		"whatjobs-th",
		"whatjobs-tr",
		"whatjobs-tz",
		"whatjobs-ug",
		"whatjobs-uk",
		"whatjobs-uy",
		"whatjobs-ve",
		"whatjobs-vn",
		"whatjobs-za",
	},
}

// SourceFamilyOf returns the family a source belongs to, or the source itself when it
// belongs to none.
func SourceFamilyOf(source string) string {
	for family, members := range SourceFamilies {
		if source == family {
			return family
		}
		for _, m := range members {
			if strings.EqualFold(m, source) {
				return family
			}
		}
	}
	return source
}
