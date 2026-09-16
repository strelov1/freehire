package atsapply

import "testing"

// A live Ashby posting (Leaply, 2026-09-16) parked with its résumé upload unresolved:
// "file uploads other than the résumé are not resolved by this package". It WAS the résumé.
// Ashby keys its built-in fields as "_systemfield_resume", and the employer had labelled it
// in Ukrainian — so neither the exact-id check nor the English-only label check matched, and
// an application that had everything else it needed could not attach a CV.
func TestIsResumeField_RecognisesAshbysOwnFieldAndNonEnglishLabels(t *testing.T) {
	cases := []struct {
		name  string
		field MergedField
	}{
		{"ashby system field", MergedField{ID: "_systemfield_resume", Label: "Резюме"}},
		{"ashby system field, English label", MergedField{ID: "_systemfield_resume", Label: "Resume"}},
		{"cyrillic label", MergedField{ID: "q_1", Label: "Резюме"}},
		{"label is just CV", MergedField{ID: "q_2", Label: "CV"}},
		{"label names a CV upload", MergedField{ID: "q_3", Label: "Завантажте своє CV"}},
		{"plain id", MergedField{ID: "resume", Label: ""}},
		{"English label", MergedField{ID: "q_4", Label: "Resume/CV"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !isResumeField(tc.field) {
				t.Errorf("isResumeField(%+v) = false, want true", tc.field)
			}
		})
	}
}

// Everything else stays unresolved. A cover letter or a portfolio is a file this package
// genuinely cannot produce, and calling one a résumé would upload the CV into it.
func TestIsResumeField_LeavesOtherUploadsAlone(t *testing.T) {
	cases := []MergedField{
		{ID: "_systemfield_coverletter", Label: "Супровідний лист"},
		{ID: "q_5", Label: "Cover letter"},
		{ID: "q_6", Label: "Portfolio"},
		{ID: "q_7", Label: "Мотиваційний лист"},
		// "cv" inside an unrelated word must not count.
		{ID: "q_8", Label: "Recvued documents"},
	}
	for _, f := range cases {
		if isResumeField(f) {
			t.Errorf("isResumeField(%+v) = true, want false", f)
		}
	}
}
