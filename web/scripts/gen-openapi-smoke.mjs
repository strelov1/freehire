// Automated check for the OpenAPI generator that feeds the Scalar-based /docs/api
// reference. Same rationale as gen-api-docs-smoke.mjs: no JS unit runner here by
// choice, so this is the RED->GREEN test for renderOpenApi.
//
//   node scripts/gen-openapi-smoke.mjs   # asserts; exits non-zero on failure

import { loadDocsModules } from './gen-api-docs.mjs';
import { renderOpenApi } from './gen-openapi.mjs';

const checks = [];
function assert(name, cond) {
  checks.push({ name, ok: Boolean(cond) });
}

async function main() {
  const { spec, filters } = await loadDocsModules();

  const a = renderOpenApi(spec, filters);
  const b = renderOpenApi(spec, filters);
  assert('idempotent (two renders identical)', JSON.stringify(a) === JSON.stringify(b));

  assert('is OpenAPI 3.1', a.openapi === '3.1.0');
  assert('has info.title', typeof a.info?.title === 'string' && a.info.title.length > 0);

  // Overview + Filtering content lands in info.description as headed Markdown.
  const desc = a.info.description;
  assert('description has Pagination heading', desc.includes('## Pagination'));
  assert('description states the deep-paging rule', desc.includes('10000'));
  assert('description has "What is not here" heading', desc.includes('## What is not here'));
  assert('description has Filtering jobs heading', desc.includes('## Filtering jobs'));
  assert('description lists a string facet', desc.includes('`seniority`'));
  assert('description lists the and-mode modifier', desc.includes('_mode=and'));
  const recipe = filters.RECIPES[0];
  assert('description has the first recipe query', desc.includes(recipe.query));

  // Endpoint coverage and shape.
  const jobsSearch = a.paths['/jobs/search']?.get;
  assert('documents GET /jobs/search', Boolean(jobsSearch));
  assert('tags come from the group title', jobsSearch.tags?.includes('Jobs'));

  // auth: 'none' is an explicit empty security requirement, not an omitted field.
  assert('public endpoint has explicit empty security', Array.isArray(a.paths['/jobs'].get.security) && a.paths['/jobs'].get.security.length === 0);

  // Security schemes named per auth level, all riding the same session cookie
  // where applicable, semantically distinguished by their own description.
  const schemes = a.components.securitySchemes;
  assert('has cookieAuth scheme', schemes.cookieAuth?.type === 'apiKey' && schemes.cookieAuth.in === 'cookie');
  assert('has apiKeyAuth scheme', schemes.apiKeyAuth?.type === 'http' && schemes.apiKeyAuth.scheme === 'bearer');
  assert('has moderatorAuth scheme', Boolean(schemes.moderatorAuth));
  assert('has extensionAuth scheme', Boolean(schemes.extensionAuth));

  // cookie-or-key is an OR of the two schemes.
  const oauthExchange = a.paths['/market/coverage']?.post;
  assert('cookie-or-key endpoint found', Boolean(oauthExchange));
  assert(
    'cookie-or-key security is an OR of cookieAuth/apiKeyAuth',
    oauthExchange.security?.length === 2 &&
      oauthExchange.security.some((s) => 'cookieAuth' in s) &&
      oauthExchange.security.some((s) => 'apiKeyAuth' in s),
  );

  // Prose-placeholder params ("(any search filter)") are not real parameter
  // names; they fold into the description instead of becoming a malformed
  // OpenAPI parameter.
  assert(
    'placeholder param is not a literal parameter',
    !(oauthExchange.parameters ?? []).some((p) => p.name.startsWith('(')),
  );
  assert('placeholder param note reaches the description', oauthExchange.description.includes('Any search facet param scopes the market'));

  // SSE endpoints get a text/event-stream response, not application/json.
  const stream = a.paths['/jobs/{slug}/match-analysis/stream']?.get;
  assert('SSE endpoint found', Boolean(stream));
  assert('SSE endpoint uses text/event-stream', 'text/event-stream' in (stream.responses?.['200']?.content ?? {}));
  assert('SSE endpoint has no application/json response', !('application/json' in (stream.responses?.['200']?.content ?? {})));

  // A second, differently-shaped SSE endpoint: its example opens with a named
  // `event:` frame before the `data:` line, not `data:` first — the shape that
  // slipped past a naive "starts with data:" check (code review finding).
  const assistantStream = a.paths['/assistant/sessions/{id}/messages']?.post;
  assert('named-event SSE endpoint found', Boolean(assistantStream));
  assert(
    'named-event SSE endpoint uses text/event-stream',
    'text/event-stream' in (assistantStream.responses?.['200']?.content ?? {}),
  );
  assert(
    'named-event SSE endpoint has no application/json response',
    !('application/json' in (assistantStream.responses?.['200']?.content ?? {})),
  );

  // A third SSE endpoint, documented as SSE in prose ("same SSE shape as
  // .../messages") but with no responseExample at all — nothing to sniff a
  // leading `data:`/`event:` line from (code review finding).
  const autopilot = a.paths['/assistant/sessions/{id}/autopilot']?.post;
  assert('prose-only SSE endpoint found', Boolean(autopilot));
  assert(
    'prose-only SSE endpoint uses text/event-stream',
    'text/event-stream' in (autopilot.responses?.['200']?.content ?? {}),
  );

  // Parameter examples reach the generated schema, not just placeholder notes.
  const jobsSlug = a.paths['/jobs/{slug}']?.get;
  assert('jobs/{slug} found', Boolean(jobsSlug));
  const slugParam = jobsSlug.parameters?.find((p) => p.name === 'slug');
  assert('path parameter carries its example', Boolean(slugParam?.example));

  // Deprecated marking, exercised on a synthetic fixture (no shipped endpoint
  // currently qualifies — see openspec/changes/migrate-api-docs-scalar/tasks.md).
  const deprecatedFixture = {
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
  const d = renderOpenApi(deprecatedFixture, filters);
  const fixtureOp = d.paths['/fixture/{id}'].get;
  assert('deprecated operation is marked', fixtureOp.deprecated === true);
  assert('deprecated note names the replacement', fixtureOp.description.includes('GET /fixture/v2/{id}'));
  // Path params are always required in OpenAPI, even if the source data omits it.
  assert('path param is forced required', fixtureOp.parameters.find((p) => p.name === 'id')?.required === true);

  let failed = 0;
  for (const c of checks) {
    if (!c.ok) failed++;
    console.log(`${c.ok ? 'PASS' : 'FAIL'}  ${c.name}`);
  }
  if (failed) {
    console.error(`\n${failed} check(s) failed`);
    process.exit(1);
  }
  console.log(`\nAll ${checks.length} checks passed.`);
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
