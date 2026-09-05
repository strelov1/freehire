import { defineMessages } from '$lib/i18n/t';

// One catalog for the whole Activity section rather than five. Its four tab views
// (`SavedJobs`, `JobHistory`, `AnalysesView`, `Hidden`) carry two or three strings
// each and are reachable from nowhere else, so they are one surface with one
// vocabulary — splitting it per component would spread eight lines over five files
// and make the four empty states drift apart. The route layout and its child pages
// read the same catalog, which is why it lives here rather than under the route:
// a `$lib` component may not import from `routes/`.
export const messages = defineMessages(
  {
    // The layout's base title; each child page overrides it with its own.
    headTitle: 'Activity — freehire',
    title: 'Activity',
    tablistLabel: 'Activity view',
    tabs: {
      saved: 'Saved',
      history: 'History',
      matches: 'Matches',
      hidden: 'Hidden',
    },
    headTitles: {
      saved: 'Saved — Activity — freehire',
      history: 'History — Activity — freehire',
      matches: 'Matches — Activity — freehire',
      hidden: 'Hidden — Activity — freehire',
    },
    saved: {
      loadError: "Couldn't load your saved jobs.",
      empty: 'Nothing saved yet. Jobs you save will show up here.',
    },
    history: {
      loadError: "Couldn't load your history.",
      empty: 'Nothing viewed yet. Jobs you open will show up here.',
    },
    matches: {
      loadError: "Couldn't load your analyses.",
      empty: 'No AI match analyses yet. Open a job and run “Analyse match with AI”.',
      closed: 'Closed',
      stale: 'Stale',
      unknownCompany: 'Unknown company',
    },
    hidden: {
      loadError: "Couldn't load your hidden jobs.",
      empty: 'Nothing hidden. Jobs you hide from the feed show up here.',
      unhide: 'Un-hide',
      unhideTitle: 'Un-hide — show this job in the feed again',
    },
  },
  {
    ru: {
      headTitle: 'Активность — freehire',
      title: 'Активность',
      tablistLabel: 'Раздел активности',
      tabs: {
        saved: 'Сохранённые',
        history: 'История',
        matches: 'Совпадения',
        hidden: 'Скрытые',
      },
      headTitles: {
        saved: 'Сохранённые — Активность — freehire',
        history: 'История — Активность — freehire',
        matches: 'Совпадения — Активность — freehire',
        hidden: 'Скрытые — Активность — freehire',
      },
      saved: {
        loadError: 'Не удалось загрузить сохранённые вакансии.',
        empty: 'Пока пусто. Сохранённые вакансии появятся здесь.',
      },
      history: {
        loadError: 'Не удалось загрузить историю.',
        empty: 'Пока пусто. Вакансии, которые вы открывали, появятся здесь.',
      },
      matches: {
        loadError: 'Не удалось загрузить ваши анализы.',
        empty: 'Анализов пока нет. Откройте вакансию и запустите «Анализ соответствия с AI».',
        closed: 'Закрыта',
        stale: 'Устарел',
        unknownCompany: 'Компания неизвестна',
      },
      hidden: {
        loadError: 'Не удалось загрузить скрытые вакансии.',
        empty: 'Скрытых нет. Вакансии, которые вы скроете из ленты, появятся здесь.',
        unhide: 'Вернуть',
        unhideTitle: 'Вернуть — снова показывать эту вакансию в ленте',
      },
    },
  },
);
