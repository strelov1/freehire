import { defineMessages } from '$lib/i18n/t';

// `BOARD_COLUMNS`'s own labels (Preparing/Applied/Interview/Offer/Closed) come from
// the generated pipeline-stage vocabulary (`$lib/board.ts` → `STAGE_GROUPS`), shared
// with the funnel and `JobDrawer`'s stage picker — that vocabulary stays English
// everywhere until it gets its own pass, the same boundary `emailStatus.ts` drew.
export const messages = defineMessages(
  {
    loadError: "Couldn't load your board.",
    searchPlaceholder: 'Search company or role',
    searchAria: 'Search applications by company or role',
    clearSearchAria: 'Clear search',
    needsAttention: 'Needs attention',
    matchedOfTotal: '{matched} of {total}',
    rehearsalFailed: 'Could not start the rehearsal.',
    debriefFailed: 'Could not start the debrief.',
  },
  {
    ru: {
      loadError: 'Не удалось загрузить доску.',
      searchPlaceholder: 'Поиск по компании или роли',
      searchAria: 'Поиск заявок по компании или роли',
      clearSearchAria: 'Очистить поиск',
      needsAttention: 'Требует внимания',
      matchedOfTotal: '{matched} из {total}',
      rehearsalFailed: 'Не удалось начать репетицию.',
      debriefFailed: 'Не удалось начать обсуждение.',
    },
  },
);
