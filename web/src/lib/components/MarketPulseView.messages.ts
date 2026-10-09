import { defineMessages, plurals } from '$lib/i18n/t';

// `skillLabel(skill.skill)` names the skill itself (profile/market data), never
// catalog text — the catalog here covers only the surrounding UI chrome.
export const messages = defineMessages(
  {
    loadError: "Couldn't load your market pulse.",
    emptyTitle: 'No skill trend yet',
    emptyBody:
      'Add skills to your profile, or check back in a week — a trend needs at least one skill that has shown up in an open role.',
    emptyButton: 'Go to profile',
    searchPlaceholder: 'Find a skill…',
    searchAriaLabel: 'Filter skills',
    noMatch: 'No skill matches "{query}".',
    // The count renders in its own span; this is the bare noun, declined by
    // `plural()` against that count — see PlanView.messages.ts's `model call(s)`.
    openRoles: plurals({ one: 'open role', other: 'open roles' }),
    demandAriaLabel: '{label} demand over the retained history',
  },
  {
    ru: {
      loadError: 'Не удалось загрузить ваш пульс рынка.',
      emptyTitle: 'Пока нет тренда по навыкам',
      emptyBody:
        'Добавьте навыки в профиль или зайдите через неделю — для тренда нужен хотя бы один навык, который встретился в открытой вакансии.',
      emptyButton: 'Перейти в профиль',
      searchPlaceholder: 'Найти навык…',
      searchAriaLabel: 'Фильтр по навыкам',
      noMatch: 'Ничего не найдено по запросу «{query}».',
      openRoles: plurals({
        one: 'открытая вакансия',
        few: 'открытые вакансии',
        many: 'открытых вакансий',
        other: 'открытые вакансии',
      }),
      demandAriaLabel: '{label}: спрос за всю сохранённую историю',
    },
  },
);
