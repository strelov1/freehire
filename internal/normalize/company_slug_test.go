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
		{"a symbol word is not part of the key", "Acme ™", "acme"},
		{"a non-latin word is kept and transliterated", "Яндекс ООО", "iandeks-ooo"},
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
