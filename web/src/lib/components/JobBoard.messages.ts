import { defineMessages } from '#lib/i18n/t.js';

// `BOARD_COLUMNS`'s own labels (Preparing/Applied/Interview/Offer/Closed) come from
// the generated pipeline-stage vocabulary (`#lib/board.ts` → `STAGE_GROUPS`), shared
// with the funnel and `JobDrawer`'s stage picker — that vocabulary stays English
// everywhere until it gets its own pass, the same boundary `emailStatus.ts` drew.
export const messages = defineMessages(
  {
    loadError: "Couldn't load your board.",
    searchPlaceholder: 'Search company or role',
    searchAria: 'Search applications by company or role',
    clearSearchAria: 'Clear search',
    needsAttention: 'Needs attention',
    stageFilterAria: 'Filter applications by stage',
    allStages: 'All stages',
    noStage: 'No stage',
    matchedOfTotal: '{matched} of {total}',
    rehearsalFailed: 'Could not start the rehearsal.',
    debriefFailed: 'Could not start the debrief.',
    untitledJob: 'Untitled role',
    movedTo: 'Moved {job} to {stage}.',
    moveFailed: "Couldn't move {job} — it's back in {stage}. Try again.",
    moveFailedNoStage: "Couldn't move {job}. Try again.",
    notesFailed: "Couldn't save your notes for {job}. Open it to try again.",
    removeFailed: "Couldn't remove it from your board. Try again.",
    removed: 'Removed {job} from your board.',
    dismissError: 'Dismiss',
    emptyTitle: 'Your board is empty',
    emptyDescription:
      'Track every application from first save to offer in one place. Save a job from any listing and it lands here.',
    emptyFindJobs: 'Find jobs',
    emptyUploadCv: 'Upload your CV',
  },
  {
    ru: {
      loadError: 'Не удалось загрузить доску.',
      searchPlaceholder: 'Поиск по компании или роли',
      searchAria: 'Поиск заявок по компании или роли',
      clearSearchAria: 'Очистить поиск',
      needsAttention: 'Требует внимания',
      stageFilterAria: 'Фильтр заявок по этапу',
      allStages: 'Все этапы',
      noStage: 'Без этапа',
      matchedOfTotal: '{matched} из {total}',
      rehearsalFailed: 'Не удалось начать репетицию.',
      debriefFailed: 'Не удалось начать обсуждение.',
      untitledJob: 'Без названия',
      movedTo: '{job}: перенесено в «{stage}».',
      moveFailed: 'Не удалось перенести {job} — карточка вернулась в «{stage}». Попробуйте ещё раз.',
      moveFailedNoStage: 'Не удалось перенести {job}. Попробуйте ещё раз.',
      notesFailed: 'Не удалось сохранить заметки к {job}. Откройте карточку, чтобы повторить.',
      removeFailed: 'Не удалось убрать с доски. Попробуйте ещё раз.',
      removed: '{job}: убрано с доски.',
      dismissError: 'Закрыть',
      emptyTitle: 'Доска пока пуста',
      emptyDescription:
        'Здесь видно каждую заявку — от сохранения до оффера. Сохраните вакансию из любого списка, и она появится здесь.',
      emptyFindJobs: 'Найти вакансии',
      emptyUploadCv: 'Загрузить CV',
    },
  },
);
