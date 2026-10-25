import { defineMessages } from '#lib/i18n/t.js';

export const messages = defineMessages(
  {
    // The card's description, read after its indicators: dragging is pointer-only, and
    // the stage picker in the opened panel is the keyboard's way to move a card.
    moveHint: 'Open to change its stage.',
  },
  {
    ru: {
      moveHint: 'Откройте, чтобы сменить этап.',
    },
  },
);
