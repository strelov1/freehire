package classify

import (
	"slices"
	"testing"
)

func TestCategoriesPreservesPhraseBoundariesWhenCollapsingSpaces(t *testing.T) {
	if got := Categories("AI Linguistic  Quality Evaluator - Tamil (Singapore)"); !slices.Contains(got, "qa") {
		t.Errorf("Categories() = %v, want qa", got)
	}
	if got := Categories("Linguistic  \nQuality Evaluator"); slices.Contains(got, "qa") {
		t.Errorf("Categories() = %v, want no cross-line alias", got)
	}
}

func TestLanguageWorkCategory(t *testing.T) {
	tests := []struct {
		title string
		want  string
	}{
		{"AI Linguistic Evaluator - Marathi", "qa"},
		{"AI Linguistic  Quality Evaluator - Tamil (Singapore)", "qa"},
		{"Language Quality Specialist (CDMX)", "qa"},
		{"Linguist – Malagasy – UI Technical / Marketing – Remote", "technical_writing"},
		{"English into Albanian Financial/Banking freelance linguists", "technical_writing"},
		{"Afrikaans Language Specialist - AI Trainer", "technical_writing"},
		{"Audio Transcription Adjudicator V2 - Spanish (Spain)", "technical_writing"},
		{"Northern Uzbek Linguists Needed | Remote Transcription Project", "technical_writing"},
		{"Senior German Data Annotator", "operations"},
		{"Remote Data Labeling Specialist for AI Training", "operations"},
		{"Data Labeller - AI Training (Remote)", "operations"},
		{"Generative AI Annotator - Flexible Hours", "operations"},
		{"Tala_Annotation/Labelling_tl_PH-zh_CN", "operations"},
		{"AI Benchmark Engineer | Native Language Specialist - Dutch - Remote", "ml_ai"},
		{"Thai Voice Acting Specialist - AI Trainer", "creative"},
		{"Research Scientist, Speech & Audio", "ml_ai"},
		{"Account Executive (Localization Industry)", "sales"},
		{"Computer Vision Engineer - Data Annotation Tools", "ml_ai"},
		{"CAD Engineer - AI Trainer", ""},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			if got := Parse(tt.title).Category; got != tt.want {
				t.Errorf("category = %q, want %q", got, tt.want)
			}
		})
	}
}
