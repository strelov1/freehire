import { defineMessages, plurals } from '$lib/i18n/t';

// `draft.subject`/`draft.body`/`draft.recipient*` are server-composed (the actual
// outreach draft), not catalog text — only the dialog's own chrome is translated.
export const messages = defineMessages(
  {
    loadFailed: "Couldn't build the follow-up draft.",
    recordFailed: "Couldn't record the follow-up — your draft is unaffected.",
    copyFailed: "Couldn't copy to the clipboard.",
    thisApplication: 'This application',
    heading: 'Follow up',
    noReply: plurals({ one: 'no reply for {days} day', other: 'no reply for {days} days' }),
    closeDialogAria: 'Close dialog',
    closeAria: 'Close',
    building: 'Building your draft…',
    to: 'To',
    subject: 'Subject',
    privacyNote: 'Send it from your own mail — freehire never writes to anyone on your behalf.',
    openIn: 'Open in',
    mailApp: 'Mail app',
    close: 'Close',
    copied: 'Copied',
    copyDraft: 'Copy draft',
  },
  {
    ru: {
      loadFailed: 'Не удалось подготовить письмо для напоминания.',
      recordFailed: 'Не удалось записать напоминание — черновик не пострадал.',
      copyFailed: 'Не удалось скопировать в буфер обмена.',
      thisApplication: 'Эта заявка',
      heading: 'Напомнить о себе',
      noReply: plurals({
        one: 'нет ответа {days} день',
        few: 'нет ответа {days} дня',
        many: 'нет ответа {days} дней',
        other: 'нет ответа {days} дня',
      }),
      closeDialogAria: 'Закрыть диалог',
      closeAria: 'Закрыть',
      building: 'Готовим черновик…',
      to: 'Кому',
      subject: 'Тема',
      privacyNote: 'Отправьте это со своей почты — freehire никогда не пишет от вашего имени.',
      openIn: 'Открыть в',
      mailApp: 'почтовом приложении',
      close: 'Закрыть',
      copied: 'Скопировано',
      copyDraft: 'Скопировать черновик',
    },
  },
);
