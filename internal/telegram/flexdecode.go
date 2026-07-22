package telegram

import (
	"encoding/json"

	"github.com/strelov1/freehire/internal/flexjson"
)

// UnmarshalJSON tolerates a "remote" flag the model returns as "true"/"yes" or 1 instead
// of a bool, then delegates the rest via an alias (no recursion). encoding/json would
// otherwise abort the whole post's extraction over one such slip. The exported Remote
// stays bool; Validate runs on the canonical shape as before.
func (j *ExtractedJob) UnmarshalJSON(b []byte) error {
	type alias ExtractedJob
	aux := struct {
		Remote flexjson.Bool `json:"remote"`
		*alias
	}{alias: (*alias)(j)}
	if err := json.Unmarshal(b, &aux); err != nil {
		return err
	}
	j.Remote = bool(aux.Remote)
	return nil
}
