package telegram

import "regexp"

// vacancyMarkers matches signals that a post plausibly advertises a job: hiring
// verbs and nouns (RU + EN), role-seeking phrasing, salary amounts, and apply
// cues. Deliberately permissive — the filter's job is only to spare the LLM from
// posts that are clearly not vacancies (memes, digests, course ads); the LLM is
// the real classifier and may still return zero vacancies.
var vacancyMarkers = regexp.MustCompile(`(?i)` +
	// RU hiring verbs/nouns
	`ваканси|ищем|ищут|требуетс|нужен|нужна|нужны|наним|набираем|` +
	`зарплат|оклад|на руки|стажировк|резюме|` +
	// EN hiring
	`hiring|vacanc|looking for|join (our|the) team|apply|salary|` +
	// UA hiring verbs/nouns. Ukrainian "вакансія" does NOT match the RU "ваканси"
	// above — Cyrillic і (U+0456) and и (U+0438) are distinct runes. Each stem is the
	// shortest that covers its inflections ("шукає" also covers "шукаємо"). Excluded
	// after scoring 306 live posts: "наймаємо" and "зарплатн" never fired (the RU
	// "зарплат" already prefixes the latter), and "потрібен"/"відгук" matched as much
	// editorial content as hiring.
	`вакансі|шукає|запрошуємо|стажуванн|досвід роботи|` +
	// salary amounts: "250 000 руб", "60 000 грн", "$120k", "120k-200k", "€80k"
	`\d[\d\s]{2,}\s*(руб|₽|грн|₴|€|\$)|[$€£]\s?\d+\s?k|\d+\s?k\s*[-–—]\s*\$?\d+\s?k`)

// LooksLikeVacancy reports whether a post should enter the extraction queue.
// Posts that fail are still stored (so re-crawls skip them) but marked done with
// zero vacancies and never sent to the LLM.
func LooksLikeVacancy(text string) bool {
	return vacancyMarkers.MatchString(text)
}
