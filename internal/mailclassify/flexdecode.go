package mailclassify

import (
	"encoding/json"

	"github.com/strelov1/freehire/internal/flexjson"
)

// UnmarshalJSON tolerates a confidence the model quotes ("0.8") and a matched job id it
// answers as "none"/"0", then delegates the rest via an alias (no recursion). encoding/json
// would otherwise abort the whole classification over one such slip, silently leaving the
// email unclassified. The exported fields stay float64/int64; Sanitize runs as before.
func (c *Classification) UnmarshalJSON(b []byte) error {
	type alias Classification
	aux := struct {
		Confidence   flexjson.Float `json:"confidence"`
		MatchedJobID flexjson.Int64 `json:"matched_job_id"`
		*alias
	}{alias: (*alias)(c)}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	c.Confidence = float64(aux.Confidence)
	c.MatchedJobID = int64(aux.MatchedJobID)
	return nil
}
