package telegram

import "testing"

func TestLooksLikeVacancy(t *testing.T) {
	pass := []struct{ name, text string }{
		{"ru marker вакансия", "Вакансия: Go разработчик в финтех"},
		{"ru marker ищем", "Ищем SEO-специалиста в криптопроект (удаленно)"},
		{"ru salary", "Платят 250 000 руб на руки, офис в Москве"},
		{"en hiring", "We are hiring a senior backend engineer"},
		{"en salary range", "ML & full-stack engineers, $110k-220k + bonus + equity, London"},
		{"board template", "Senior Fullstack Engineer\n#удаленка #senior\nCompany: RugsDotFun\nSalary: $120k - $200k"},
		{"required experience", "Требуется опыт от 2 лет, стек: Go, Postgres. Резюме в личку"},
		{"recall bias: weak but plausible", "Команде нужен продакт. Подробности у @someone"},
		// Ukrainian. Each case rests on exactly ONE Ukrainian alternative, so removing
		// that alternative fails this case and nothing else. None carries a RU or EN
		// marker: "вакансія" cannot match the RU "ваканси" (і and и are distinct runes).
		{"ua marker вакансія", "Вакансія: Golang розробник у продуктову команду"},
		{"ua marker шукаємо", "Шукаємо QA Engineer у команду"},
		{"ua marker запрошуємо", "Запрошуємо Java-інженера до продуктової команди"},
		{"ua marker стажування", "Оплачуване стажування для студентів, Київ"},
		{"ua marker досвід роботи", "Бекенд-інженер, досвід роботи від 3 років, Львів"},
	}
	for _, tc := range pass {
		t.Run("pass/"+tc.name, func(t *testing.T) {
			if !LooksLikeVacancy(tc.text) {
				t.Errorf("filtered out a plausible vacancy: %q", tc.text)
			}
		})
	}

	reject := []struct{ name, text string }{
		{"meme", "Пятница! Всем хороших выходных 🎉"},
		{"news digest", "Дайджест новостей недели: Яндекс выпустил новую модель, OpenAI снова в суде"},
		{"course ad", "Скидка 50% на курс по Python до конца недели! Успей записаться"},
		{"empty-ish", "🔥🔥🔥"},
		{"ua news digest", "Дайджест новин тижня: Google оновив пошук"},
		{"ua course ad", "Знижка 50% на курс — встигни записатись"},
		// A hryvnia amount is not a hiring signal: the editorial channels price event
		// tickets in the same three-digit range a salary would occupy.
		{"ua event ticket priced in hryvnia", "Встигніть купити квиток на DOU Day Picnic за 500 грн"},
	}
	for _, tc := range reject {
		t.Run("reject/"+tc.name, func(t *testing.T) {
			if LooksLikeVacancy(tc.text) {
				t.Errorf("let through an obvious non-vacancy: %q", tc.text)
			}
		})
	}
}
