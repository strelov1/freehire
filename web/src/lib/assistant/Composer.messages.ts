import { defineMessages } from '#lib/i18n/t.js';

export const messages = defineMessages(
  {
    queuedCount: '{count} queued',
    removeFromQueueAria: 'Remove from queue',
    removeTitle: 'Remove',
    placeholder: 'Message the agent — Enter to send, Shift+Enter for newline',
    stopAria: 'Stop the assistant',
    messageAria: 'Message to the assistant',
    queueMessageAria: 'Queue message',
    sendMessageAria: 'Send message',
  },
  {
    ru: {
      queuedCount: 'В очереди: {count}',
      removeFromQueueAria: 'Убрать из очереди',
      removeTitle: 'Убрать',
      placeholder: 'Сообщение ассистенту — Enter отправить, Shift+Enter новая строка',
      stopAria: 'Остановить ассистента',
      messageAria: 'Сообщение ассистенту',
      queueMessageAria: 'Поставить сообщение в очередь',
      sendMessageAria: 'Отправить сообщение',
    },
  },
);
