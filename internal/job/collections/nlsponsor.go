package collections

import (
	"errors"
	"html"
	"regexp"
	"strings"
)

// nlSponsorURL is the IND public register of recognised sponsors for the "work"
// residence purpose (which covers the highly-skilled-migrant scheme). A single stable page,
// with no route breakdown: recognition is the whole fact.
const nlSponsorURL = "https://ind.nl/en/public-register-recognised-sponsors/public-register-work"

// nlSponsorRow matches one register row: the organisation as a row header, its KvK
// number in the cell beside it.
var nlSponsorRow = regexp.MustCompile(
	`(?is)<th[^>]*scope=["']row["'][^>]*>(.*?)</th>\s*<td[^>]*>(.*?)</td>`)

// tagPattern strips the inline markup the register wraps some names in.
var tagPattern = regexp.MustCompile(`<[^>]*>`)

// ParseNLSponsors reads the IND register page into one record per recognised
// sponsor, keeping the KvK number as the identity field that tells two same-named
// organisations apart. A page that yields no rows is an error, not an empty register:
// returning nothing would reconcile the credential off every company.
func ParseNLSponsors(page []byte) ([]Record, error) {
	matches := nlSponsorRow.FindAllStringSubmatch(string(page), -1)
	records := make([]Record, 0, len(matches))
	for _, m := range matches {
		name := cellText(m[1])
		if name == "" {
			continue
		}
		records = append(records, Record{Name: name, Meta: map[string]string{"kvk": cellText(m[2])}})
	}
	if len(records) == 0 {
		return nil, errors.New("collections: nl recognised-sponsor register parsed to no rows")
	}
	return records, nil
}

// cellText reduces a table cell to its visible text: markup removed, HTML entities
// decoded, whitespace collapsed.
func cellText(cell string) string {
	return strings.Join(strings.Fields(html.UnescapeString(tagPattern.ReplaceAllString(cell, " "))), " ")
}
