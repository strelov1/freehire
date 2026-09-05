import { defineMessages } from '$lib/i18n/t';

// The `Authorization: Bearer <key>` header and the CLI command in the reveal box
// are protocol, not prose — they stay English in every locale, and are composed in
// the component rather than living here.
export const messages = defineMessages(
  {
    headTitle: 'API keys — freehire',
    signedOut: 'Sign in to create and manage API keys.',
    title: 'API keys',
    intro: {
      // The paragraph is split around its two inline links and the header code
      // sample, in reading order. Every segment ENDS a phrase and the next one
      // opens with its own punctuation, so no segment needs a leading or
      // trailing space to read correctly — and the sentence's final period is a
      // literal in the markup, since it is the same character in every locale.
      lead: 'Reach the API without a browser — search, open jobs, and track applications from a script. Use the',
      cliLink: 'freehire CLI',
      orSendDirectly: ', or send the key directly as',
      seeThe: '. See the',
      apiReferenceLink: 'API reference',
      // The one segment that FOLLOWS a link, so it opens a phrase rather than
      // ending one and takes its leading space from the markup. It exists so the
      // English sentence keeps its original order — inverting the clause to avoid
      // a fourth key would have been a copy change smuggled in as a refactor.
      apiReferenceTail: 'for every endpoint and filter.',
    },
    reveal: {
      // "New key “<name>”" — the component supplies the quoted name.
      newKeyPrefix: 'New key',
      copyNow: 'Copy it now — it won’t be shown again.',
      dismiss: 'Dismiss',
      copy: 'Copy',
      copied: 'Copied',
      newToCli: 'New to the CLI? See the',
      commandReferenceLink: 'command reference',
    },
    form: {
      nameLabel: 'Name',
      namePlaceholder: 'e.g. CI bot',
      expiryLabel: 'Expiry',
      expiryNever: 'No expiry',
      expiry30: '30 days',
      expiry90: '90 days',
      expiry365: '1 year',
      confirmPasswordLabel: 'Confirm password',
      create: 'Create key',
      creating: 'Creating…',
    },
    reauth: {
      prompt: 'Confirm your identity with a connected provider before creating or revoking a key.',
      loading: 'Loading connected providers…',
      error: 'Could not load connected providers.',
      // Split around the provider name, which is a brand and never translated.
      confirmWithPrefix: 'Confirm with',
    },
    errors: {
      passwordRequired: 'Enter your password to confirm this security change.',
      wrongPassword: 'That password is not right.',
      reauthBeforeCreate: 'Confirm your identity before creating a key.',
      reauthBeforeRevoke: 'Confirm your identity before revoking a key.',
      createFailed: 'Could not create the key. Please try again.',
      revokeFailed: 'Could not revoke the key. Please try again.',
    },
    list: {
      loadError: "Couldn't load your API keys.",
      empty: 'No API keys yet. Create one above to use the API from a script.',
      // "Created <2 days ago> · <last used 3 hours ago|never used> · expires <in a month>"
      createdPrefix: 'Created',
      lastUsedPrefix: 'last used',
      neverUsed: 'never used',
      expiresPrefix: 'expires',
      revoke: 'Revoke',
    },
    revokeDialog: {
      // "Revoke "<name>"?" — the component supplies the quoted name.
      titlePrefix: 'Revoke',
      titleSuffix: '?',
      description: 'Any script using it stops working immediately.',
      confirmLabel: 'Revoke',
    },
  },
  {
    ru: {
      headTitle: 'API-ключи — freehire',
      signedOut: 'Войдите, чтобы создавать API-ключи и управлять ими.',
      title: 'API-ключи',
      intro: {
        lead: 'Обращайтесь к API без браузера — ищите вакансии, открывайте их и ведите отклики из скрипта. Используйте',
        cliLink: 'freehire CLI',
        orSendDirectly: ' или передавайте ключ напрямую как',
        seeThe: '. Смотрите',
        apiReferenceLink: 'справочник API',
        apiReferenceTail: '— там все эндпоинты и фильтры.',
      },
      reveal: {
        newKeyPrefix: 'Новый ключ',
        copyNow: 'Скопируйте сейчас — второй раз он показан не будет.',
        dismiss: 'Закрыть',
        copy: 'Скопировать',
        copied: 'Скопировано',
        newToCli: 'Впервые работаете с CLI? Откройте',
        commandReferenceLink: 'справочник команд',
      },
      form: {
        nameLabel: 'Название',
        namePlaceholder: 'например, CI bot',
        expiryLabel: 'Срок действия',
        expiryNever: 'Бессрочно',
        expiry30: '30 дней',
        expiry90: '90 дней',
        expiry365: '1 год',
        confirmPasswordLabel: 'Подтвердите пароль',
        create: 'Создать ключ',
        creating: 'Создание…',
      },
      reauth: {
        prompt:
          'Подтвердите свою личность через подключённого провайдера, прежде чем создавать или отзывать ключ.',
        loading: 'Загрузка подключённых провайдеров…',
        error: 'Не удалось загрузить подключённых провайдеров.',
        confirmWithPrefix: 'Подтвердить через',
      },
      errors: {
        passwordRequired: 'Введите пароль, чтобы подтвердить изменение настроек безопасности.',
        wrongPassword: 'Неверный пароль.',
        reauthBeforeCreate: 'Подтвердите свою личность, прежде чем создавать ключ.',
        reauthBeforeRevoke: 'Подтвердите свою личность, прежде чем отзывать ключ.',
        createFailed: 'Не удалось создать ключ. Попробуйте ещё раз.',
        revokeFailed: 'Не удалось отозвать ключ. Попробуйте ещё раз.',
      },
      list: {
        loadError: 'Не удалось загрузить ваши API-ключи.',
        empty: 'API-ключей пока нет. Создайте ключ выше, чтобы обращаться к API из скрипта.',
        createdPrefix: 'Создан',
        lastUsedPrefix: 'последний раз использован',
        neverUsed: 'ни разу не использован',
        expiresPrefix: 'истекает',
        revoke: 'Отозвать',
      },
      revokeDialog: {
        titlePrefix: 'Отозвать',
        titleSuffix: '?',
        description: 'Любой скрипт, который им пользуется, перестанет работать немедленно.',
        confirmLabel: 'Отозвать',
      },
    },
  },
);
