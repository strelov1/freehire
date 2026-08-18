// Package normalize derives normalized keys from raw source data. It is the
// home for the pipeline's name-to-slug normalization.
package normalize

import (
	"strings"
	"unicode"

	"github.com/mozillazg/go-unidecode"
)

// Slug turns a name into its natural key, usable verbatim in a URL path:
// transliterated to ASCII, lowercased, with each run of non-alphanumeric
// characters collapsed to a single hyphen and leading/trailing hyphens trimmed.
// Non-Latin names are romanized (e.g. "Яндекс" → "iandeks", "小红书" →
// "xiao-hong-shu"), so the resulting slug is always ASCII — public_slug and
// company_slug are URL path segments, and a Cyrillic/CJK slug breaks routing.
// An empty or untransliterable name yields an empty slug, which the write path
// treats as "no company".
//
// It deliberately does not strip legal suffixes (LLC, Inc) — that is [CompanySlug]'s
// job, and the two are different keys. Non-Latin forms (ООО, 有限公司) are stripped by
// neither: the form test reduces a word to its ASCII letters, so a Cyrillic or CJK
// suffix simply is not one. Slug is faithful to the name it
// was given, which is what a URL path segment and a job's public slug need; CompanySlug
// answers "which employer is this", where "RingCentral" and "RingCentral, Inc." must not
// be two answers. Reach for CompanySlug whenever the value keys a COMPANY.
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
