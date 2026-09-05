import { defineMessages, plurals } from '$lib/i18n/t';

// `plan.allowances[].feature` and the invoice/subscription statuses are stable API
// identifiers, so both are MAPPED here rather than printed — an untranslated map
// entry falls through to the raw token, which is the same behaviour the English
// source already had for a feature it did not know.
export const messages = defineMessages(
  {
    headTitle: 'Your plan — freehire',
    signedOut: 'Sign in to view your plan.',
    title: 'Your plan',
    description:
      'Every AI feature is available on every plan. What changes is how much of each you can do in a day — and it starts over every night.',
    planStrip: {
      pro: 'Pro',
      free: 'Free',
      runsUntilPrefix: 'Runs until',
      freeSubtitle: 'Same features, daily limits',
      upgrade: 'Upgrade to Pro',
      manageSubscription: 'Manage subscription',
    },
    subscription: {
      heading: 'Subscription',
      // "$5.00 / month" — the interval comes from the provider as `month`/`year`.
      interval: {
        month: 'month',
        year: 'year',
      },
      status: {
        active: 'Active',
        trialing: 'Trial',
        // Worth spelling out: this is not "cancelled", it is "your card needs
        // attention and you still have access". A subscriber who reads it as
        // cancelled will not fix the card.
        past_due: 'Payment failed — retrying, access continues',
        canceled: 'Cancelled',
        unpaid: 'Unpaid',
      },
      cancelledPrefix: 'Cancelled — access runs until',
      nextChargePrefix: 'Next charge',
      receipt: 'Receipt',
    },
    today: {
      heading: 'Today',
      resetsAtPrefix: 'Resets at',
      unlimited: 'Unlimited',
      features: {
        tailor: 'CV editing sessions',
        match: 'Job analyses',
        assistant: 'Assistant messages',
        dictation: 'Dictation',
      },
    },
    usage: {
      heading: 'AI activity today',
      modelCalls: plurals({ one: 'model call', other: 'model calls' }),
      tokens: plurals({ one: 'token', other: 'tokens' }),
      failedSuffix: 'failed',
      resetsPrefix: 'resets',
      explanation:
        'One message takes several calls — the assistant works in rounds. This counts the work, not what it costs you; the allowances above are what you spend.',
    },
    history: {
      heading: 'Recent activity',
      loadError: "Couldn't load your plan.",
      empty: 'Nothing yet. What you do with the AI features will appear here.',
    },
  },
  {
    ru: {
      headTitle: 'Тариф — freehire',
      signedOut: 'Войдите, чтобы посмотреть свой тариф.',
      title: 'Ваш тариф',
      description:
        'Все AI-функции доступны на любом тарифе. Отличается только то, сколько каждой можно сделать за день — и каждую ночь счётчик обнуляется.',
      planStrip: {
        pro: 'Pro',
        free: 'Free',
        runsUntilPrefix: 'Действует до',
        freeSubtitle: 'Те же функции, дневные лимиты',
        upgrade: 'Перейти на Pro',
        manageSubscription: 'Управление подпиской',
      },
      subscription: {
        heading: 'Подписка',
        interval: {
          month: 'месяц',
          year: 'год',
        },
        status: {
          active: 'Активна',
          trialing: 'Пробный период',
          past_due: 'Платёж не прошёл — повторяем, доступ сохраняется',
          canceled: 'Отменена',
          unpaid: 'Не оплачена',
        },
        cancelledPrefix: 'Отменена — доступ действует до',
        nextChargePrefix: 'Следующее списание',
        receipt: 'Чек',
      },
      today: {
        heading: 'Сегодня',
        resetsAtPrefix: 'Обнуляется в',
        unlimited: 'Без ограничений',
        features: {
          tailor: 'Сессии редактирования CV',
          match: 'Анализы вакансий',
          assistant: 'Сообщения ассистенту',
          dictation: 'Диктовка',
        },
      },
      usage: {
        heading: 'AI-активность сегодня',
        modelCalls: plurals({
          one: 'обращение к модели',
          few: 'обращения к модели',
          many: 'обращений к модели',
          other: 'обращения к модели',
        }),
        tokens: plurals({
          one: 'токен',
          few: 'токена',
          many: 'токенов',
          other: 'токена',
        }),
        failedSuffix: 'с ошибкой',
        resetsPrefix: 'обнуляется',
        explanation:
          'Одно сообщение — это несколько обращений: ассистент работает раундами. Здесь считается работа, а не то, во что она вам обходится; тратятся лимиты выше.',
      },
      history: {
        heading: 'Недавняя активность',
        loadError: 'Не удалось загрузить ваш тариф.',
        empty: 'Пока пусто. Здесь появится то, что вы сделаете с AI-функциями.',
      },
    },
  },
);
