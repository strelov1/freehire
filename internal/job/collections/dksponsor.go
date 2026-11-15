package collections

import (
	"errors"
	"regexp"
	"strings"
)

// dkSponsorURL is SIRI's list of companies certified for the Danish Fast-track
// scheme, a single stable page like the IND register.
//
// The list is narrower than "employers that sponsor": companies can also hire through the Pay
// Limit or Positive List schemes uncertified, so the credential's copy names the scheme.
const dkSponsorURL = "https://www.nyidanmark.dk/en-GB/Words-and-concepts/SIRI/Certified-companies"

// dkSponsorRow matches one row of the table: company name, then CVR number. The
// table is pasted into a CMS, so cell markup varies row to row; only the two-cell shape is
// relied on.
var dkSponsorRow = regexp.MustCompile(
	`(?is)<tr[^>]*>\s*<td[^>]*>(.*?)</td>\s*<td[^>]*>(.*?)</td>\s*</tr>`)

// ParseDKSponsors reads the SIRI page into one record per certified company,
// keeping the CVR number as the identity field. The header row is a <td> row too, so it is
// skipped by its text. A page that yields no rows is an error: the page changed, and returning
// nothing would reconcile the credential off every company.
func ParseDKSponsors(page []byte) ([]Record, error) {
	var records []Record
	for _, m := range dkSponsorRow.FindAllStringSubmatch(string(page), -1) {
		name := cellText(m[1])
		if name == "" || strings.EqualFold(name, "Name of company") {
			continue
		}
		records = append(records, Record{Name: name, Meta: map[string]string{"cvr": cellText(m[2])}})
	}
	if len(records) == 0 {
		return nil, errors.New("collections: dk certified-companies list parsed to no rows")
	}
	return records, nil
}
