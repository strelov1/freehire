import { defineMessages } from '$lib/i18n/t';

export const messages = defineMessages(
  {
    headTitle: 'Security · freehire',
    title: 'Security',
    subtitle: 'Change your password and end sessions on other devices.',
    password: {
      heading: 'Password',
      subheading: 'Changing it signs out every other device. You stay signed in here.',
      noPasswordNotice:
        'This account signs in with a provider and has no password. To add one — a second way in, independent of the provider — sign out, choose “Forgot your password?” on the sign-in screen, and set it with the code we email you.',
      currentPasswordLabel: 'Current password',
      newPasswordLabel: 'New password',
      repeatPasswordLabel: 'Repeat new password',
      mismatchError: 'The two new passwords do not match.',
      wrongCurrentPassword: 'That current password is not right.',
      weakPassword: 'Choose a password of 8–72 characters.',
      genericError: 'Something went wrong. Please try again.',
      changed: 'Password changed. Other devices were signed out.',
      saving: 'Saving…',
      save: 'Change password',
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
      subtitle: 'Измените пароль и завершите сессии на других устройствах.',
      password: {
        heading: 'Пароль',
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
        genericError: 'Что-то пошло не так. Попробуйте ещё раз.',
        changed: 'Пароль изменён. Другие устройства вышли из аккаунта.',
        saving: 'Сохранение…',
        save: 'Изменить пароль',
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
