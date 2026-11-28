package aggregators

import "encoding/json"

// looseID reads a posting id that an API emits as a JSON string for some postings and as a bare
// number for others (Adzuna does, observed live; Jooble documents an integer). A page hitting
// the other shape must decode rather than fail whole, mirroring habrJobLocations/habrAddress's
// shape-tolerant UnmarshalJSON.
type looseID string

func (id *looseID) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		*id = looseID(s)
		return nil
	}
	var n json.Number
	if err := json.Unmarshal(b, &n); err != nil {
		return err
	}
	*id = looseID(n.String())
	return nil
}
