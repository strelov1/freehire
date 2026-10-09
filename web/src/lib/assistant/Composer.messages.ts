import { defineMessages } from '$lib/i18n/t';

export const messages = defineMessages(
  {
    queuedCount: '{count} queued',
    removeFromQueueAria: 'Remove from queue',
    removeTitle: 'Remove',
    placeholder: 'Message the agent — Enter to send, Shift+Enter for newline',
    stopAria: 'Stop the assistant',
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
      queueMessageAria: 'Поставить сообщение в очередь',
      sendMessageAria: 'Отправить сообщение',
    },
  },
);
