import { BASE_URL, type Group } from '../api-spec-types';

export const marketPulseGroup: Group = {
  title: 'Market pulse',
  intro:
    'Your own profile skills’ demand trend, joined against the weekly market ' +
    'snapshot. Cookie-only, unlike the aggregate insights above — this reads ' +
    'your saved profile.',
  endpoints: [
    {
      method: 'GET',
      path: '/me/market-pulse',
      auth: 'cookie',
      summary: 'Weekly demand trend for your profile skills.',
      description:
        'One entry per profile skill seen in at least one open job; a skill ' +
        'never seen is omitted rather than reported with a zero. `change_pct` ' +
        'is `null` with fewer than two snapshots, or when the earliest ' +
        'snapshot’s count was zero. A profile with no skills yet answers an ' +
        'empty `data` array, not an error.',
      curl: `curl "${BASE_URL}/me/market-pulse" -b cookies.txt`,
      responseExample: `{
  "data": [
    {
      "skill": "go",
      "open_count": 640,
      "change_pct": 12.5,
      "series": [
        { "week_start": "2026-07-06", "open_count": 570 },
        { "week_start": "2026-08-10", "open_count": 640 }
      ]
    }
  ],
  "meta": { "week_start": "2026-08-10" }
}`,
    },
  ],
};
