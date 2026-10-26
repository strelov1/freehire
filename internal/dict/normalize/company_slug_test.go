package normalize

import "testing"

// TestCompanySlug pins the catalogue's company-key rule.
//
// The cases fall into two groups. Most guard behaviour that already holds and must survive
// the move to field-level tokenization — the repeating strip that reduces "Acme GmbH & Co.
// KG" to one word, and the names that must NOT be stripped at all. The punctuated forms are
// the reason the tokenization changes: a form is recognised by its ASCII letters, so "B.V."
// is the `bv` token, which a check against Slug's hyphenated output ("booking-b-v") can
// never see.
func TestCompanySlug(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"trailing legal form", "RingCentral, Inc.", "ringcentral"},
		{"no space before the form", "Sun Technologies,Inc.", "sun-technologies"},
		{"hyphen before the form", "Hapag-Lloyd AG", "hapag-lloyd"},
		{"punctuated legal form", "Booking B.V.", "booking"},
		{"punctuated romance form", "Acme S.A.", "acme"},
		{"punctuated nordic form", "Trafalgar A/S", "trafalgar"},
		{"compound form strips whole, stepping over the ampersand", "Acme GmbH & Co. KG", "acme"},
		{"ampersand company form", "Tiffany & Co.", "tiffany"},
		{"strip stops at a word that is not a form", "Acme Holdings Ltd", "acme-holdings"},
		{"form word inside a name is kept", "Limited Brands", "limited-brands"},
		{"a name that is only a form is not erased", "Limited", "limited"},
		{"no form to strip", "Yandex", "yandex"},
		// An apostrophe is not a word break, so it must not become one in the key. Slug turns
		// "Kohl's" into kohl-s, and once the canon is DERIVED from the name that artefact
		// could no longer lose to the clean sibling spelling — it elected kohl-s over kohls
		// across 2,939 postings.
		{"possessive apostrophe", "Kohl's", "kohls"},
		{"possessive apostrophe, US style", "Domino's", "dominos"},
		{"apostrophe inside a name", "Brink's Incorporated", "brinks"},
		{"typographic apostrophe", "Macy\u2019s", "macys"},
		{"a symbol word is not part of the key", "Acme ™", "acme"},
		{"a non-latin word is kept and transliterated", "Яндекс ООО", "iandeks-ooo"},
		// Japanese writes the form with no space, on either side of the name; before it was
		// matched literally, 株式会社 was romanised as Chinese into "zhu-shi-hui-she".
		{"japanese form as a prefix", "株式会社Speee", "speee"},
		{"japanese form as a suffix", "freee株式会社", "freee"},
		{"japanese form with a full-width space", "株式会社\u3000Kaizen Platform", "kaizen-platform"},
		{"japanese limited company", "有限会社Acme", "acme"},
		{"japanese godo kaisha", "Amazon Japan合同会社", "amazon-japan"},
		{"japanese abbreviated form, full-width brackets", "（株）Acme", "acme"},
		{"japanese abbreviated form, half-width brackets", "Acme(株)", "acme"},
		{"japanese abbreviated form, one character", "㈱Acme", "acme"},
		{"japanese form and a latin form together", "株式会社Acme Inc.", "acme"},
		{"japanese form inside a name is kept", "Acme株式会社 東京支店", "acmezhu-shi-hui-she-dong-jing-zhi-dian"},
		{"a name that is only a japanese form is not erased", "株式会社", "zhu-shi-hui-she"},
		{"empty stays empty", "", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := CompanySlug(tc.in); got != tc.want {
				t.Errorf("CompanySlug(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
