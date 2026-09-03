package atsapply

import "testing"

// A trimmed stand-in for the real Greenhouse application form scanned in the 2026-09-02
// spike (webflow job 7951430) — enough shapes to exercise the parser: plain required/
// optional text inputs, a react-select-style text input with no `required` HTML attribute
// (country), a file upload, a textarea, and a checkbox group sharing one `name` (the EEOC
// shape) that must collapse into one logical field rather than N.
const greenhouseFixtureHTML = `
<html><body>
<form id="application-form">
  <input id="first_name" name="first_name" type="text" required>
  <input id="last_name" name="last_name" type="text" required>
  <input id="email" name="email" type="text" required>
  <input id="country" name="country" type="text">
  <input id="resume" name="resume" type="file" required>
  <textarea id="question_67131484" name="question_67131484"></textarea>
  <input id="gh_src" name="gh_src" type="hidden" value="opaque-token">
  <input id="gender_1" name="gender" type="checkbox" value="Male">
  <input id="gender_2" name="gender" type="checkbox" value="Female">
</form>
</body></html>
`

func TestScanGreenhouseForm_ExtractsPlainFields(t *testing.T) {
	fields, err := ScanGreenhouseForm(greenhouseFixtureHTML)
	if err != nil {
		t.Fatalf("ScanGreenhouseForm: %v", err)
	}

	byID := map[string]DOMField{}
	for _, f := range fields {
		byID[f.ID] = f
	}

	if f, ok := byID["first_name"]; !ok || !f.Required || f.Kind != "text" {
		t.Errorf("first_name = %+v (ok=%v), want required text", f, ok)
	}
	if f, ok := byID["country"]; !ok || f.Required {
		t.Errorf("country = %+v (ok=%v), want present and NOT required (no HTML attribute)", f, ok)
	}
	if f, ok := byID["resume"]; !ok || f.Kind != "file" || !f.Required {
		t.Errorf("resume = %+v (ok=%v), want required file", f, ok)
	}
	if f, ok := byID["question_67131484"]; !ok || f.Kind != "textarea" {
		t.Errorf("question_67131484 = %+v (ok=%v), want textarea", f, ok)
	}
}

func TestScanGreenhouseForm_SkipsHiddenFields(t *testing.T) {
	fields, err := ScanGreenhouseForm(greenhouseFixtureHTML)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range fields {
		if f.ID == "gh_src" {
			t.Errorf("hidden field gh_src was scanned as %+v, want it skipped — it is platform-filled, never a candidate answer", f)
		}
	}
}

func TestScanGreenhouseForm_GroupsACheckboxGroupByName(t *testing.T) {
	fields, err := ScanGreenhouseForm(greenhouseFixtureHTML)
	if err != nil {
		t.Fatal(err)
	}

	var genderFields []DOMField
	for _, f := range fields {
		if f.Name == "gender" {
			genderFields = append(genderFields, f)
		}
	}
	if len(genderFields) != 1 {
		t.Fatalf("gender fields = %d, want exactly 1 (a checkbox group is one logical field, not one per option)", len(genderFields))
	}
	if !genderFields[0].Multi || genderFields[0].Kind != "checkbox_group" {
		t.Errorf("gender field = %+v, want Multi=true Kind=checkbox_group", genderFields[0])
	}
	if len(genderFields[0].Options) != 2 {
		t.Errorf("gender options = %d, want 2", len(genderFields[0].Options))
	}
}

func TestScanGreenhouseForm_NoApplicationFormIsAnError(t *testing.T) {
	if _, err := ScanGreenhouseForm(`<html><body>not a form page</body></html>`); err == nil {
		t.Fatal("want an error when #application-form is not on the page")
	}
}

// Found by code review: a plain input with neither an id nor a name attribute — the real
// shape the 2026-09-02 live smoke test measured on a live posting — must still be recorded
// as its own field, not silently dropped. Dropping a required one would let
// Plan.FullyResolved() report true while a real required question was never even seen.
func TestScanGreenhouseForm_APlainInputWithNoIDOrNameIsStillScanned(t *testing.T) {
	const html = `<html><body><form id="application-form">
	  <input type="text" required>
	</form></body></html>`

	fields, err := ScanGreenhouseForm(html)
	if err != nil {
		t.Fatalf("ScanGreenhouseForm: %v", err)
	}
	if len(fields) != 1 {
		t.Fatalf("fields = %d, want the id-less/name-less input recorded, not dropped", len(fields))
	}
	if !fields[0].Required {
		t.Error("want the field's required attribute preserved")
	}
}

// The severe version of the same gap: TWO id-less/name-less inputs on one page must not
// collide on the same fallback key — the second must not silently vanish.
func TestScanGreenhouseForm_TwoPlainInputsWithNoIDOrNameDoNotCollide(t *testing.T) {
	const html = `<html><body><form id="application-form">
	  <input type="text" required>
	  <input type="text" required>
	</form></body></html>`

	fields, err := ScanGreenhouseForm(html)
	if err != nil {
		t.Fatalf("ScanGreenhouseForm: %v", err)
	}
	if len(fields) != 2 {
		t.Fatalf("fields = %d, want both id-less/name-less inputs kept as distinct fields", len(fields))
	}
}

// A textarea/select with no id or name must not be dropped either — scanSimple's own,
// even more direct instance of the same gap (it returned immediately on an empty key,
// never recording anything at all).
func TestScanGreenhouseForm_ATextareaWithNoIDOrNameIsStillScanned(t *testing.T) {
	const html = `<html><body><form id="application-form">
	  <textarea required></textarea>
	</form></body></html>`

	fields, err := ScanGreenhouseForm(html)
	if err != nil {
		t.Fatalf("ScanGreenhouseForm: %v", err)
	}
	if len(fields) != 1 {
		t.Fatalf("fields = %d, want the id-less/name-less textarea recorded, not dropped", len(fields))
	}
}
