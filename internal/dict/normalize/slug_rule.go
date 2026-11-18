package normalize

import "strings"

// SlugPredatesRule reports whether a stored company slug was minted from THIS name by an older
// company rule: it is the faithful [Slug] of the name, yet [CompanySlug] now derives a different
// one (a legal form the rule has learned since, e.g. the Japanese 株式会社 once stored as
// "zhu-shi-hui-she-…" and now keyed as a `kk` mark).
//
// The Slug test keeps this to the rule's own change: a slug that is not the plain slug of the
// latest name drifted because the source renamed the employer, which is a judgement about the
// rename, not about a form.
//
// The two are compared FOLDED, because the rule and Slug disagree about word breaks in ways
// that say nothing about the employer ("ディー・エヌ・エー" splits at the middle dot; "Linc'well"
// loses its apostrophe in the rule but is split by Slug).
//
// A slug minted by an older rule that stripped only PART of the form also qualifies: "Acme Pvt
// Ltd" keyed `acme-pvt` while the rule knew "Ltd" but not "Pvt". So a stored slug qualifies when
// it lies BETWEEN the current key and the plain slug (the key a prefix of it, it a prefix of the
// plain slug): what separates it from the key is a trailing word the rule now strips. A renamed
// employer fails from either side: "Acme" renamed "Acme Holdings Ltd" is shorter than its new
// key, and a different name is no prefix of the plain slug.
func SlugPredatesRule(stored, name string) bool {
	derived := CompanySlug(name)
	if derived == stored {
		return false
	}
	plain := Slug(name)
	if derived == plain {
		// The rule makes nothing of this name that Slug does not, so it cannot have moved the stored
		// slug; one that differs (even only in word breaks) came from another spelling: a judgement.
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
