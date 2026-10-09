import { defineMessages } from '$lib/i18n/t';

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
