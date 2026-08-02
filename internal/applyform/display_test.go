package applyform

import "testing"

func questionTexts(d Display) []string {
	out := make([]string, 0, len(d.Questions))
	for _, q := range d.Questions {
		out = append(out, q.Text)
	}
	return out
}

func findQuestion(t *testing.T, d Display, text string) Question {
	t.Helper()
	for _, q := range d.Questions {
		if q.Text == text {
			return q
		}
	}
	t.Fatalf("no question %q among %v", text, questionTexts(d))
	return Question{}
}

func TestForDisplayShowsTheEmployersQuestionsWhole(t *testing.T) {
	form := Form{
		Provider: "greenhouse",
		Fields: []Field{
			{ID: "first_name", Label: "First Name", Type: TypeText, Required: true},
			{ID: "question_1", Label: "Why did you apply?", Type: TypeTextarea, Required: true},
			{ID: "question_2", Label: "LinkedIn profile", Type: TypeText},
		},
	}

	d := form.ForDisplay()

	if d.Provider != "greenhouse" {
		t.Errorf("provider = %q, want %q", d.Provider, "greenhouse")
	}
	essay := findQuestion(t, d, "Why did you apply?")
	if !essay.Required || essay.Answer != "written answer" {
		t.Errorf("essay = %+v, want required with a written-answer hint", essay)
	}
	optional := findQuestion(t, d, "LinkedIn profile")
	if optional.Required {
		t.Error("LinkedIn profile came back required")
	}
	// The question text is the employer's, shown as published — the whole point of
	// preferring the questions over a summary of them.
	if got := len(d.Questions); got != 2 {
		t.Errorf("questions = %v, want only the employer's two", questionTexts(d))
	}
}

// Name, email, phone and a CV are on every form. One bullet each would pad the list
// with what everyone already expects, so they are stated once — but stated, because
// a form that does NOT want a CV is worth knowing too.
func TestForDisplayCollapsesTheStandardFields(t *testing.T) {
	for _, tc := range []struct {
		provider string
		fields   []Field
		want     []string
	}{
		{
			provider: "greenhouse",
			fields: []Field{
				{ID: "first_name", Label: "First Name", Type: TypeText},
				{ID: "last_name", Label: "Last Name", Type: TypeText},
				{ID: "email", Label: "Email", Type: TypeText},
				{ID: "resume", Label: "Resume/CV", Type: TypeFile},
				{ID: "resume_text", Label: "Resume/CV", Type: TypeTextarea},
			},
			// Greenhouse offers the CV as an upload OR pasted text under one label;
			// the reader should see it once.
			want: []string{"First Name", "Last Name", "Email", "Resume/CV"},
		},
		{
			provider: "recruitee",
			fields: []Field{
				{ID: "name", Label: "Full name", Type: TypeText},
				{ID: "email", Label: "Email", Type: TypeText},
				{ID: "cv", Label: "CV", Type: TypeFile},
				{ID: "phone", Label: "Phone", Type: TypeText},
			},
			want: []string{"Full name", "Email", "CV", "Phone"},
		},
		{
			provider: "ashby",
			fields: []Field{
				{ID: "_systemfield_name", Label: "Name", Type: TypeText},
				{ID: "_systemfield_email", Label: "Email", Type: TypeText},
				{ID: "_systemfield_resume", Label: "Resume", Type: TypeFile},
			},
			want: []string{"Name", "Email", "Resume"},
		},
	} {
		d := Form{Provider: tc.provider, Fields: tc.fields}.ForDisplay()

		if len(d.Questions) != 0 {
			t.Errorf("%s: questions = %v, want none — these are all standard fields",
				tc.provider, questionTexts(d))
		}
		if len(d.Basics) != len(tc.want) {
			t.Errorf("%s: basics = %v, want %v", tc.provider, d.Basics, tc.want)
			continue
		}
		for i, b := range d.Basics {
			if b != tc.want[i] {
				t.Errorf("%s: basics = %v, want %v", tc.provider, d.Basics, tc.want)
				break
			}
		}
	}
}

