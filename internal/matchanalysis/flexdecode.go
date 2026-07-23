package matchanalysis

import (
	"encoding/json"

	"github.com/strelov1/freehire/internal/flexjson"
)

// UnmarshalJSON tolerates a dimension score the model returns as a string ("85") or a
// ratio ("8/10") instead of an integer, then delegates the rest via an alias (no
// recursion). encoding/json would otherwise abort the whole recruiter verdict over one
// such slip. The exported Score stays int; sanitizeVerdict clamps it as before.
func (d *dimScore) UnmarshalJSON(b []byte) error {
	type alias dimScore
	aux := struct {
		Score flexjson.Int `json:"score"`
		*alias
	}{alias: (*alias)(d)}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	d.Score = int(aux.Score)
	return nil
}
