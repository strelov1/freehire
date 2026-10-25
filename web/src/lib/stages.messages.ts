import { defineMessages } from '#lib/i18n/t.js';
import { STAGE_GROUPS, STAGE_LABELS } from './generated/contracts';

type GroupId = (typeof STAGE_GROUPS)[number]['id'];

// English IS the generated table — this catalog adds translations and no vocabulary of its
// own. Seeded from `STAGE_LABELS` and the group labels so a stage added in Go reaches the
// English UI with its generated label at once; `stages.test.ts` then fails until every
// translated locale names it too, rather than letting it fall back to English silently.
const groupLabels = Object.fromEntries(STAGE_GROUPS.map((g) => [g.id, g.label])) as Record<GroupId, string>;

export const messages = defineMessages(
  {
    stages: { ...STAGE_LABELS } as Record<string, string>,
    groups: groupLabels as Record<string, string>,
  },
  {
    ru: {
      stages: {
        preparing: 'Подготовка',
        applied: 'Отклик отправлен',
        screening: 'Скрининг',
        responded: 'Получен ответ',
        interview: 'Собеседование',
        offer: 'Оффер',
        accepted: 'Оффер принят',
        rejected: 'Отказ',
        withdrawn: 'Отозвано',
        expired: 'Вакансия закрыта',
      },
      groups: {
        preparing: 'Подготовка',
        applied: 'Отклик',
        interview: 'Собеседование',
        offer: 'Оффер',
        closed: 'Закрыто',
      },
    },
  },
);
