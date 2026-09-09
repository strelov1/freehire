// Shared synthetic fixture for exercising the `deprecated` field mechanism in
// both generators' smoke tests. No shipped endpoint currently carries
// `deprecated` — see openspec/changes/migrate-api-docs-scalar/tasks.md — so
// there is no real endpoint to assert against; a fixture endpoint stands in
// for one, kept in one place so the two smoke tests can't quietly diverge on
// its shape.
export function buildDeprecatedFixture(spec) {
  return {
    BASE_URL: spec.BASE_URL,
    OVERVIEW: spec.OVERVIEW,
    AUTH_LABELS: spec.AUTH_LABELS,
    GROUPS: [
      {
        title: 'Fixture',
        intro: 'Synthetic group for the deprecated-endpoint smoke check.',
        endpoints: [
          {
            method: 'GET',
            path: '/fixture/{id}',
            auth: 'none',
            summary: 'A fixture endpoint.',
            deprecated: { since: '2026-09-08', replacement: 'GET /fixture/v2/{id}' },
            pathParams: [{ name: 'id', type: 'integer', description: 'Fixture id.' }],
            curl: `curl "${spec.BASE_URL}/fixture/1"`,
          },
        ],
      },
    ],
  };
}
