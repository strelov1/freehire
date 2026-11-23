import { defineMessages } from '#lib/i18n/t.js';

export const messages = defineMessages(
  {
    title: 'Hide {company} from Matches?',
    description:
      'Their jobs stop appearing in Matches, in every source. You can bring them back from Companies to avoid in your profile.',
    confirm: 'Hide company',
    full: 'Your Companies to avoid list is full (200). Remove one in your profile, then try again.',
    failed: "Couldn't hide this company. Try again.",
  },
  {
    ru: {
      title: 'Скрыть {company} из подборки?',
      description:
        'Вакансии компании перестанут появляться в подборке, из любого источника. Вернуть её можно в профиле, в «Компании для исключения».',
      confirm: 'Скрыть компанию',
      full: 'Список «Компании для исключения» заполнен (200). Уберите одну в профиле и повторите.',
      failed: 'Не удалось скрыть компанию. Попробуйте ещё раз.',
    },
  },
);
