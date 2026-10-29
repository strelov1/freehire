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
		// matched literally, 株式会社 was romanised as Chinese into "zhu-shi-hui-she". It is
		// kept as a Latin mark rather than dropped, so a Japanese firm never lands on a foreign
		// namesake's key, and every spelling of the same form lands on the same mark.
		{"japanese form as a prefix", "株式会社Speee", "speee-kk"},
		{"japanese form as a suffix", "Speee株式会社", "speee-kk"},
		{"japanese abbreviated form, full-width brackets", "（株）Speee", "speee-kk"},
		{"japanese abbreviated form, half-width brackets", "Speee(株)", "speee-kk"},
		{"japanese abbreviated form, one character", "㈱Speee", "speee-kk"},
		{"japanese form with a full-width space", "株式会社\u3000Kaizen Platform", "kaizen-platform-kk"},
		{"japanese limited company has its own mark", "有限会社Acme", "acme-yk"},
		{"japanese godo kaisha has its own mark", "Amazon Japan合同会社", "amazon-japan-gk"},
		{"a latin form is still dropped beside a japanese one", "株式会社Acme Inc.", "acme-kk"},
		{"the mark is written once when the form repeats", "株式会社Acme株式会社", "acme-kk"},
		// Mynavi writes full-width letters and the middle dot where hrmos and herp write
		// ASCII and a space; unidecode folds both, and the form test must not depend on which.
		{"full-width letters with a japanese form", "ＦＵＴＵＲＥＭＩＮＤ株式会社", "futuremind-kk"},
		{"full-width abbreviated form", "（株）ＡＢＣ", "abc-kk"},
		{"middle dot is a word break", "株式会社 Future・Mind", "future-mind-kk"},
		{"japanese form inside a name is kept", "Acme株式会社 東京支店", "acmezhu-shi-hui-she-dong-jing-zhi-dian"},
		{"a name that is only a japanese form is not erased", "株式会社", "zhu-shi-hui-she"},
		// The South-Asian private-company forms. "Pvt" is a form wherever it stands, but
		// "Private" and the bracketed "(P)" are forms only directly before a stripped "Limited"
		// or "Ltd": alone, "Private" is an ordinary word some employers' names end in.
		{"indian pvt ltd", "Acme Software Pvt Ltd", "acme-software"},
		{"indian pvt ltd, punctuated", "Acme Software Pvt. Ltd.", "acme-software"},
		{"indian pvt ltd, bracketed", "Acme Software (Pvt) Ltd", "acme-software"},
		{"bare pvt is a form on its own", "Zoho Corporation Pvt", "zoho"},
		{"indian private limited", "Acme Private Limited", "acme"},
		{"indian private ltd", "Acme Private Ltd.", "acme"},
		{"indian private limited, bracketed", "Acme (Private) Limited", "acme"},
		{"indian (p) ltd", "Infomaze Elite (P) Ltd.", "infomaze-elite"},
		{"singapore pte ltd", "EPAM Systems Pte. Ltd.", "epam-systems"},
		{"singapore pte ltd, bracketed", "Volvo Group Singapore (Pte.) Ltd.", "volvo-group-singapore"},
		// A dot is not a word break (it lives inside "B.V."), so these arrive as ONE word.
		{"forms glued by a dot", "Acme Pvt.Ltd.", "acme"},
		{"singapore forms glued by a dot", "Acme Pte.Ltd", "acme"},
		{"a dotted name is not a glued form", "Acme Foo.com", "acme-foo-com"},
		{"private alone is a word, not a form", "Julius Baer Private", "julius-baer-private"},
		{"private before a non-form is kept", "Private Bank", "private-bank"},
		{"private mid-name is kept", "Private Equity Partners", "private-equity-partners"},
		{"private before a non-limited form is kept", "Acme Private GmbH", "acme-private"},
		{"a bracketed initial alone is kept", "Milton Roy India (P)", "milton-roy-india-p"},
		{"a name that is only a compound form is not erased", "Private Limited", "private"},
		// The European and Latin-American forms (freehire#3876): each keyed its employer apart
		// from the same company's unsuffixed spelling — "Microsoft s.r.o.", "Accenture Kft.".
		{"balkan d.o.o.", "JYSK d.o.o.", "jysk"},
		{"croatian simple d.o.o.", "Simply Clean j.d.o.o.", "simply-clean"},
		{"balkan d.d.", "Hrvatski Telekom d.d.", "hrvatski-telekom"},
		{"czech s.r.o. after a comma", "Devoteam, s.r.o.", "devoteam"},
		{"hungarian kft", "Accenture Kft.", "accenture"},
		{"hungarian zrt", "Raiffeisen Bank Zrt.", "raiffeisen-bank"},
		{"hungarian nyrt", "OTP Bank Nyrt.", "otp-bank"},
		{"bulgarian eood", "Sunotec EOOD", "sunotec"},
		{"albanian sh.p.k.", "Acme sh.p.k.", "acme"},
		{"finnish oyj", "Elisa Oyj", "elisa"},
		{"norwegian asa", "Orkla ASA", "orkla"},
		{"lithuanian uab", "Reiz Tech UAB", "reiz-tech"},
		{"icelandic ehf", "Treble Technologies ehf.", "treble-technologies"},
		{"spanish s.l.", "Kapres Technology, S.L.", "kapres-technology"},
		{"brazilian ltda", "Accenture LTDA", "accenture"},
		{"portuguese lda", "Wondercom, Lda", "wondercom"},
		{"french sas", "Airbus S.A.S.", "airbus"},
		{"french sarl", "Emagine Consulting SARL", "emagine-consulting"},
		{"swiss sagl", "WellD Sagl", "welld"},
		{"belgian bvba", "Dataroots BVBA", "dataroots"},
		{"german ug", "Sorella UG", "sorella"},
		{"malaysian sdn bhd", "Acme Sdn Bhd", "acme"},
		{"malaysian bhd alone", "Mimos Bhd", "mimos"},
		{"sdn alone is not a form", "Acme Sdn", "acme-sdn"},
		// The Polish form is three words, and only the last is a form on its own.
		{"polish sp. z o.o.", "Antal Sp. z o.o.", "antal"},
		{"polish spółka z o.o.", "Mondi Spółka z o.o.", "mondi"},
		{"polish spz o.o.", "Acme Poland Sp.z o.o.", "acme-poland"},
		{"polish spolka, ascii", "Acme Spolka z o.o.", "acme"},
		{"polish o. o. spaced", "Grupa Allegro Sp. z o. o.", "grupa-allegro"},
		{"polish o. o. spaced, capitals", "KPS FOOD SP. Z O. O.", "kps-food"},
		{"polish o. o. spaced, bracketed", "Acme (Sp. z o. o.)", "acme"},
		{"a spaced d. o. o. is left as it was", "Acme d. o. o.", "acme-d-o-o"},
		{"estonian ou", "Shuttle99 OU", "shuttle99"},
		{"estonian oü", "Shuttle99 OÜ", "shuttle99"},
		{"a trailing z alone is kept", "Generation Z", "generation-z"},
		{"zoo is not the polish form", "Phoenix Zoo", "phoenix-zoo"},
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
