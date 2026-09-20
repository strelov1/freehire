import { defineMessages } from '$lib/i18n/t';

// `status` mirrors the API's `EmployerAccount['status']` enum, rendered verbatim as a
// status pill's label — the same reason MySubmissionsView keeps its own status map rather
// than printing the wire value.
export const messages = defineMessages(
  {
    headTitle: 'Employer — freehire',
    signedOut: 'Sign in to manage your company.',
    loadError: "Couldn't load your employer account.",

    // Claim step: no account yet.
    claimTitle: 'Manage your company on freehire',
    claimIntro:
      'Claim your company to edit its profile and publish your own vacancies directly — no crawler, no waiting for a moderator to hand-enter them.',
    companyNameLabel: 'Company name',
    companyNamePlaceholder: 'Acme',
    workEmailLabel: 'Work email',
    workEmailPlaceholder: 'you@acme.com',
    workEmailHint: 'A public email provider (Gmail, Outlook, …) is not accepted — use your company address.',
    claimSubmit: 'Claim company',
    claiming: 'Claiming…',

    // Confirm step: code just mailed.
    confirmTitle: 'Confirm your work email',
    confirmIntro: 'We mailed a 6-digit code to {email}. Enter it below to confirm.',
    codeLabel: 'Code',
    confirmSubmit: 'Confirm',
    confirming: 'Confirming…',

    // Pending review (code confirmed, or already pending on reload).
    pendingTitle: 'Verification pending',
    pendingBody:
      "We couldn't automatically match your work email to {company}'s known domain, so a moderator is reviewing your claim. This is usually quick.",

    // Revoked.
    revokedTitle: 'This claim was revoked',
    revokedBody: 'An administrator revoked access to {company}. Contact support if you believe this is a mistake.',

    // Dashboard shell.
    dashboardTitle: '{company}',
    tabProfile: 'Profile',
    tabVacancies: 'Vacancies',

    // Profile tab.
    profileSaved: 'Saved.',
    profileSaveError: 'Could not save. Please try again.',
    profileTaglineLabel: 'Tagline',
    profileTaglinePlaceholder: 'A one-line description of what you do',
    profileDescriptionLabel: 'Description',
    profileWebsiteLabel: 'Website',
    profileIndustriesLabel: 'Industries',
    profileYearFoundedLabel: 'Year founded',
    profileEmployeeCountLabel: 'Employee count',
    profileHqCountryLabel: 'HQ country (ISO code)',
    profileSubindustryLabel: 'Subindustry',
    profileSave: 'Save profile',
    profileSaving: 'Saving…',

    // Vacancies tab.
    vacanciesLoadError: "Couldn't load your vacancies.",
    vacanciesEmpty: 'No vacancies yet — publish your first one below.',
    newVacancy: 'New vacancy',
    editVacancy: 'Edit',
    closeVacancy: 'Close',
    closing: 'Closing…',
    closedLabel: 'closed',
    formTitleNew: 'Publish a vacancy',
    formTitleEdit: 'Edit vacancy',
    fieldUrl: 'Apply URL',
    fieldUrlHint: 'Where a candidate applies — your careers page, ATS link, or a mailto:.',
    fieldTitle: 'Title',
    fieldLocation: 'Location',
    fieldRemote: 'Remote',
    fieldDescription: 'Description',
    fieldWorkMode: 'Work mode',
    fieldEmploymentType: 'Employment type',
    fieldSeniority: 'Seniority',
    fieldSkills: 'Skills',
    publish: 'Publish',
    publishing: 'Publishing…',
    saveChanges: 'Save changes',
    savingChanges: 'Saving…',
    cancel: 'Cancel',
    vacancyError: 'Could not save the vacancy. Please try again.',
    urlTakenError: 'This URL is already used by another employer’s vacancy.',
  },
  {
    ru: {
      headTitle: 'Работодатель — freehire',
      signedOut: 'Войдите, чтобы управлять компанией.',
      loadError: 'Не удалось загрузить данные аккаунта работодателя.',

      claimTitle: 'Управляйте своей компанией на freehire',
      claimIntro:
        'Заявите права на компанию, чтобы редактировать её профиль и публиковать свои вакансии напрямую — без краулера и без ожидания модератора.',
      companyNameLabel: 'Название компании',
      companyNamePlaceholder: 'Acme',
      workEmailLabel: 'Рабочий email',
      workEmailPlaceholder: 'you@acme.com',
      workEmailHint: 'Публичные почтовые сервисы (Gmail, Outlook и т.п.) не принимаются — используйте корпоративный адрес.',
      claimSubmit: 'Заявить компанию',
      claiming: 'Отправка…',

      confirmTitle: 'Подтвердите рабочий email',
      confirmIntro: 'Мы отправили 6-значный код на {email}. Введите его ниже.',
      codeLabel: 'Код',
      confirmSubmit: 'Подтвердить',
      confirming: 'Подтверждение…',

      pendingTitle: 'Заявка на проверке',
      pendingBody:
        'Не удалось автоматически сопоставить ваш рабочий email с известным доменом компании «{company}», поэтому заявку проверяет модератор. Обычно это быстро.',

      revokedTitle: 'Доступ отозван',
      revokedBody: 'Администратор отозвал доступ к компании «{company}». Если это ошибка, обратитесь в поддержку.',

      dashboardTitle: '{company}',
      tabProfile: 'Профиль',
      tabVacancies: 'Вакансии',

      profileSaved: 'Сохранено.',
      profileSaveError: 'Не удалось сохранить. Попробуйте ещё раз.',
      profileTaglineLabel: 'Краткое описание',
      profileTaglinePlaceholder: 'Одна строка о том, чем вы занимаетесь',
      profileDescriptionLabel: 'Описание',
      profileWebsiteLabel: 'Сайт',
      profileIndustriesLabel: 'Индустрии',
      profileYearFoundedLabel: 'Год основания',
      profileEmployeeCountLabel: 'Количество сотрудников',
      profileHqCountryLabel: 'Страна головного офиса (код ISO)',
      profileSubindustryLabel: 'Субиндустрия',
      profileSave: 'Сохранить профиль',
      profileSaving: 'Сохранение…',

      vacanciesLoadError: 'Не удалось загрузить вакансии.',
      vacanciesEmpty: 'Пока нет вакансий — опубликуйте первую ниже.',
      newVacancy: 'Новая вакансия',
      editVacancy: 'Изменить',
      closeVacancy: 'Закрыть',
      closing: 'Закрытие…',
      closedLabel: 'закрыта',
      formTitleNew: 'Публикация вакансии',
      formTitleEdit: 'Редактирование вакансии',
      fieldUrl: 'Ссылка для отклика',
      fieldUrlHint: 'Куда попадёт кандидат — ваша страница вакансий, ссылка ATS или mailto:.',
      fieldTitle: 'Название',
      fieldLocation: 'Локация',
      fieldRemote: 'Удалённо',
      fieldDescription: 'Описание',
      fieldWorkMode: 'Формат работы',
      fieldEmploymentType: 'Тип занятости',
      fieldSeniority: 'Уровень',
      fieldSkills: 'Навыки',
      publish: 'Опубликовать',
      publishing: 'Публикация…',
      saveChanges: 'Сохранить изменения',
      savingChanges: 'Сохранение…',
      cancel: 'Отмена',
      vacancyError: 'Не удалось сохранить вакансию. Попробуйте ещё раз.',
      urlTakenError: 'Эта ссылка уже используется вакансией другого работодателя.',
    },
  },
);
