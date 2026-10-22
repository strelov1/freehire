import { defineMessages } from '#lib/i18n/t.js';

// Facet option labels (work mode, region, country) come from the dictionary
// (#lib/facets) and are NOT catalog keys here — they render in whatever the
// dictionary itself is written in until a separate change localizes it.
export const messages = defineMessages(
  {
    hint: 'All optional — used to tailor your job filters.',
    workFormat: 'Work format',
    whereBased: "Where you're based",
    cityOrCountry: 'City or country',
    searching: 'Searching…',
    pickFormatFirst: 'Pick a work format above to set where you can work.',
    remoteReach: 'Remote — regions you can work for (empty = worldwide)',
    addCountries: 'Add specific countries',
    openToRelocation: 'Open to relocation',
    relocateWhere: "Where you'd relocate (empty = anywhere)",
    addCity: 'Add a city',
  },
  {
    ru: {
      hint: 'Только для настройки фильтров вакансий — необязательно.',
      workFormat: 'Формат работы',
      whereBased: 'Где вы базируетесь',
      cityOrCountry: 'Город или страна',
      searching: 'Ищем…',
      pickFormatFirst: 'Выберите формат работы выше, чтобы указать, где вы можете работать.',
      remoteReach: 'Удалённо — регионы, из которых вы можете работать (пусто = весь мир)',
      addCountries: 'Добавить страны',
      openToRelocation: 'Готов(а) к переезду',
      relocateWhere: 'Куда вы готовы переехать (пусто = куда угодно)',
      addCity: 'Добавить город',
    },
  },
);
