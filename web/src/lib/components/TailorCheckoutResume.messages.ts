import { defineMessages } from '#lib/i18n/t.js';

export const messages = defineMessages(
  {
    active: '{tier} is active on your account',
    pickUp:
      "Pick up where you left off: the same role and the same CV. Continuing starts one of today's CV editing sessions.",
    continue: 'Continue tailoring',
    notNow: 'Not now',
    workspaceFailed: "Couldn't open the workspace. Your task is saved — try again.",
    checkingTitle: 'Checking your plan…',
    checkingBody: 'This can take a minute after you pay. Your tailoring task is saved.',
    savedTitle: 'Your tailoring task is saved',
    notYet: "Pro isn't showing on your account yet. If you just paid, check again in a moment.",
    noPlan: 'No Pro plan is active on your account. You can try again or go back to the role.',
    checkAgain: 'Check again',
    tryAgain: 'Try Pro again',
    backToRole: 'Back to the role',
    dismiss: 'Dismiss',
    checkoutFailed: 'Checkout is not available right now. Please try again in a moment.',
  },
  {
    ru: {
      active: 'Тариф {tier} подключён',
      pickUp:
        'Продолжите с того же места: та же вакансия и то же CV. Продолжение запустит одну из сегодняшних сессий редактирования CV.',
      continue: 'Продолжить адаптацию',
      notNow: 'Не сейчас',
      workspaceFailed: 'Не удалось открыть редактор. Задача сохранена — попробуйте ещё раз.',
      checkingTitle: 'Проверяем ваш тариф…',
      checkingBody: 'После оплаты это может занять минуту. Задача по адаптации CV сохранена.',
      savedTitle: 'Задача по адаптации CV сохранена',
      notYet: 'Pro пока не появился в аккаунте. Если вы только что оплатили, проверьте ещё раз через минуту.',
      noPlan: 'Pro в аккаунте не подключён. Можно попробовать ещё раз или вернуться к вакансии.',
      checkAgain: 'Проверить ещё раз',
      tryAgain: 'Подключить Pro',
      backToRole: 'Вернуться к вакансии',
      dismiss: 'Скрыть',
      checkoutFailed: 'Оплата сейчас недоступна. Попробуйте ещё раз чуть позже.',
    },
  },
);
