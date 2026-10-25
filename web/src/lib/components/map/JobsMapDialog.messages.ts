import { defineMessages, plurals } from '#lib/i18n/t.js';

export const messages = defineMessages(
  {
    title: 'Jobs on the map',
    mapAria: 'Map of where the matching jobs are',
    // {places} is a whole phrase from `countries`, so the noun agrees with its own number.
    summary: plurals({ one: '{count} job in {places}', other: '{count} jobs in {places}' }),
    countries: plurals({ one: '{count} country', other: '{count} countries' }),
    summaryIn: plurals({ one: '{count} job in {country}', other: '{count} jobs in {country}' }),
    showJobs: plurals({ one: 'Show {count} job', other: 'Show {count} jobs' }),
    showJobsPending: 'Show jobs',
    worldView: 'World view',
    remoteWorldwide: 'Remote worldwide · {count}',
    updating: 'Updating…',
    placedShare: '{placed} of {total} placed in a city',
    citiesFailed: "Couldn't load cities, so the map shows the country.",
    loadFailed: "Couldn't load the map. Try again in a moment.",
    busiestPlaces: 'Busiest places',
  },
  {
    ru: {
      title: 'Вакансии на карте',
      mapAria: 'Карта мест, где находятся подходящие вакансии',
      summary: plurals({
        one: '{count} вакансия в {places}',
        few: '{count} вакансии в {places}',
        many: '{count} вакансий в {places}',
        other: '{count} вакансии в {places}',
      }),
      countries: plurals({ one: '{count} стране', few: '{count} странах', many: '{count} странах', other: '{count} странах' }),
      summaryIn: plurals({
        one: '{count} вакансия: {country}',
        few: '{count} вакансии: {country}',
        many: '{count} вакансий: {country}',
        other: '{count} вакансии: {country}',
      }),
      showJobs: plurals({
        one: 'Показать {count} вакансию',
        few: 'Показать {count} вакансии',
        many: 'Показать {count} вакансий',
        other: 'Показать {count} вакансии',
      }),
      showJobsPending: 'Показать вакансии',
      worldView: 'Весь мир',
      remoteWorldwide: 'Удалённо из любой страны · {count}',
      updating: 'Обновляю…',
      placedShare: 'В городах: {placed} из {total}',
      citiesFailed: 'Не удалось загрузить города, поэтому на карте показана страна.',
      loadFailed: 'Не удалось загрузить карту. Попробуйте чуть позже.',
      busiestPlaces: 'Самые активные места',
    },
  },
);
