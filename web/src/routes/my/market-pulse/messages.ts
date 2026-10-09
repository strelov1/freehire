import { defineMessages } from '$lib/i18n/t';

export const messages = defineMessages(
  {
    headTitle: 'Market pulse — freehire',
    signedOut: 'Sign in to view your market pulse.',
    title: 'Market pulse',
    subtitle: 'How you compare to the live market — role coverage and your own skill-demand trend.',
    tabs: {
      coverage: 'Coverage',
      trend: 'Skill trend',
    },
    tabStripLabel: 'Market pulse sections',
    profileCta: {
      title: 'Complete your profile to see market coverage',
      body: "Coverage compares your CV's skills against the live market for a role you choose — add a profile first.",
      button: 'Go to profile',
    },
    loadError: "Couldn't load the report.",
    filterDescription:
      'Narrow the market to see how it reshapes your CV — pick roles, regions and seniority to compare against.',
  },
  {
    ru: {
      headTitle: 'Пульс рынка — freehire',
      signedOut: 'Войдите, чтобы увидеть свой пульс рынка.',
      title: 'Пульс рынка',
      subtitle: 'Как вы выглядите на фоне живого рынка — покрытие по роли и динамика спроса на ваши навыки.',
      tabs: {
        coverage: 'Покрытие',
        trend: 'Тренд навыков',
      },
      tabStripLabel: 'Разделы пульса рынка',
      profileCta: {
        title: 'Заполните профиль, чтобы увидеть покрытие рынка',
        body: 'Покрытие сравнивает навыки вашего резюме с живым рынком для выбранной роли — сначала добавьте профиль.',
        button: 'Перейти в профиль',
      },
      loadError: 'Не удалось загрузить отчёт.',
      filterDescription:
        'Сузьте рынок, чтобы увидеть, как это меняет оценку вашего резюме — выберите роли, регионы и уровень для сравнения.',
    },
  },
);
