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
	// above — Cyrillic і (U+0456) and и (U+0438) are distinct runes. "шукає" covers
	// "шукаємо"/"шукаєте" but deliberately not "шукаю", the job-SEEKER form. Excluded
	// after scoring live posts: "наймаємо" and "зарплатн" never fired (the RU "зарплат"
	// already prefixes the latter), and "потрібен"/"відгук" matched as much editorial
	// content as hiring.
	//
	// Hryvnia amounts are deliberately NOT a marker, unlike руб/₽/€/$. The currency is
	// low-denomination, so the amount pattern's 3-digit floor cannot separate a salary
	// from a conference ticket: on the Ukrainian cohort every post it admitted on its
	// own was an event ticket, a fundraiser, or a raffle, and none was a vacancy.
	`вакансі|шукає|запрошуємо|стажуванн|досвід роботи|` +
	// Spanish, and deliberately the LABELLED FIELD rather than the bare noun. Measured
	// 2026-09-23 over the 15,203 posts this filter had rejected to date: `empresa:` matched
	// 6,335 of the 6,340 rejected posts of the one Spanish channel and NOTHING in any other
	// channel, while bare `empresa` bought 2 more of them and admitted 4 posts elsewhere.
	// The channel's vacancies are a bot template ("Empresa: <name>", "Ubicación: <place>");
	// its humans use the same word in ordinary sentences ("no voy a bloquear a esa empresa
	// en el bot") and even "contratando" while advertising nothing, so the colon is what
	// separates the advertisement from the conversation.
	//
	// `ubicación:` is NOT a second alternative: it fired on 5,655 of those posts and never
	// once without `empresa:` beside it, so it would add nothing but a line to keep in step
	// — the same reason the Ukrainian "наймаємо" above was dropped after scoring.
	`empresa\s*:|` +
	// salary amounts: "250 000 руб", "$120k", "120k-200k", "€80k"
	`\d[\d\s]{2,}\s*(руб|₽|€|\$)|[$€£]\s?\d+\s?k|\d+\s?k\s*[-–—]\s*\$?\d+\s?k`)

// LooksLikeVacancy reports whether a post's TEXT holds a hiring marker. It is half of
// the admission rule — see AdmitsPost, which is what callers should ask.
func LooksLikeVacancy(text string) bool {
	return vacancyMarkers.MatchString(text)
}

// AdmitsPost reports whether a post should enter the extraction queue: its text carries a
// marker, or it links out to a vacancy a destination adapter can resolve (so a link-out
// digest is not dropped before the extractor can follow it). A nil matcher means no
// registry is configured and only the text can admit.
//
// Posts this refuses are still stored — so re-crawls skip them — but recorded as done with
// zero vacancies and never sent to the LLM.
//
// It is a function rather than a line inside CrawlRunner because it now has TWO readers:
// the crawl, and cmd/backfill-telegram-prefilter, which re-offers already-stored posts
// after the markers widen. Those two disagreeing is not a cosmetic drift — the backfill
// would requeue posts the next crawl refuses, or leave behind the ones it now admits.
func AdmitsPost(text string, links []Link, m LinkMatcher) bool {
	if LooksLikeVacancy(text) {
		return true
	}
	return m != nil && m.Matches(links)
}
