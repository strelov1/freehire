import { defineMessages } from '#lib/i18n/t.js';

export const messages = defineMessages(
  {
    heading: 'Screening answers',
    description:
      'Answer these once and the browser extension can fill them into matching questions on real application forms.',
    fields: {
      authorizedCountries: 'Authorized to work in (comma-separated countries)',
      authorizedCountriesPlaceholder: 'United States, Germany',
      visaSponsorship: 'Need visa sponsorship?',
      relocate: 'Willing to relocate?',
      age18: '18 or older?',
      privacyConsent: "Accept employers' privacy terms for me?",
      privacyConsentHint:
        'Auto-apply ticks the required privacy / personal-data consent on application forms. It never opts you in to marketing, SMS or future-openings lists.',
      noticePeriod: 'Notice period (days)',
      desiredSalaryAmount: 'Desired salary amount',
      currency: 'Currency (ISO 4217)',
      salaryPeriod: 'Salary period',
    },
    triState: {
      notStated: 'Not stated',
      yes: 'Yes',
      no: 'No',
    },
    periods: {
      year: 'Year',
      month: 'Month',
      day: 'Day',
      hour: 'Hour',
    },
    save: 'Save screening answers',
    saved: 'Screening answers saved.',
    saveFailed: 'Could not save screening answers.',
    mustBeNumber: '{field} must be a number.',
  },
  {
    ru: {
      heading: 'Ответы на вопросы анкеты',
      description:
        'Заполните один раз — расширение браузера подставит эти ответы в похожие вопросы на реальных формах заявки.',
      fields: {
        authorizedCountries: 'Есть разрешение на работу в (страны через запятую)',
        authorizedCountriesPlaceholder: 'США, Германия',
        visaSponsorship: 'Нужна визовая поддержка?',
        relocate: 'Готовы к переезду?',
        age18: '18 лет или больше?',
        privacyConsent: 'Принимать за меня политику конфиденциальности работодателя?',
        privacyConsentHint:
          'Автоотклик ставит обязательную галочку согласия на обработку персональных данных. На рассылки, SMS и «будущие вакансии» он вас не подписывает.',
        noticePeriod: 'Срок уведомления (дней)',
        desiredSalaryAmount: 'Желаемая зарплата',
        currency: 'Валюта (ISO 4217)',
        salaryPeriod: 'Период выплаты',
      },
      triState: {
        notStated: 'Не указано',
        yes: 'Да',
        no: 'Нет',
      },
      periods: {
        year: 'Год',
        month: 'Месяц',
        day: 'День',
        hour: 'Час',
      },
      save: 'Сохранить ответы',
      saved: 'Ответы сохранены.',
      saveFailed: 'Не удалось сохранить ответы.',
      mustBeNumber: '«{field}» должно быть числом.',
    },
  },
);
