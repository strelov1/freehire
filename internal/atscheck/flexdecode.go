package atscheck

import (
	"encoding/json"

	"github.com/strelov1/freehire/internal/flexjson"
)

// UnmarshalJSON tolerates a content-quality score the model returns as a string ("85") or
// "85/100" instead of an integer, then delegates the rest via an alias (no recursion).
// encoding/json would otherwise abort the whole review over one such slip. The exported
// ContentQuality stays int; sanitize clamps it as before.
func (r *Review) UnmarshalJSON(b []byte) error {
	type alias Review
	aux := struct {
		ContentQuality flexjson.Int `json:"content_quality"`
		*alias
	}{alias: (*alias)(r)}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	r.ContentQuality = int(aux.ContentQuality)
	return nil
}
