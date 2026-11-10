import { BASE_URL, type Group } from '../api-spec-types';

export const geographyGroup: Group = {
  title: 'Geography',
  intro: 'Public reference data backing search and profile city autocompletes.',
  endpoints: [
    {
      method: 'GET',
      path: '/geo/cities',
      auth: 'none',
      summary: 'Prefix-search city names for autocomplete.',
      description:
        'Population-ranked prefix search over the embedded city dictionary, ' +
        'optionally narrowed to one country. A blank `q` returns an empty list.',
      query: [
        { name: 'q', type: 'string', description: 'Prefix to match.', example: 'berl' },
        { name: 'country', type: 'string', description: 'ISO 3166-1 alpha-2 code to narrow to.', example: 'DE' },
      ],
      curl: `curl "${BASE_URL}/geo/cities?q=berl&country=DE"`,
      responseExample: `{ "data": [ { "value": "Berlin", "country": "DE" } ] }`,
    },
  ],
};