// The equal-opportunity survey is not the employer's questions. It is a mandated
// survey the platform serves in its own block, always optional and near-identical
// everywhere; listing it would bury the questions a candidate has to prepare for.
func TestForDisplayDropsTheDemographicSurvey(t *testing.T) {
	form := Form{
		Provider: "greenhouse",
		Fields: []Field{
			{ID: "question_1", Label: "Why did you apply?", Type: TypeTextarea},
			{ID: "race", Label: "Race", Type: TypeSelect, Demographic: true},
			{ID: "gender", Label: "Gender", Type: TypeSelect, Demographic: true},
			{ID: "disability_status", Label: "Disability Status", Type: TypeSelect, Demographic: true},
		},
	}

	d := form.ForDisplay()

	if got := questionTexts(d); len(got) != 1 || got[0] != "Why did you apply?" {
		t.Errorf("questions = %v, want only the employer's own", got)
	}
}

// A hidden control and a block of explanatory text are not questions: nobody answers
// either one.
func TestForDisplayDropsNonQuestions(t *testing.T) {
	form := Form{
		Provider: "greenhouse",
		Fields: []Field{
			{ID: "longitude", Label: "Longitude", Type: TypeHidden},
			{ID: "note", Label: "Please read our remote-work policy below.", Type: TypeInfo},
			{ID: "question_1", Label: "Where do you live?", Type: TypeText},
		},
	}

	d := form.ForDisplay()

	if got := questionTexts(d); len(got) != 1 || got[0] != "Where do you live?" {
		t.Errorf("questions = %v, want the hidden control and the info block dropped", got)
	}
}

func TestForDisplayNamesTheAnswerKind(t *testing.T) {
	for _, tc := range []struct {
		kind FieldType
		want string
	}{
		{TypeTextarea, "written answer"},
		{TypeSelect, "choose one"},
		{TypeMultiSelect, "choose any"},
		{TypeBoolean, "yes / no"},
		{TypeFile, "upload"},
		// A one-line answer is the default expectation, so naming it adds noise.
		{TypeText, ""},
		{TypeNumber, ""},
		{TypeDate, ""},
		// A kind the capture could not normalize gets no word rather than a guessed
		// one — the same dict-only rule the capture itself follows. The question is
		// still shown; only the hint about its cost is withheld.
		{"", ""},
	} {
		d := Form{
			Provider: "ashby",
			Fields:   []Field{{ID: "q", Label: "Q", Type: tc.kind, RawType: "whatever"}},
		}.ForDisplay()

		got := findQuestion(t, d, "Q")
		if got.Answer != tc.want {
			t.Errorf("kind %q -> answer %q, want %q", tc.kind, got.Answer, tc.want)
		}
	}
}

// A form asking nothing beyond a CV is itself worth knowing — it is the one-click
// apply a candidate is hunting for.
func TestForDisplayOfAFormWithNoQuestions(t *testing.T) {
	d := Form{
		Provider: "recruitee",
		Fields: []Field{
			{ID: "name", Label: "Full name", Type: TypeText},
			{ID: "email", Label: "Email", Type: TypeText},
			{ID: "cv", Label: "CV", Type: TypeFile},
		},
	}.ForDisplay()

	if len(d.Questions) != 0 {
		t.Errorf("questions = %v, want none", questionTexts(d))
	}
	if len(d.Basics) != 3 {
		t.Errorf("basics = %v, want the three standard fields", d.Basics)
	}
}

// An employer's question keeps its place in the order the platform presented it —
// that order is how the form will actually read.
func TestForDisplayKeepsTheFormsOrder(t *testing.T) {
	d := Form{
		Provider: "ashby",
		Fields: []Field{
			{ID: "q1", Label: "First", Type: TypeText},
			{ID: "_systemfield_email", Label: "Email", Type: TypeText},
			{ID: "q2", Label: "Second", Type: TypeText},
		},
	}.ForDisplay()

	got := questionTexts(d)
	if len(got) != 2 || got[0] != "First" || got[1] != "Second" {
		t.Errorf("questions = %v, want them in the platform's order", got)
	}
}
