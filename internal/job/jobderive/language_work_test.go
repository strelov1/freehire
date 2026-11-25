package jobderive

import (
	"slices"
	"testing"
)

func TestDeriveLanguageWorkSkills(t *testing.T) {
	tests := []struct {
		title       string
		description string
		want        []string
	}{
		{"Audio Transcription Adjudicator V2 - Spanish (Spain)", "", []string{"audio-transcription", "spanish"}},
		{"AI Linguistic Evaluator - Marathi", "", []string{"linguistic-qa", "marathi"}},
		{"Bengali Audio Transcription QA", "", []string{"audio-transcription", "bengali"}},
		{"Freelance Translator - Tagalog (Philippines)- Medical Benefits", "Language Pair: English into Tagalog (Philippines)", []string{"tagalog"}},
		{"AI Linguistic Evaluator - Marathi", "Applicants must be fluent in Marathi.", []string{"linguistic-qa", "marathi"}},
		{"Tagalog Speech Translation Subjective Evaluator", "", []string{"tagalog"}},
		{"Tala_Annotation/Labelling_tl_PH-zh_CN", "We are recruiting for a Tagalog Speech Translation Subjective Evaluator.", []string{"data-annotation", "tagalog"}},
		{"Filipino Language Specialist - AI Trainer", "Filipino language proficiency required.", []string{"filipino"}},
		{"Thai Voice Acting Specialist - AI Trainer", "", []string{"thai"}},
		{"Remote Indonesian QA Rater – AI & Search Quality", "", []string{"indonesian"}},
		{"AI Benchmark Engineer | Native Language Specialist - Indonesian - Remote", "", []string{"indonesian"}},
		{"Personalized Internet Ads Assessor", "Full professional proficiency in English and Indonesian language.", []string{"indonesian"}},
		{"Korean to Thai Game Translator", "", []string{"thai"}},
		{"Remote Data Labeling Specialist for AI Training", "Experience with data annotation is preferred.", []string{"data-annotation"}},
		{"Localization QA Tester", "", []string{"linguistic-qa"}},
		{"Data Annotator - Filipino Team", "", []string{"data-annotation"}},
		{"Sales Lead - Spanish (Spain)", "", nil},
		{"Regional Account Manager", "The Filipino team works with Spanish clients based near Tagalog Street. We react quickly to change.", nil},
		{"Content Coordinator", "We do audio transcription and react quickly to change.", []string{"audio-transcription"}},
		{"Software Engineer", "We process telemetry.", nil},
	}
	for _, tt := range tests {
		t.Run(tt.title, func(t *testing.T) {
			got := Derive(Input{Title: tt.title, Description: tt.description}).Skills
			if !slices.Equal(got, tt.want) {
				t.Errorf("skills = %v, want %v", got, tt.want)
			}
		})
	}
}
