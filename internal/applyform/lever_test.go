package applyform

import (
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// The markup below is copied in shape from live Lever apply pages, including the parts
// that would be got wrong by assumption: a required question states the requirement
// three ways at once, and a radio group is several inputs sharing one submit name with
// the readable text in a sibling span rather than in the input.
const leverPage = `<html><body><form>
<ul class="application-questions">

<li class="application-question">
  <div class="application-label"><div class="text">Full name<span class="required">✱</span></div></div>
  <div class="application-field required-field">
    <input type="text" name="name" required="required" />
  </div>
</li>

<li class="application-question">
  <div class="application-label"><div class="text">LinkedIn URL</div></div>
  <div class="application-field"><input type="text" name="urls[LinkedIn]" /></div>
</li>

<li class="application-question">
  <div class="application-label"><div class="text">Resume/CV<span class="required">✱</span></div></div>
  <div class="application-field required-field"><input type="file" name="resume" required="required" /></div>
</li>

<li class="application-question custom-question">
  <div class="application-label multiple-choice"><div class="text">Are you authorized to work in the United States?<span class="required">✱</span></div></div>
  <div class="application-field required-field">
    <ul data-qa="multiple-choice">
      <li><label><input type="radio" name="cards[115d9079][field0]" value="Yes" required="required" /><span class="application-answer-alternative">Yes</span></label></li>
      <li><label><input type="radio" name="cards[115d9079][field0]" value="No" required="required" /><span class="application-answer-alternative">No</span></label></li>
    </ul>
  </div>
</li>

<li class="application-question custom-question">
  <div class="application-label"><div class="text">If you selected international, what location(s)?</div></div>
  <div class="application-field"><textarea name="cards[f27077c2][field2]"></textarea></div>
</li>

<li class="application-question custom-question">
  <div class="application-label"><div class="text">How did you hear about us?<span class="required">✱</span></div></div>
  <div class="application-field required-field">
    <select name="cards[82aec075][field0]" required="required">
      <option value="">Select...</option>
      <option value="Referral">A friend told me</option>
      <option value="Event">At an event</option>
    </select>
  </div>
</li>

<li class="application-question">
  <div class="application-label"><div class="text">I agree to the storage of my data<span class="required">✱</span></div></div>
  <div class="application-field required-field">
    <input type="hidden" name="consent[store]" value="false" />
    <input type="checkbox" name="consent[store]" value="true" required="required" />
  </div>
</li>

<li class="application-question">
  <div class="application-label"><div class="text">Nothing to answer here</div></div>
  <div class="application-field"><p>Just a note from the employer.</p></div>
</li>

<li class="application-question">
  <div class="application-field full-width"><ul><li><label>
    <input type="hidden" name="consent[marketing]" value="0" />
    <input type="checkbox" name="consent[marketing]" value="1" />
    <p class="application-answer-alternative">Yes, Acme can contact me about future roles for up to 1 year</p>
  </label></li></ul></div>
</li>

<li class="application-question">
  <div class="application-field full-width">
    <p data-qa="legitimate-interest-copy"><div>Acme collects and processes your personal data solely for your application.</div></p>
    <ul><li><label>
      <span class=""><div>Yes, notify me about future roles that match my profile.</div></span>
      <input type="hidden" name="consent[notify]" value="0" />
      <input type="checkbox" name="consent[notify]" value="1" />
    </label></li></ul>
  </div>
</li>

</ul></form></body></html>`

func parseLever(t *testing.T) Form {
	t.Helper()
	doc, err := html.Parse(strings.NewReader(leverPage))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	return FromLever(doc)
}

func leverField(t *testing.T, f Form, id string) Field {
	t.Helper()
	for _, got := range f.Fields {
		if got.ID == id {
			return got
		}
	}
	var ids []string
	for _, got := range f.Fields {
		ids = append(ids, got.ID)
	}
	t.Fatalf("no control %q among %v", id, ids)
	return Field{}
}

func TestFromLeverReadsTheControlShapes(t *testing.T) {
	form := parseLever(t)

	if form.Provider != "lever" {
		t.Errorf("provider = %q, want %q", form.Provider, "lever")
	}

	for _, tc := range []struct {
		id       string
		label    string
		typ      FieldType
		required bool
	}{
		{"name", "Full name", TypeText, true},
		{"urls[LinkedIn]", "LinkedIn URL", TypeText, false},
		{"resume", "Resume/CV", TypeFile, true},
		{"cards[115d9079][field0]", "Are you authorized to work in the United States?", TypeSelect, true},
		{"cards[f27077c2][field2]", "If you selected international, what location(s)?", TypeTextarea, false},
		{"cards[82aec075][field0]", "How did you hear about us?", TypeSelect, true},
		{"consent[store]", "I agree to the storage of my data", TypeBoolean, true},
	} {
		got := leverField(t, form, tc.id)
		if got.Label != tc.label {
			t.Errorf("%s label = %q, want %q", tc.id, got.Label, tc.label)
		}
		if got.Type != tc.typ {
			t.Errorf("%s type = %q, want %q", tc.id, got.Type, tc.typ)
		}
		if got.Required != tc.required {
			t.Errorf("%s required = %v, want %v", tc.id, got.Required, tc.required)
		}
	}
}

// Several radio inputs share one submit name. Read control by control this becomes one
// question per alternative — "Are you authorized to work in the United States?" twice,
// once for Yes and once for No.
func TestFromLeverFoldsARadioGroupIntoOneControl(t *testing.T) {
	form := parseLever(t)

	seen := 0
	for _, f := range form.Fields {
		if f.ID == "cards[115d9079][field0]" {
			seen++
		}
	}
	if seen != 1 {
		t.Fatalf("the radio group produced %d controls, want 1", seen)
	}

	got := leverField(t, form, "cards[115d9079][field0]")
	want := []Option{{Label: "Yes", Value: "Yes"}, {Label: "No", Value: "No"}}
	if len(got.Options) != len(want) {
		t.Fatalf("options = %+v, want %+v", got.Options, want)
	}
	for i, o := range got.Options {
		if o != want[i] {
			t.Errorf("option %d = %+v, want %+v", i, o, want[i])
		}
	}
}

// A select's two halves differ: the submit token is the option's value attribute and the
// text a candidate reads is its content. The placeholder is not an answer.
func TestFromLeverReadsSelectOptions(t *testing.T) {
	got := leverField(t, parseLever(t), "cards[82aec075][field0]")

	want := []Option{
		{Label: "A friend told me", Value: "Referral"},
		{Label: "At an event", Value: "Event"},
	}
	if len(got.Options) != len(want) {
		t.Fatalf("options = %+v, want %+v — the empty placeholder is not an answer", got.Options, want)
	}
	for i, o := range got.Options {
		if o != want[i] {
			t.Errorf("option %d = %+v, want %+v", i, o, want[i])
		}
	}
}

// Lever appends a glyph to a required question's label. It marks the requirement, which
// is the control's flag; leaving it in the text would put punctuation in the middle of
// every required question on the page.
func TestFromLeverStripsTheRequiredGlyph(t *testing.T) {
	for _, id := range []string{"name", "resume", "cards[115d9079][field0]"} {
		got := leverField(t, parseLever(t), id)
		if strings.ContainsRune(got.Label, '✱') {
			t.Errorf("%s label = %q, want the marker off the text", id, got.Label)
		}
		if !got.Required {
			t.Errorf("%s required = false, want the marker read as the flag", id)
		}
	}
}

// A block with nothing to answer is a note from the employer, not a question.
func TestFromLeverSkipsABlockWithNoControl(t *testing.T) {
	for _, f := range parseLever(t).Fields {
		if f.Label == "Nothing to answer here" {
			t.Errorf("captured %+v, want a block with no control skipped", f)
		}
	}
}

// The hidden partner of a consent checkbox carries the unchecked value and is not a
// second question; the pair is one control.
func TestFromLeverFoldsTheConsentPair(t *testing.T) {
	form := parseLever(t)

	seen := 0
	for _, f := range form.Fields {
		if f.ID == "consent[store]" {
			seen++
		}
	}
	if seen != 1 {
		t.Errorf("consent produced %d controls, want 1", seen)
	}
}

// The marketing-consent block carries no application-label at all: the text a candidate
// reads sits beside the checkbox instead. Without a fallback the control is captured
// with an empty label, which reached production as a stray comma at the end of the
// standard-fields line.
func TestFromLeverLabelsAConsentBoxFromItsOwnText(t *testing.T) {
	// Two tenants, two different wrappers around the same idea — the first fix read one
	// of them and left the other unnamed on production. The rule that covers both is the
	// enclosing <label>, which is what makes the text a label in the first place.
	for _, tc := range []struct{ id, want string }{
		{"consent[marketing]", "Yes, Acme can contact me about future roles for up to 1 year"},
		{"consent[notify]", "Yes, notify me about future roles that match my profile."},
	} {
		got := leverField(t, parseLever(t), tc.id)
		if got.Label != tc.want {
			t.Errorf("%s label = %q, want %q", tc.id, got.Label, tc.want)
		}
		if got.Type != TypeBoolean {
			t.Errorf("%s type = %q, want %q", tc.id, got.Type, TypeBoolean)
		}
	}
}
