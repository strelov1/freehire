package skilltag

import (
	"regexp"
	"strings"
)

var languageWorkLanguages = []string{
	"bengali", "filipino", "indonesian", "marathi", "spanish", "tagalog", "tamil", "telugu", "thai",
}

// These phrases describe work or fluency in a language. A language name on its
// own can name a location, a market or a team, so it only resolves as an explicit
// skill token via Canonicalize.
var languageWorkPhraseAliases = func() []phraseAlias {
	aliases := []phraseAlias{
		{"linguistic qa", "linguistic-qa"},
		{"linguistic quality assurance", "linguistic-qa"},
		{"linguistic quality evaluator", "linguistic-qa"},
		{"linguistic evaluator", "linguistic-qa"},
		{"linguistic ai auditor", "linguistic-qa"},
		{"linguistic tester", "linguistic-qa"},
		{"language quality specialist", "linguistic-qa"},
		{"localization qa", "linguistic-qa"},
		{"localisation qa", "linguistic-qa"},
		{"localization testing", "linguistic-qa"},
		{"localisation testing", "linguistic-qa"},
		{"audio transcription", "audio-transcription"},
		{"speech transcription", "audio-transcription"},
		{"transcriptionist", "audio-transcription"},
		{"data annotation", "data-annotation"},
		{"annotation/labelling", "data-annotation"},
		{"data annotator", "data-annotation"},
		{"data labeling", "data-annotation"},
		{"data labelling", "data-annotation"},
		{"data labeler", "data-annotation"},
		{"data labeller", "data-annotation"},
		{"data ai annotator", "data-annotation"},
		{"ai annotator", "data-annotation"},
		{"ai video annotator", "data-annotation"},
		{"ai video qc annotator", "data-annotation"},
		{"ad rater indonesian speakers", "indonesian"},
		{"indonesian qa rater", "indonesian"},
		{"native language specialist indonesian", "indonesian"},
		{"proficiency in english and indonesian", "indonesian"},
		{"ad rater thai speakers", "thai"},
		{"korean to thai", "thai"},
		{"thai voice acting", "thai"},
		{"translator thai", "thai"},
	}
	for _, language := range languageWorkLanguages {
		aliases = append(aliases,
			phraseAlias{"native " + language + " speaker", language},
			phraseAlias{"fluent in " + language, language},
			phraseAlias{"fluency in " + language, language},
			phraseAlias{language + " proficiency", language},
			phraseAlias{language + " language proficiency", language},
			phraseAlias{language + " language specialist", language},
			phraseAlias{language + " language expert", language},
			phraseAlias{language + " audio transcription", language},
			phraseAlias{language + " speech translation", language},
			phraseAlias{"english into " + language, language},
		)
	}
	return aliases
}()

var languageWorkTitleMatchers = compilePhraseMatchers(languageWorkPhraseAliases)

// A title that names a language task can put its working language after the
// role, as in "Audio Transcription Adjudicator - Spanish (Spain)".
var languageWorkTitleSuffix = regexp.MustCompile(`\s[-–—:]\s*(` +
	strings.Join(languageWorkLanguages, "|") + `)(?:\s*\([^()]*\))?$`)

func tagLanguageWorkTitle(title string, tags map[string]struct{}) {
	title = normalize(title)
	for _, matcher := range languageWorkTitleMatchers {
		if matcher.matches(title) {
			tags[matcher.canonical] = struct{}{}
		}
	}
	for _, task := range []string{"audio-transcription", "data-annotation", "linguistic-qa"} {
		if _, ok := tags[task]; ok {
			if match := languageWorkTitleSuffix.FindStringSubmatch(title); match != nil {
				tags[match[1]] = struct{}{}
			}
			return
		}
	}
}
