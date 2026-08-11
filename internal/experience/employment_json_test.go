package experience

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
)

func TestEmploymentMarshalJobKeepsCompany(t *testing.T) {
	id := uuid.New()
	e := Employment{ID: id, Kind: KindJob, Company: "RingCentral", Role: "SWE"}
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["company"] != "RingCentral" {
		t.Errorf("company = %v, want RingCentral", m["company"])
	}
	if _, ok := m["name"]; ok {
		t.Errorf("job must not emit name, got %v", m["name"])
	}
}

func TestEmploymentMarshalProjectEmitsName(t *testing.T) {
	id := uuid.New()
	e := Employment{ID: id, Kind: KindProject, Company: "telagon.io", Link: "https://telagon.io"}
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	if m["name"] != "telagon.io" {
		t.Errorf("name = %v, want telagon.io", m["name"])
	}
	if _, ok := m["company"]; ok {
		t.Errorf("project must not emit company, got %v", m["company"])
	}
	if m["link"] != "https://telagon.io" {
		t.Errorf("link = %v, want URL retained", m["link"])
	}
}

func TestEmploymentUnmarshalProjectName(t *testing.T) {
	var e Employment
	if err := json.Unmarshal([]byte(`{"kind":"project","name":"opensched","link":"https://opensched.dev"}`), &e); err != nil {
		t.Fatal(err)
	}
	if e.Kind != KindProject || e.Company != "opensched" || e.Link != "https://opensched.dev" {
		t.Errorf("got %+v, want company filled from name", e)
	}
}

func TestEmploymentUnmarshalProjectLegacyCompany(t *testing.T) {
	var e Employment
	if err := json.Unmarshal([]byte(`{"kind":"project","company":"opensched"}`), &e); err != nil {
		t.Fatal(err)
	}
	if e.Company != "opensched" {
		t.Errorf("company = %q, want legacy company accepted as place label", e.Company)
	}
}

func TestEmploymentUnmarshalProjectNameWinsOverCompany(t *testing.T) {
	var e Employment
	if err := json.Unmarshal([]byte(`{"kind":"project","name":"keep","company":"ignore"}`), &e); err != nil {
		t.Fatal(err)
	}
	if e.Company != "keep" {
		t.Errorf("company = %q, want name to win", e.Company)
	}
}

func TestEmploymentUnmarshalJobIgnoresName(t *testing.T) {
	var e Employment
	if err := json.Unmarshal([]byte(`{"kind":"job","company":"Acme","name":"not-a-company","role":"Dev"}`), &e); err != nil {
		t.Fatal(err)
	}
	if e.Company != "Acme" {
		t.Errorf("company = %q, want job company; name must be ignored", e.Company)
	}
}
