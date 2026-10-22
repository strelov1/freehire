import { defineMessages } from '#lib/i18n/t.js';

export const messages = defineMessages(
  {
    lastSkillBlocked: 'You need at least one skill — add another before removing this one.',
    // `{skill}` is substituted via `#lib/i18n/t`'s `format()` — see its doc comment
    // for why a template beats splitting this into a prefix/suffix pair.
    saveFailed: 'Could not update {skill} in your profile. Try again.',
  },
  {
    ru: {
      lastSkillBlocked: 'Нужен хотя бы один навык — добавьте другой, прежде чем убрать этот.',
      saveFailed: 'Не удалось обновить {skill} в вашем профиле. Попробуйте ещё раз.',
    },
  },
);
