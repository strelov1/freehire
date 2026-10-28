import { defineMessages } from '#lib/i18n/t.js';

export const messages = defineMessages(
  {
    headTitle: 'Security · freehire',
    title: 'Security',
    subtitle: 'Your email, password, and the devices signed in to this account.',
    cancel: 'Cancel',
    emailChanged: 'Email changed. Other devices were signed out.',
    passwordChanged: 'Password changed. Other devices were signed out.',
    email: {
      heading: 'Email',
      change: 'Change email',
      dialogTitle: 'Change email',
      providersNote:
        'Google and other sign-in providers already linked to this account keep working after the change.',
      newEmailLabel: 'New email',
      confirmPrompt: 'Your email is how you sign in, so confirm it is you before changing it.',
      sendCode: 'Send confirmation code',
      sending: 'Sending…',
      codeSentPrefix: 'We sent a six-digit code to',
      codeSentSuffix: '. Enter it to finish — nothing changes until you do.',
      codeLabel: 'Code',
      confirm: 'Confirm',
      saving: 'Saving…',
      resend: 'Send a new code',
      otherAddress: 'Use a different address',
      invalidEmail: 'That does not look like an email address.',
      sameEmail: 'That is already your email.',
      emailTaken: 'Another freehire account already uses this email.',
      tooSoon: 'A code was just sent. Wait a minute before asking for another.',
      invalidCode: 'That code is wrong or has expired. Check the mail, or send a new code.',
      unavailable: 'We cannot send email right now. Please try again later.',
      genericError: 'Something went wrong. Please try again.',
    },
    password: {
      heading: 'Password',
      isSet: 'Set',
      change: 'Change password',
      dialogTitle: 'Change password',
      subheading: 'Changing it signs out every other device. You stay signed in here.',
      noPasswordNotice:
        'This account signs in with a provider and has no password. To add one — a second way in, independent of the provider — sign out, choose “Forgot your password?” on the sign-in screen, and set it with the code we email you.',
      currentPasswordLabel: 'Current password',
      newPasswordLabel: 'New password',
      repeatPasswordLabel: 'Repeat new password',
      mismatchError: 'The two new passwords do not match.',
      wrongCurrentPassword: 'That current password is not right.',
      weakPassword: 'Choose a password of 8–72 characters.',
      staleSession:
        'This sign-in session is too old to confirm a security change. Please sign out and sign back in, then try again.',
      genericError: 'Something went wrong. Please try again.',
      saving: 'Saving…',
      save: 'Save password',
    },
    sessions: {
      heading: 'Sessions',
      subheading:
        'Sign out of every device, including this one. Use this if you think someone else has access. Your API keys keep working — revoke those individually.',
      signingOut: 'Signing out…',
      signOut: 'Sign out everywhere',
    },
    dangerZone: {
      heading: 'Danger zone',
      description: 'Permanently erase your account and everything in it. This cannot be undone.',
    },
  },
  {
    ru: {
      headTitle: 'Безопасность · freehire',
      title: 'Безопасность',
      // "сессии", matching the `sessions.heading` below — the subtitle said
      // "сеансы" for the same thing.
      subtitle: 'Почта, пароль и устройства, на которых выполнен вход.',
      cancel: 'Отмена',
      emailChanged: 'Почта изменена. Другие устройства вышли из аккаунта.',
      passwordChanged: 'Пароль изменён. Другие устройства вышли из аккаунта.',
      email: {
        heading: 'Почта',
        change: 'Сменить почту',
        dialogTitle: 'Смена почты',
        providersNote:
          'Вход через Google и другие сервисы, уже привязанные к аккаунту, продолжит работать после смены.',
        newEmailLabel: 'Новая почта',
        confirmPrompt: 'Почта — это ваш логин, поэтому перед сменой подтвердите, что это вы.',
        sendCode: 'Отправить код',
        sending: 'Отправка…',
        codeSentPrefix: 'Мы отправили шестизначный код на',
        codeSentSuffix: '. Введите его, чтобы завершить смену — до этого ничего не изменится.',
        codeLabel: 'Код',
        confirm: 'Подтвердить',
        saving: 'Сохранение…',
        resend: 'Отправить новый код',
        otherAddress: 'Указать другой адрес',
        invalidEmail: 'Это не похоже на адрес почты.',
        sameEmail: 'Это и так ваша почта.',
        emailTaken: 'Эта почта уже используется другим аккаунтом freehire.',
        tooSoon: 'Код только что отправлен. Подождите минуту, прежде чем запросить новый.',
        invalidCode: 'Код неверный или устарел. Проверьте письмо или запросите новый код.',
        unavailable: 'Сейчас мы не можем отправить письмо. Попробуйте позже.',
        genericError: 'Что-то пошло не так. Попробуйте ещё раз.',
      },
      password: {
        heading: 'Пароль',
        isSet: 'Задан',
        change: 'Сменить пароль',
        dialogTitle: 'Смена пароля',
        subheading:
          'При смене пароля все остальные устройства выходят из аккаунта. Здесь вы останетесь в системе.',
        noPasswordNotice:
          'Этот аккаунт входит через провайдера и не имеет пароля. Чтобы добавить его — ещё один способ входа, независимый от провайдера — выйдите из аккаунта, выберите «Забыли пароль?» на экране входа и задайте пароль с помощью кода, который мы отправим на почту.',
        currentPasswordLabel: 'Текущий пароль',
        newPasswordLabel: 'Новый пароль',
        repeatPasswordLabel: 'Повторите новый пароль',
        mismatchError: 'Новые пароли не совпадают.',
        wrongCurrentPassword: 'Текущий пароль указан неверно.',
        weakPassword: 'Выберите пароль от 8 до 72 символов.',
        staleSession:
          'Эта сессия слишком старая, чтобы подтвердить изменение настроек безопасности. Выйдите и войдите заново, затем повторите.',
        genericError: 'Что-то пошло не так. Попробуйте ещё раз.',
        saving: 'Сохранение…',
        save: 'Сохранить пароль',
      },
      sessions: {
        heading: 'Сессии',
        subheading:
          'Выйдите из всех устройств, включая это. Используйте, если думаете, что кто-то ещё получил доступ. Ваши API-ключи продолжат работать — отзывайте их по отдельности.',
        signingOut: 'Выход…',
        signOut: 'Выйти везде',
      },
      dangerZone: {
        heading: 'Опасная зона',
        description:
          'Безвозвратно удалите аккаунт и всё, что с ним связано. Это действие нельзя отменить.',
      },
    },
  },
);
