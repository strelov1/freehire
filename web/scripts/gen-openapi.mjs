// Generates the OpenAPI 3.1 document that the Scalar-based /docs/api reference
// renders, from the same single typed source as gen-api-docs.mjs
// (web/src/lib/docs/api-spec.ts + filters.ts), so the two representations of
// the API cannot drift.
//
// This is UNRELATED to web/static/openapi.yaml, which is a hand-maintained,
// deliberately narrow OpenAPI 3.0.3 document for the ChatGPT Actions importer
// (pinned to that shape by the importer's own limits — see
// openspec/changes/migrate-api-docs-scalar/design.md). This generator's output
// is a separate file with the whole public API surface, consumed only by the
// Scalar reference.
//
//   node scripts/gen-openapi.mjs    # writes ../static/api-reference.openapi.json

import { writeFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';
import { loadDocsModules } from './gen-api-docs.mjs';

const here = dirname(fileURLToPath(import.meta.url));
const webRoot = resolve(here, '..');
const outFile = resolve(webRoot, 'static', 'api-reference.openapi.json');

const SESSION_COOKIE_NAME = 'hire_token';

const SECURITY_SCHEMES = {
  cookieAuth: {
    type: 'apiKey',
    in: 'cookie',
    name: SESSION_COOKIE_NAME,
    description: 'Browser session cookie set on sign-in (HttpOnly, same-origin).',
  },
  apiKeyAuth: {
    type: 'http',
    scheme: 'bearer',
    description: 'Personal API key sent as `Authorization: Bearer <token>`.',
  },
  moderatorAuth: {
    type: 'apiKey',
    in: 'cookie',
    name: SESSION_COOKIE_NAME,
    description: 'Browser session cookie from an account with the moderator role.',
  },
  extensionAuth: {
    type: 'apiKey',
    in: 'cookie',
    name: SESSION_COOKIE_NAME,
    description: 'Browser session cookie, reachable only through the browser extension consent flow.',
  },
};

// auth -> OpenAPI `security` value. 'none' is an explicit empty requirement
// (not an omitted field), so a reader never has to guess whether "no security
// listed" meant public or merely undocumented.
function securityFor(auth) {
  switch (auth) {
    case 'none':
      return [];
    case 'cookie-or-key':
      return [{ cookieAuth: [] }, { apiKeyAuth: [] }];
    case 'cookie':
      return [{ cookieAuth: [] }];
    case 'moderator':
      return [{ moderatorAuth: [] }];
    case 'extension':
      return [{ extensionAuth: [] }];
    default:
      throw new Error(`Unknown auth level "${auth}" — add it to securityFor() in gen-openapi.mjs`);
  }
}

// Param.type is a closed, hand-written vocabulary (see api-spec.ts), not a JSON
// Schema type string. An unmapped value throws rather than silently emitting an
// untyped schema, so a new type string forces this table to be extended.
const TYPE_SCHEMAS = {
  string: { type: 'string' },
  'string (date)': { type: 'string', format: 'date' },
  'string (YYYY-MM-DD)': { type: 'string', format: 'date' },
  'string (RFC3339)': { type: 'string', format: 'date-time' },
  'string (uuid)': { type: 'string', format: 'uuid' },
  'string (UUID)': { type: 'string', format: 'uuid' },
  'string (form field)': { type: 'string' },
  'string[]': { type: 'array', items: { type: 'string' } },
  'string[] (2 uuids)': { type: 'array', items: { type: 'string', format: 'uuid' }, minItems: 2, maxItems: 2 },
  integer: { type: 'integer' },
  number: { type: 'number' },
  boolean: { type: 'boolean' },
  array: { type: 'array' },
  object: { type: 'object' },
  'object[]': { type: 'array', items: { type: 'object' } },
  'file (form field)': { type: 'string', format: 'binary' },
  any: {},
  varies: {},
};

function schemaFor(rawType) {
  const schema = TYPE_SCHEMAS[rawType];
  if (!schema) throw new Error(`Unknown param type "${rawType}" — add it to TYPE_SCHEMAS in gen-openapi.mjs`);
  return schema;
}

// A handful of body/query params document "any of these apply" with a
// parenthesized pseudo-name ("(any search filter)") rather than a real
// parameter name. OpenAPI requires a concrete `name`, so these fold into the
// operation description instead of becoming a malformed parameter.
function isPlaceholderParam(p) {
  return p.name.startsWith('(');
}

function placeholderNote(p) {
  return p.example ? `${p.description} (e.g. \`${p.example}\`)` : p.description;
}

function toParameter(p, location) {
  return {
    name: p.name,
    in: location,
    required: location === 'path' ? true : Boolean(p.required),
    description: p.description,
    schema: schemaFor(p.type),
  };
}

function usesFormData(bodyParams) {
  return bodyParams.some((p) => p.type.includes('(form field)'));
}

function requestBodyFor(ep) {
  const real = (ep.body ?? []).filter((p) => !isPlaceholderParam(p));
  const placeholders = (ep.body ?? []).filter(isPlaceholderParam);
  const properties = {};
  const required = [];
  for (const p of real) {
    properties[p.name] = { ...schemaFor(p.type), description: p.description };
    if (p.required) required.push(p.name);
  }
  const schema = {
    type: 'object',
    ...(Object.keys(properties).length ? { properties } : {}),
    ...(required.length ? { required } : {}),
    // A body made only of placeholder fields ("(any job field)") has no fixed
    // shape to declare, so it stays open rather than an empty, over-narrow object.
    ...(placeholders.length && !real.length ? { additionalProperties: true } : {}),
  };
  const mediaType = usesFormData(ep.body) ? 'multipart/form-data' : 'application/json';
  return { required: required.length > 0, content: { [mediaType]: { schema } } };
}

// An SSE frame opens with either a `data:` line (no named event) or an
// `event:` line naming the kind before its `data:` line — checking only for
// a leading `data:` misses the second shape (e.g.
// POST /assistant/sessions/{id}/messages, whose frames are `event: <kind>`
// then `data: {...}`).
function isSseExample(responseExample) {
  return /^(data|event):/.test(responseExample.trimStart());
}

// Every documented endpoint has exactly one example response today (no status
// code is modeled in the source data), so it is always keyed 200 — an SSE
// stream included, since streaming does not change the HTTP status.
function responsesFor(ep) {
  if (!ep.responseExample) return { 200: { description: 'Success' } };
  const isSse = isSseExample(ep.responseExample);
  const mediaType = isSse ? 'text/event-stream' : 'application/json';
  return {
    200: {
      description: 'Success',
      content: { [mediaType]: { schema: { type: 'string' }, example: ep.responseExample } },
    },
  };
}

function operationId(method, path) {
  const cleaned = path.replace(/[{}]/g, '').replace(/[^a-zA-Z0-9]+/g, '_').replace(/^_|_$/g, '');
  return `${method.toLowerCase()}_${cleaned}`;
}

function operationFor(ep) {
  const placeholderNotes = [...(ep.query ?? []), ...(ep.body ?? [])].filter(isPlaceholderParam).map(placeholderNote);

  const descriptionParts = [];
  if (ep.description) descriptionParts.push(ep.description);
  if (ep.filterable) descriptionParts.push('Plus every filter in the "Filtering jobs" section above.');
  for (const note of placeholderNotes) descriptionParts.push(note);
  if (ep.deprecated) {
    descriptionParts.push(`**Deprecated** since ${ep.deprecated.since} — use \`${ep.deprecated.replacement}\` instead.`);
  }

  const parameters = [
    ...(ep.pathParams ?? []).map((p) => toParameter(p, 'path')),
    ...(ep.query ?? []).filter((p) => !isPlaceholderParam(p)).map((p) => toParameter(p, 'query')),
  ];

  const op = {
    operationId: operationId(ep.method, ep.path),
    summary: ep.summary,
    ...(descriptionParts.length ? { description: descriptionParts.join('\n\n') } : {}),
    ...(parameters.length ? { parameters } : {}),
    ...(ep.body?.length ? { requestBody: requestBodyFor(ep) } : {}),
    responses: responsesFor(ep),
    security: securityFor(ep.auth),
    ...(ep.deprecated ? { deprecated: true } : {}),
  };
  return op;
}

// A "Param | Filter | Values" Markdown table — the shape both filter tables
// below share.
function filterTableLines(rows) {
  const lines = ['| Param | Filter | Values |', '| --- | --- | --- |'];
  for (const f of rows) lines.push(`| \`${f.param}\` | ${f.label} | ${f.values} |`);
  return lines;
}

// Overview + Filtering content has no natural per-operation home (it is
// cross-cutting: the response envelope, the global pagination rule, the error
// table, the auth model, and the filter vocabulary apply across many
// endpoints). It becomes Markdown in info.description instead — Scalar builds
// a navigable sidebar and search index from its headings.
function buildDescription(overview, filters) {
  const { FILTER_FACETS, FILTER_EXTRAS, FILTER_MODIFIERS, RECIPES } = filters;
  const out = [];
  for (const section of overview) {
    out.push(`## ${section.title}`);
    out.push('');
    for (const p of section.paragraphs) {
      out.push(p);
      out.push('');
    }
    if (section.code) {
      out.push('```json');
      out.push(section.code);
      out.push('```');
      out.push('');
    }
  }

  out.push('## Filtering jobs');
  out.push('');
  out.push(
    'These parameters apply to `GET /jobs/search` and `GET /jobs/facets`. Combine ' +
      'any of them with full-text `q`.',
  );
  out.push('');
  for (const m of FILTER_MODIFIERS) out.push(`- ${m}`);
  out.push('');
  out.push('### Facets');
  out.push('');
  out.push(...filterTableLines(FILTER_FACETS));
  out.push('');
  out.push('### Numeric & boolean filters');
  out.push('');
  out.push(...filterTableLines(FILTER_EXTRAS));
  out.push('');
  out.push('### Recipes');
  out.push('');
  for (const r of RECIPES) out.push(`- **${r.title}** — \`${r.query}\``);

  return out.join('\n').trimEnd();
}

// Pure: (spec, filters) -> OpenAPI document object. Deterministic, no IO.
export function renderOpenApi(spec, filters) {
  const { BASE_URL, OVERVIEW, GROUPS } = spec;

  const paths = {};
  const tags = [];
  for (const group of GROUPS) {
    tags.push({ name: group.title, description: group.intro });
    for (const ep of group.endpoints) {
      paths[ep.path] ??= {};
      paths[ep.path][ep.method.toLowerCase()] = { ...operationFor(ep), tags: [group.title] };
    }
  }

  return {
    openapi: '3.1.0',
    info: {
      title: 'freehire API',
      description: buildDescription(OVERVIEW, filters),
      version: '1.0.0',
    },
    servers: [{ url: BASE_URL }],
    tags,
    paths,
    components: { securitySchemes: SECURITY_SCHEMES },
  };
}

async function main() {
  const { spec, filters } = await loadDocsModules();
  const doc = renderOpenApi(spec, filters);
  const json = `${JSON.stringify(doc, null, 2)}\n`;
  await writeFile(outFile, json);
  console.log(`Wrote ${outFile} (${json.length} bytes)`);
}

if (process.argv[1] && resolve(process.argv[1]) === fileURLToPath(import.meta.url)) {
  main().catch((err) => {
    console.error(err);
    process.exit(1);
  });
}
