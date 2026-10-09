import { defineMessages } from '$lib/i18n/t';

export const messages = defineMessages(
  {
    notFound: 'This conversation could not be found.',
    micUnavailable: 'The microphone is unavailable — check this site’s permission for it.',
    disconnected: 'The call disconnected unexpectedly.',
    connectFailed: 'Could not connect the call ({status}).',
    startFailed: 'Could not start voice mode.',
    connecting: 'Connecting…',
    close: 'Close',
    endingSoon: 'This call ends in about a minute.',
    interviewer: 'Interviewer:',
    you: 'You:',
    endCall: 'End call',
  },
  {
    ru: {
      notFound: 'Этот разговор не найден.',
      micUnavailable: 'Микрофон недоступен — проверьте разрешение для этого сайта.',
      disconnected: 'Звонок неожиданно прервался.',
      connectFailed: 'Не удалось подключить звонок ({status}).',
      startFailed: 'Не удалось запустить голосовой режим.',
      connecting: 'Подключаемся…',
      close: 'Закрыть',
      endingSoon: 'Звонок завершится примерно через минуту.',
      interviewer: 'Интервьюер:',
      you: 'Вы:',
      endCall: 'Завершить звонок',
    },
  },
);
