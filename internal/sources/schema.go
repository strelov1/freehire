package sources

import (
	"bytes"
	"encoding/json"
	"strings"
)

// Shared schema.org JobPosting decode helpers. ATS boards that server-render (or inline) a
// schema.org JobPosting ld+json block share the same field shapes; these types and mappers let
// each adapter decode the common parts without redeclaring them.

// schemaEmploymentType maps a schema.org JobPosting employmentType enum onto the freehire
// vocabulary, returning "" for an unknown/absent value (e.g. "OTHER") so the description parser
// decides. Shared by the ld+json adapters (northstone, briefhq, djinni, talentlyft, wantapply,
// onstrider).
func schemaEmploymentType(t string) string {
	switch strings.ToUpper(strings.TrimSpace(t)) {
	case "FULL_TIME":
		return "full_time"
	case "PART_TIME":
		return "part_time"
	case "CONTRACTOR", "TEMPORARY":
		return "contract"
	case "INTERN", "INTERNSHIP":
		return "internship"
	default:
		return ""
	}
}

// schemaAddress is a schema.org PostalAddress as ATS JobPosting ld+json emits it. The ld+json
// adapters decode jobLocation.address into it instead of each redeclaring the same three fields.
type schemaAddress struct {
	AddressLocality string `json:"addressLocality"`
	AddressRegion   string `json:"addressRegion"`
	AddressCountry  string `json:"addressCountry"`
}

// Location joins the address parts as "City, Region, Country", skipping blanks. Adapters whose
// jobLocation carries all three parts use it instead of re-spelling the joinNonEmpty. A board
// that intentionally omits the region (to avoid it surfacing in the text) must NOT use this.
func (a schemaAddress) Location() string {
	return joinNonEmpty(a.AddressLocality, a.AddressRegion, a.AddressCountry)
}

// schemaPlace is one schema.org Place (a jobLocation entry): its postal address.
type schemaPlace struct {
	Address schemaAddress `json:"address"`
}

// schemaPlaces decodes JobPosting.jobLocation, which ATS ld+json emits as EITHER a single Place
// object (single-location jobs) OR an array of them (multi-location), normalizing both to a
// slice. Without this a single-object jobLocation fails to unmarshal into a []schemaPlace and the
// whole posting is silently dropped. Shared by icims, jobvite, and twogis.
type schemaPlaces []schemaPlace

func (p *schemaPlaces) UnmarshalJSON(b []byte) error {
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		var arr []schemaPlace
		if err := json.Unmarshal(trimmed, &arr); err != nil {
			return err
		}
		*p = arr
		return nil
	}
	var one schemaPlace
	if err := json.Unmarshal(trimmed, &one); err != nil {
		return err
	}
	*p = []schemaPlace{one}
	return nil
}
