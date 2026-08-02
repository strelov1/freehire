package applyform

import "testing"

// fieldByID finds a captured control, failing the test when it is missing — every
// assertion below is about a specific control, so "not there" is always the real failure.
func fieldByID(t *testing.T, f Form, id string) Field {
	t.Helper()
	for _, got := range f.Fields {
		if got.ID == id {
			return got
		}
	}
	t.Fatalf("no field %q in %d captured fields", id, len(f.Fields))
	return Field{}
}

func hasField(f Form, id string) bool {
	for _, got := range f.Fields {
		if got.ID == id {
			return true
		}
	}
	return false
}

// Recruitee describes the standard fields as three-way flags rather than as questions, so
// the mapper has to turn configuration into controls. name and email carry no flag at all
// — the platform rejects an application without them regardless, as its own validation
// response shows.
func TestFromRecruiteeStandardFields(t *testing.T) {
	form := FromRecruitee(RecruiteeOffer{
		OptionsCV:          "required",
		OptionsPhone:       "required",
		OptionsCoverLetter: "optional",
		OptionsPhoto:       "off",
		OptionsSalutation:  "off",
		OptionsTitle:       "off",
	})

	if form.Provider != "recruitee" {
		t.Errorf("provider = %q, want %q", form.Provider, "recruitee")
	}

	for _, want := range []struct {
		id       string
		typ      FieldType
		required bool
	}{
		{"name", TypeText, true},
		{"email", TypeText, true},
		{"cv", TypeFile, true},
		{"phone", TypeText, true},
		{"cover_letter", TypeFile, false},
	} {
		got := fieldByID(t, form, want.id)
		if got.Type != want.typ {
			t.Errorf("%s type = %q, want %q", want.id, got.Type, want.typ)
		}
		if got.Required != want.required {
			t.Errorf("%s required = %v, want %v", want.id, got.Required, want.required)
		}
	}

	// A flag set to "off" means the employer switched the control off — capturing it
	// would tell a candidate to prepare something nobody is going to ask for.
	for _, id := range []string{"photo", "salutation", "title"} {
		if hasField(form, id) {
			t.Errorf("captured %q, want it omitted when the platform reports it off", id)
		}
	}
}

func TestFromRecruiteeOpenQuestions(t *testing.T) {
	form := FromRecruitee(RecruiteeOffer{
		OpenQuestions: []RecruiteeQuestion{
			{
				ID: 4252606, Position: 9, Required: true, Kind: "single_choice",
				Body: "What type of contract do you prefer?",
				Options: []RecruiteeQuestionOption{
					{ID: 6540221, Position: 0, Body: "Contract of Employment"},
					{ID: 6540222, Position: 1, Body: "B2B agreement"},
				},
			},
			{ID: 4281763, Position: 1, Required: true, Kind: "boolean", Body: "Do you require visa sponsorship?"},
		},
	})

	t.Run("an enumerated question keeps the platform's option ids", func(t *testing.T) {
		got := fieldByID(t, form, "4252606")
		if got.Type != TypeSelect {
			t.Errorf("type = %q, want %q", got.Type, TypeSelect)
		}
		if got.Label != "What type of contract do you prefer?" {
			t.Errorf("label = %q, want the employer's own wording", got.Label)
		}
		want := []Option{
			{Label: "Contract of Employment", Value: "6540221"},
			{Label: "B2B agreement", Value: "6540222"},
		}
		if len(got.Options) != len(want) {
			t.Fatalf("options = %v, want %v", got.Options, want)
		}
		for i, o := range got.Options {
			if o != want[i] {
				t.Errorf("option %d = %v, want %v (the submit id, not the label)", i, o, want[i])
			}
		}
	})

	t.Run("questions are captured in the order the platform presents them", func(t *testing.T) {
		// The boolean sits at position 1 and the choice at position 9, so the boolean
		// comes first no matter which order the payload happened to list them in.
		var ids []string
		for _, f := range form.Fields {
			if f.RawType != "" && (f.ID == "4252606" || f.ID == "4281763") {
				ids = append(ids, f.ID)
			}
		}
		if len(ids) != 2 || ids[0] != "4281763" {
			t.Errorf("question order = %v, want the lower position first", ids)
		}
	})
}

// The kind vocabulary was measured across ~250 live offers rather than guessed, and the
// dict-only rule applies: a kind outside it yields no normalized type, never a nearest
// guess, and the platform's own word survives so the gap is visible.
func TestFromRecruiteeKindVocabulary(t *testing.T) {
	for _, tc := range []struct {
		kind string
		want FieldType
	}{
		{"string", TypeText},
		{"text", TypeTextarea},
		{"single_choice", TypeSelect},
		{"multi_choice", TypeMultiSelect},
		{"boolean", TypeBoolean},
		{"legal", TypeBoolean},
		{"file", TypeFile},
		{"date", TypeDate},
		{"number", TypeNumber},
		{"salary", TypeNumber},
		{"infobox", TypeInfo},
		{"video", ""},
		{"something_new", ""},
	} {
		form := FromRecruitee(RecruiteeOffer{
			OpenQuestions: []RecruiteeQuestion{{ID: 1, Kind: tc.kind, Body: "Q"}},
		})
		got := fieldByID(t, form, "1")
		if got.Type != tc.want {
			t.Errorf("kind %q -> type %q, want %q", tc.kind, got.Type, tc.want)
		}
		if got.RawType != tc.kind {
			t.Errorf("kind %q -> raw type %q, want it preserved", tc.kind, got.RawType)
		}
	}
}

// An employer who configured nothing still produces a form — the standard fields the
// platform always demands are themselves the answer to "what does applying cost here".
func TestFromRecruiteeBareOfferStillYieldsStandardFields(t *testing.T) {
	form := FromRecruitee(RecruiteeOffer{})

	if !hasField(form, "name") || !hasField(form, "email") {
		t.Errorf("captured %d fields, want at least the always-demanded name and email", len(form.Fields))
	}
}
