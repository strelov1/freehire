import { defineMessages, plurals } from '#lib/i18n/t.js';

// `plan.allowances[].feature` and the invoice/subscription statuses are stable API
// identifiers, so both are MAPPED here rather than printed. A feature the map does not
// know yet is humanised by PlanView ("cover-letter" → "Cover letter") rather than printed
// raw; PlanView.test.ts holds the map to every feature the backend declares.
export const messages = defineMessages(
  {
    headTitle: 'Your plan — freehire',
    signedOut: 'Sign in to view your plan.',
    title: 'Your plan',
    description:
      'Free covers reading your CV and AI search. Pro unlocks every AI feature, each with a daily allowance that starts over every night.',
    planStrip: {
      pro: 'Pro',
      ultra: 'Ultra',
      free: 'Free',
      runsUntilPrefix: 'Runs until',
      freeSubtitle: 'Same features, daily limits',
      upgrade: 'Upgrade to Pro',
      upgradeUltra: 'Upgrade to Ultra',
      // The provider's portal is where a subscription is changed AND cancelled, so the label
      // has to cover both — "Cancel" alone would hide the card change, and a separate cancel
      // button would need a flow of our own that could disagree with what happened to the
      // money.
      manageSubscription: 'Manage or cancel',
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
      // Shown only when the provider reports more than one active subscription for this
      // customer at once — a state this page never creates going forward, but can still
      // describe for a customer it already happened to.
      duplicateWarning:
        'You have more than one active subscription, and are being charged for each of them. Use "Manage or cancel" to cancel the extra one.',
    },
    today: {
      heading: 'Today',
      resetsAtPrefix: 'Resets at',
      unlimited: 'Unlimited',
      // A feature whose daily allowance on this plan is zero. "0 / 0" would read as a
      // spent allowance, which is a different thing from one the plan does not include.
      notOnPlan: 'Not on {plan}',
      features: {
        tailor: 'CV editing sessions',
        match: 'Job analyses',
        assistant: 'Assistant messages',
        dictation: 'Dictation',
        'cover-letter': 'Cover letters',
        'auto-apply': 'Automatic applications',
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
        'На Free доступны разбор CV и AI-поиск. Pro открывает все AI-функции — у каждой свой дневной лимит, и каждую ночь он обнуляется.',
      planStrip: {
        pro: 'Pro',
        ultra: 'Ultra',
        free: 'Free',
        runsUntilPrefix: 'Действует до',
        freeSubtitle: 'Те же функции, дневные лимиты',
        upgrade: 'Перейти на Pro',
        upgradeUltra: 'Перейти на Ultra',
        manageSubscription: 'Изменить или отменить',
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
        duplicateWarning:
          'У вас активно больше одной подписки, и оплата идёт по каждой из них. Используйте «Изменить или отменить», чтобы отменить лишнюю.',
      },
      today: {
        heading: 'Сегодня',
        resetsAtPrefix: 'Обнуляется в',
        unlimited: 'Без ограничений',
        notOnPlan: 'Нет в тарифе {plan}',
        features: {
          tailor: 'Сессии редактирования CV',
          match: 'Анализы вакансий',
          assistant: 'Сообщения ассистенту',
          dictation: 'Диктовка',
          'cover-letter': 'Сопроводительные письма',
          'auto-apply': 'Автоматические отклики',
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
