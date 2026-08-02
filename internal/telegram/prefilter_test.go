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
		// Ukrainian. None of these carry a RU or EN marker: "вакансія" cannot match the
		// RU "ваканси" (і and и are distinct runes), and "зарплат" is kept out of the
		// salary case so it rests on the hryvnia amount alone.
		{"ua marker вакансія", "Вакансія: Golang розробник у продуктову команду"},
		{"ua marker шукаємо", "Шукаємо QA Engineer у команду"},
		{"ua experience + hryvnia amount", "Досвід роботи від 2 років, 60 000 грн на місяць"},
		{"ua internship", "Запрошуємо на стажування студентів"},
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
	}
	for _, tc := range reject {
		t.Run("reject/"+tc.name, func(t *testing.T) {
			if LooksLikeVacancy(tc.text) {
				t.Errorf("let through an obvious non-vacancy: %q", tc.text)
			}
		})
	}
}
