import { BASE_URL, type Group } from '../api-spec-types';

export const activityGroup: Group = {
  title: 'Activity',
  intro: 'A public read of the catalogue-activity time series.',
  endpoints: [
    {
      method: 'GET',
      path: '/stats/jobs-activity',
      auth: 'none',
      summary: 'Public time series of added vs. removed vacancies per period.',
      description:
        'Aggregated to the requested granularity over a date range; the series is ' +
        'dense (missing periods are 0). Defaults: `granularity=day`, `to`=today, ' +
        '`from` a per-granularity window before `to`. An unknown granularity or a ' +
        'range over 4000 days is a `400`.',
      query: [
        {
          name: 'granularity',
          type: 'string',
          description: 'One of `day`, `week`, `month` (default `day`).',
          example: 'week',
        },
        {
          name: 'from',
          type: 'string (YYYY-MM-DD)',
          description: 'Start date (UTC). Defaults to a per-granularity window before `to`.',
          example: '2026-01-01',
        },
        {
          name: 'to',
          type: 'string (YYYY-MM-DD)',
          description: 'End date (UTC). Defaults to today.',
          example: '2026-06-30',
        },
      ],
      curl: `curl "${BASE_URL}/stats/jobs-activity?granularity=week"`,
      responseExample: `{
  "data": [
    { "period": "2026-06-01", "added": 120, "removed": 40 },
    { "period": "2026-06-08", "added": 98, "removed": 55 }
  ],
  "meta": { "granularity": "week", "from": "2025-06-09", "to": "2026-06-08" }
}`,
    },
  ],
};
