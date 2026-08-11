package experience

import (
	"encoding/json"

	"github.com/google/uuid"
)

// employmentWire is the kind-aware JSON shape for an Employment. Jobs keep `company`;
// projects expose the place label as `name` (still stored in Company).
//
// Kept as a helper rather than relying on struct tags alone so employmentWithAtoms
// (and any other embedder) can compose the same shape without losing sibling fields —
// an embedded type that implements MarshalJSON would otherwise replace the outer object.
type employmentWire struct {
	ID       uuid.UUID `json:"id"`
	Kind     string    `json:"kind"`
	Company  string    `json:"company,omitempty"`
	Name     string    `json:"name,omitempty"`
	Role     string    `json:"role,omitempty"`
	Location string    `json:"location,omitempty"`
	Start    string    `json:"start,omitempty"`
	End      string    `json:"end,omitempty"`
	Current  bool      `json:"current,omitempty"`
	Summary  string    `json:"summary,omitempty"`
	Link     string    `json:"link,omitempty"`
	Stack    []string  `json:"stack,omitempty"`
}

// ToWire returns the kind-aware JSON projection of e.
func (e Employment) ToWire() employmentWire {
	w := employmentWire{
		ID: e.ID, Kind: e.Kind, Role: e.Role, Location: e.Location,
		Start: e.Start, End: e.End, Current: e.Current, Summary: e.Summary,
		Link: e.Link, Stack: e.Stack,
	}
	if e.Kind == KindProject {
		w.Name = e.Company
	} else {
		w.Company = e.Company
	}
	return w
}

// MarshalJSON emits company for jobs and name for projects.
func (e Employment) MarshalJSON() ([]byte, error) {
	return json.Marshal(e.ToWire())
}

// UnmarshalJSON accepts kind-aware input: projects prefer `name`, with legacy
// `company` as a fallback into the stored Company field. Jobs take `company` only.
func (e *Employment) UnmarshalJSON(data []byte) error {
	var w employmentWire
	if err := json.Unmarshal(data, &w); err != nil {
		return err
	}
	e.ID = w.ID
	e.Kind = w.Kind
	e.Role = w.Role
	e.Location = w.Location
	e.Start = w.Start
	e.End = w.End
	e.Current = w.Current
	e.Summary = w.Summary
	e.Link = w.Link
	e.Stack = w.Stack
	if w.Kind == KindProject {
		if w.Name != "" {
			e.Company = w.Name
		} else {
			e.Company = w.Company
		}
	} else {
		e.Company = w.Company
	}
	return nil
}
