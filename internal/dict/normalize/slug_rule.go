package normalize

import "strings"

// SlugPredatesRule reports whether a stored company slug was minted from THIS name by an older
// company rule: it is the faithful [Slug] of the name, yet [CompanySlug] now derives a different
// one — a legal form the rule has learned since the slug was minted (the Japanese 株式会社,
// stored romanised as "zhu-shi-hui-she-…" and now keyed as a `kk` mark).
//
// The Slug test is what keeps this to the rule's own change. A slug that is NOT the plain slug
// of the latest name drifted for another reason — the source renamed the employer — and
// re-keying that is a judgement about the rename, not about a form.
//
// The two are compared FOLDED, because the rule and Slug disagree about word breaks in ways
// that say nothing about who the employer is: the rule splits "ディー・エヌ・エー" at the middle
// dot and drops the apostrophe of "Linc'well", where Slug glues the first and splits the
// second. Compared exactly, 472 Japanese companies minted under the old rule (2026-10-27) read
// as renamed and were left on their pinyin slug.
//
// A slug minted by an OLDER rule that already stripped PART of the form is the same case, and
// is not the plain slug: "Acme Pvt Ltd" was keyed `acme-pvt` while the rule knew "Ltd" and not
// "Pvt" (1,936 companies, 2026-10-27). So a stored slug also qualifies when it lies BETWEEN
// the two — the current key a prefix of it, it a prefix of the plain slug — because then
// everything separating it from the key is a trailing word the rule now strips as a form. A
// renamed employer fails that test from either side: "Acme" renamed "Acme Holdings Ltd" is
// shorter than its new key, and a different name is no prefix of the plain slug at all.
func SlugPredatesRule(stored, name string) bool {
	derived := CompanySlug(name)
	if derived == stored {
		return false
	}
	plain := Slug(name)
	if derived == plain {
		// The rule makes nothing of this name that Slug does not, so it cannot be what moved
		// the stored slug: one that differs (even only in word breaks, "ac-me" for "Acme")
		// came from another spelling of the name — a judgement, not the rule.
		return false
	}
	folded := FoldSlug(stored)
	if folded == FoldSlug(plain) {
		return true
	}
	return strings.HasPrefix(folded, FoldSlug(derived)) && isLeadingWords(folded, plain)
}

// isLeadingWords reports whether a folded slug is exactly the first few WORDS of a plain slug,
// so a cut inside a word never counts: `acme-p` is not a leading part of `acme-pvt-ltd`, and
// a company stored there was named "Acme P" by its source, not half-stripped by the rule.
func isLeadingWords(folded, plain string) bool {
	words := strings.Split(plain, "-")
	for k := 1; k <= len(words); k++ {
		if FoldSlug(strings.Join(words[:k], "-")) == folded {
			return true
		}
	}
	return false
}
