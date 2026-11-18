// Package normalize derives normalized keys from raw source data. It is the
// home for the pipeline's name-to-slug normalization.
package normalize

import (
	"strings"
	"unicode"

	"github.com/mozillazg/go-unidecode"
)

// Slug turns a name into its natural key, usable verbatim in a URL path: transliterated to
// ASCII, lowercased, each run of non-alphanumeric characters collapsed to one hyphen, ends
// trimmed. Non-Latin names are romanized ("Яндекс" → "iandeks", "小红书" → "xiao-hong-shu"),
// because a Cyrillic/CJK slug breaks routing. An empty or untransliterable name yields an
// empty slug, which the write path treats as "no company".
//
// It does not strip legal suffixes (LLC, Inc): that is [CompanySlug]'s job, and the two are
// different keys. Most non-Latin forms (ООО, 有限公司) are stripped by neither; the Japanese
// forms (株式会社) are matched literally by CompanySlug (see japaneseForms). Use CompanySlug
// whenever the value keys a COMPANY, where "RingCentral" and "RingCentral, Inc." must not be
// two answers.
func Slug(name string) string {
	name = unidecode.Unidecode(name)
	var b strings.Builder
	prevHyphen := false
	for _, r := range name {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(unicode.ToLower(r))
			prevHyphen = false
		case b.Len() > 0 && !prevHyphen:
			b.WriteByte('-')
			prevHyphen = true
		}
	}
	return strings.TrimRight(b.String(), "-")
}
