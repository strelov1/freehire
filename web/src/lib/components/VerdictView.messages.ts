import { defineMessages, plurals } from '$lib/i18n/t';

// `gap.name`, `skill.name` and `b.label` (bundle name) are all market/profile data —
// skill and bundle names, never catalog text. `skill.advice` is composed server-side
// (an LLM explanation) and stays English per the project's established boundary:
// strings the server writes are a backend change, not a catalog one.
//
// `skill.status` is the one wire token this view prints: `strong`/`hidden`/
// `adjacent`/`missing` from internal/job/verdict/verdict.go. Mapped via `tokenLabel`
// so a status this catalog doesn't know about still falls through to the raw token.
export const messages = defineMessages(
  {
    coverageLine: plurals({
      one: '{covered} of {total} open vacancy',
      other: '{covered} of {total} open vacancies',
    }),
    outOfReach: plurals({
      one: '{count} vacancy out of reach',
      other: '{count} vacancies out of reach',
    }),
    coverageExplanation:
      'Vacancies for this role that mention at least one of your skills. Add the skills below to reach more.',
    mustHaveCovered: 'Must-have skills covered',
    mustHaveNone:
      "No single skill appears in the majority of this role's vacancies, so there's no must-have to measure.",
    stackMatch: 'Stack match',
    coherence: 'Coherence',
    bundlesHeading: 'Skill bundles the market expects',
    tabs: {
      addSkill: 'Add a skill',
      topSkills: 'Top market skills',
    },
    noGaps: 'No in-demand skills left to add for this role — your stack already reaches its open vacancies.',
    unlockPercent: '+{pct}% of the role',
    mustHaveBadge: 'Must-have',
    marketFrequency: '{pct}% of roles',
    status: {
      strong: 'In your CV',
      hidden: 'Implied',
      adjacent: 'Related skill',
      missing: 'Missing',
    },
  },
  {
    ru: {
      coverageLine: plurals({
        one: '{covered} из {total} открытой вакансии',
        few: '{covered} из {total} открытых вакансий',
        many: '{covered} из {total} открытых вакансий',
        other: '{covered} из {total} открытых вакансий',
      }),
      outOfReach: plurals({
        one: '{count} недосягаемая вакансия',
        few: '{count} недосягаемые вакансии',
        many: '{count} недосягаемых вакансий',
        other: '{count} недосягаемые вакансии',
      }),
      coverageExplanation:
        'Вакансии по этой роли, в которых упоминается хотя бы один ваш навык. Добавьте навыки ниже, чтобы охватить больше.',
      mustHaveCovered: 'Обязательных навыков закрыто',
      mustHaveNone:
        'Ни один навык не встречается в большинстве вакансий этой роли, поэтому обязательных навыков для оценки нет.',
      stackMatch: 'Совпадение стека',
      coherence: 'Согласованность',
      bundlesHeading: 'Сочетания навыков, которые ждёт рынок',
      tabs: {
        addSkill: 'Добавить навык',
        topSkills: 'Топ навыков рынка',
      },
      noGaps: 'Для этой роли больше нет востребованных навыков, которые стоило бы добавить — ваш стек уже покрывает её открытые вакансии.',
      unlockPercent: '+{pct}% от роли',
      mustHaveBadge: 'Обязательный',
      marketFrequency: '{pct}% вакансий',
      status: {
        strong: 'В резюме',
        hidden: 'Скрыт',
        adjacent: 'Смежный',
        missing: 'Нет',
      },
    },
  },
);
