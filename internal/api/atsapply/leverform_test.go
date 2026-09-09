package atsapply

import "testing"

// Reduced from the live DOM of jobs.lever.co/coderio/6ce0e52b-e7bc-462f-ac9e-1d31c8c0e037/apply,
// captured 2026-09-09 from the production host. Every attribute below is verbatim; only
// markup between the controls was dropped.
//
// What it is here to show is that Lever's controls cannot be addressed by `id`. Most carry
// none at all; the résumé upload and the location autocomplete carry one, and in both cases
// it is a DIFFERENT string from the `name` the platform posts under — which is the trap a
// scan addressing by id would fall into silently, resolving a field Lever never heard of.
const leverFormHTML = `
<html><body>
<form id="application-form" enctype="multipart/form-data" method="POST">
  <select name="opportunityLocationId" class="opportunity-location" data-qa="opportunity-location-select"></select>
  <input class="application-file-input invisible-resume-upload" data-qa="input-resume" id="resume-upload-input" name="resume" tabindex="-1" type="file">
  <input type="text" data-qa="name-input" name="name" required="">
  <input name="email" data-qa="email-input" type="email" required="">
  <input type="text" data-qa="phone-input" name="phone" required="">
  <input class="location-input" data-qa="location-input" id="location-input" type="text" maxlength="100" name="location">
  <input type="text" data-qa="org-input" name="org">
  <input type="text" name="urls[LinkedIn]" required="">
  <input type="text" name="urls[GitHub]">
  <input type="radio" name="cards[192519c4-2fbe-4575-9ab0-db6643cd5135][field0]" value="Menos de 1 año" required="required">
  <input type="radio" name="cards[192519c4-2fbe-4575-9ab0-db6643cd5135][field0]" value="Entre 1 y 2 años" required="required">
  <button id="hcaptchaSubmitBtn" type="submit" class="hidden"></button>
  <button id="btn-submit" type="button" data-qa="btn-submit">Submit application</button>
</form>
</body></html>
`

// Under byName addressing a field's identity is its name, because that is the only thing
// Lever's controls carry — and it is what the platform posts under.
func TestScanForm_IdentifiesLeverFieldsByName(t *testing.T) {
	layout, _ := layoutFor("lever")

	fields, err := ScanForm(leverFormHTML, layout)
	if err != nil {
		t.Fatalf("ScanForm: %v", err)
	}

	found := map[string]DOMField{}
	for _, f := range fields {
		found[f.ID] = f
	}
	for _, want := range []string{"name", "email", "phone", "location", "resume", "org", "urls[LinkedIn]", "opportunityLocationId"} {
		if _, ok := found[want]; !ok {
			t.Errorf("no field identified as %q; scanned %+v", want, fields)
		}
	}
	if got := found["resume"].Kind; got != "file" {
		t.Errorf("resume kind = %q, want file", got)
	}
	if got := found["opportunityLocationId"].Kind; got != "select" {
		t.Errorf("opportunityLocationId kind = %q, want select", got)
	}
	// A radio group is the one field DOMField.ID is deliberately empty for — its members
	// share a name, not an id, and Name is authoritative there (see DOMField's own doc).
	// That predates this change and is not altered by it, so the group is looked up the way
	// the type says to.
	var group *DOMField
	for i := range fields {
		if fields[i].Name == "cards[192519c4-2fbe-4575-9ab0-db6643cd5135][field0]" {
			group = &fields[i]
		}
	}
	if group == nil {
		t.Fatalf("the employer's radio group was not scanned; got %+v", fields)
	}
	if group.Kind != "checkbox_group" {
		t.Errorf("the radio group's kind = %q, want checkbox_group", group.Kind)
	}
	if len(group.Options) != 2 {
		t.Errorf("the radio group carries %d options, want both values", len(group.Options))
	}
}

// The two controls that DO carry an id carry a different one from the name Lever posts
// under. Identifying by the id would resolve a field the platform has never heard of, and
// the failure would not surface until the fill found nothing on the page.
func TestScanForm_PrefersTheNameOverAnIdThatDiffersFromIt(t *testing.T) {
	layout, _ := layoutFor("lever")

	fields, err := ScanForm(leverFormHTML, layout)
	if err != nil {
		t.Fatalf("ScanForm: %v", err)
	}

	for _, f := range fields {
		if f.ID == "location-input" || f.ID == "resume-upload-input" {
			t.Errorf("a field was identified by its id (%q) rather than the name Lever posts under", f.ID)
		}
	}
}

// The platform that already worked must keep working: a Greenhouse control is still
// identified by its id.
func TestScanForm_StillIdentifiesGreenhouseFieldsByID(t *testing.T) {
	layout, _ := layoutFor("greenhouse")

	fields, err := ScanForm(greenhouseFixtureHTML, layout)
	if err != nil {
		t.Fatalf("ScanForm: %v", err)
	}
	if len(fields) == 0 {
		t.Fatal("no fields scanned from the vanilla Greenhouse fixture")
	}
	for _, f := range fields {
		// A checkbox/radio group is exempt: DOMField.ID is empty for one by design, since
		// its members share a name rather than an id. Every other control on a Greenhouse
		// page carries an id, and identifying one by anything else would move behaviour
		// for the platform that already worked.
		if f.Kind == "checkbox_group" {
			continue
		}
		if f.ID == "" {
			t.Errorf("a Greenhouse field came back with no identifier: %+v", f)
		}
	}
}

// A control a byName layout cannot address is dropped rather than given a synthetic key.
//
// Under byID such a control gets one so two anonymous controls do not collide in the scan.
// Under byName that key would instead mint a field that resolves and then cannot be typed
// into — the silent last-step failure the addressing setting exists to prevent.
func TestScanForm_DropsANamelessFieldUnderByNameAddressing(t *testing.T) {
	const html = `<html><body><form id="application-form">
	  <input type="text" name="email">
	  <input type="text" required="">
	</form></body></html>`
	layout, _ := layoutFor("lever")

	fields, err := ScanForm(html, layout)
	if err != nil {
		t.Fatalf("ScanForm: %v", err)
	}
	if len(fields) != 1 || fields[0].ID != "email" {
		t.Fatalf("scanned %+v, want only the addressable field", fields)
	}
}
