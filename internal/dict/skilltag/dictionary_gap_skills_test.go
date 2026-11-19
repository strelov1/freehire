package skilltag

import (
	"slices"
	"testing"
)

// TestDictionaryGapSkills covers skills people asked the dictionary for (dictionary_gaps):
// Prisma, Retool and product design resolve when asserted but collide in job prose, so job
// text reaches them only through qualified phrases; 1C-Bitrix and CVE are safe everywhere.
func TestDictionaryGapSkills(t *testing.T) {
	for _, c := range []struct{ term, want string }{
		{"Prisma", "prisma"}, {"prisma", "prisma"},
		{"Retool", "retool"}, {"retool", "retool"},
		{"Product Design", "product-design"}, {"product design", "product-design"},
		{"1C-Bitrix", "1c-bitrix"}, {"Bitrix D7", "1c-bitrix"}, {"1С-Битрикс", "1c-bitrix"},
		{"cve", "vulnerability-management"}, {"CVE", "vulnerability-management"},
	} {
		if got := Canonicalize([]string{c.term}, WithResumeAcronyms()); !slices.Equal(got, []string{c.want}) {
			t.Errorf("Canonicalize(%q, résumé) = %v, want [%s]", c.term, got, c.want)
		}
	}
	for _, term := range []string{"Prisma", "Retool", "Product Design"} {
		if got := Canonicalize([]string{term}); len(got) != 0 {
			t.Errorf("job-side Canonicalize(%q) = %v, want nothing", term, got)
		}
	}

	for _, c := range []struct{ text, want string }{
		{"Backend on Node.js with TypeScript, PostgreSQL and Prisma ORM.", "prisma"},
		{"You will build internal tools as Retool apps on top of our Postgres.", "retool"},
		{"Lead digital product design for our mobile banking app in Figma.", "product-design"},
		{"Разработка на 1С-Битрикс, знание PHP и MySQL.", "1c-bitrix"},
		{"PHP developer: Bitrix D7, custom components, MySQL.", "1c-bitrix"},
		{"Triage CVEs in our container images and patch dependencies.", "vulnerability-management"},
	} {
		if got := Parse(c.text); !slices.Contains(got, c.want) {
			t.Errorf("Parse(%q) = %v, want %s", c.text, got, c.want)
		}
	}
	for _, c := range []struct{ text, not string }{
		{"Secure our cloud with Prisma Cloud and Cortex XSOAR.", "prisma"},
		{"We will retool the warehouse line for the new product.", "retool"},
		{"Industrial engineer: product design of injection-moulded parts in SolidWorks.", "product-design"},
		{"Менеджер по продажам: ведение клиентов в Битрикс24.", "1c-bitrix"},
	} {
		if got := Parse(c.text); slices.Contains(got, c.not) {
			t.Errorf("Parse(%q) = %v, must not contain %s", c.text, got, c.not)
		}
	}
}
