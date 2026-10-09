import { defineMessages, plurals } from '$lib/i18n/t';

// `{label}` is the skill's own display name (from `skillLabel()`, a data lookup over
// the user's profile skills) — it is interpolated via `format()`, never translated.
export const messages = defineMessages(
  {
    headTitle: '{label} · Market pulse — freehire',
    signedOut: 'Sign in to view your market pulse.',
    backLink: 'Market pulse',
    loadError: "Couldn't load this skill's trend.",
    noTrend: {
      title: 'No trend for "{label}"',
      body: "Either it isn't one of your profile skills, or it hasn't shown up in an open role yet.",
      button: 'Back to market pulse',
    },
    // The count itself renders in its own larger span; this is the bare noun, declined
    // by `plural()` against that count — see PlanView.messages.ts's `model call(s)`.
    openRoles: plurals({ one: 'open role', other: 'open roles' }),
    demandAriaLabel: '{label} demand over the retained history',
    onlyOneSnapshot: 'Only one snapshot so far — check back next week for a trend line.',
  },
  {
    ru: {
      headTitle: '{label} · Пульс рынка — freehire',
      signedOut: 'Войдите, чтобы увидеть свой пульс рынка.',
      backLink: 'Пульс рынка',
      loadError: 'Не удалось загрузить тренд по этому навыку.',
      noTrend: {
        title: 'Нет тренда для «{label}»',
        body: 'Либо это не навык из вашего профиля, либо он ещё не встречался в открытых вакансиях.',
        button: 'Назад к пульсу рынка',
      },
      openRoles: plurals({
        one: 'открытая вакансия',
        few: 'открытые вакансии',
        many: 'открытых вакансий',
        other: 'открытые вакансии',
      }),
      demandAriaLabel: '{label}: спрос за всю сохранённую историю',
      onlyOneSnapshot: 'Пока есть только один снимок — загляните через неделю, когда появится тренд.',
    },
  },
);
