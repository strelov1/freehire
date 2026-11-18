// web/static/openapi.yaml: the narrow public schema served at freehire.me/openapi.yaml, which
// the freehire custom GPT imports as an Action (docs/chatgpt-actions.md). Built by
// gen-api-docs.mjs from the same merged contract as the API reference, cut down to the
// public read operations below and written as OpenAPI 3.0.3, the version the file has
// always been served in.
//
// Three things the contract does not hold come from elsewhere:
//   - the filter params, from filters.ts (the contract reads them from the raw query on
//     purpose, see typespec/search.tsp), declared here because an Action sends only the
//     params its schema names;
//   - the `data` of the operations that pass a job through as `unknown`, from the
//     typespec/job-wire.tsp models (DATA_SCHEMAS);
//   - the info prose, chatgpt-openapi-info.md.

// The operations in the schema, by contract operationId, mapped to the operationId the
// file publishes. The GPT's instructions and the live GPT call them by these names.
const OPERATIONS = {
  searchJobs: 'searchJobs',
  agentSearchJobs: 'agentSearchJobs',
  ojcpSearchJobs: 'ojcpSearchJobs',
  ojcpJobDetail: 'ojcpJobDetail',
  ojcpEmployerContext: 'ojcpEmployerContext',
  jobFacets: 'getJobFacets',
  suggest: 'getSuggestions',
  getJob: 'getJob',
  similarJobs: 'getSimilarJobs',
  listCompanies: 'searchCompanies',
  getCompany: 'getCompany',
  searchCities: 'searchCities',
};

// Operations reading a filter vocabulary from the raw query: the job search's, or the
// company listing's.
const FILTERED = { searchJobs: 'job', agentSearchJobs: 'job', jobFacets: 'job', listCompanies: 'company' };

// The model under `data` of an operation whose contract leaves it `unknown`.
const DATA_SCHEMAS = {
  searchJobs: 'Job',
  agentSearchJobs: 'Job',
  getJob: 'Job',
  similarJobs: 'Job',
  jobFacets: 'JobFacetCounts',
};

// The TypeSpec routes share Problem as a default response. Name their client errors
// in the public projection without changing generated Go interfaces. newContract
// refuses NUL in any path/query with 400 (contract.go); search/facets also refuse
// invalid queries, and job detail/similar return 404 for missing/private jobs.
const CLIENT_ERRORS = {
  searchJobs: { 400: 'Invalid parameters or search query.' },
  agentSearchJobs: { 400: 'Invalid parameters or search query.' },
  jobFacets: { 400: 'Invalid parameters, unknown facet, or incompatible facet options.' },
  suggest: { 400: 'Invalid parameters (including a NUL byte in the query).' },
  getJob: { 400: 'Invalid parameters.', 404: 'Job not found or not publicly visible.' },
  similarJobs: { 400: 'Invalid parameters.', 404: 'Job not found or not publicly visible.' },
  listCompanies: { 400: 'Invalid parameters or pagination window exceeded.' },
  searchCities: { 400: 'Invalid parameters (including a NUL byte in the query).' },
};

// The geography facets form one OR-group, so they have no `_mode`.
const GEOGRAPHY = new Set(['regions', 'countries', 'cities', 'metros']);

const METHODS = ['get', 'post', 'put', 'patch', 'delete'];

function findOperation(doc, operationId) {
  for (const [path, item] of Object.entries(doc.paths)) {
    for (const method of METHODS)
      if (item[method]?.operationId === operationId) return { path, method, op: item[method] };
  }
  throw new Error(`openapi.yaml: no operation ${operationId} in the contract or the supplement`);
}

const ref = (name) => ({ $ref: `#/components/schemas/${name}` });
const isUntyped = (schema) => schema && Object.keys(schema).every((k) => k === 'description');

// The 200 body with its untyped `data` replaced by the named model.
function typedData(op, doc, model) {
  const media = op.responses['200'].content['application/json'];
  const body = structuredClone(
    media.schema.$ref ? doc.components.schemas[media.schema.$ref.split('/').pop()] : media.schema,
  );
  const data = body.properties.data;
  if (data.type === 'array' && isUntyped(data.items)) data.items = ref(model);
  else if (isUntyped(data)) body.properties.data = ref(model);
  else throw new Error(`openapi.yaml: ${op.operationId} types its data now — drop it from DATA_SCHEMAS`);
  return { ...op.responses['200'], content: { 'application/json': { ...media, schema: body } } };
}

const strings = (options) => ({ type: 'string', ...(options && { enum: options }) });
const repeated = (options) => ({ style: 'form', explode: true, schema: { type: 'array', items: strings(options) } });

// One filters.ts row as a query parameter: repeatable strings unless it names a scalar type.
// A closed vocabulary goes in the enum, so the description keeps only what follows the list.
function rowParameter(f) {
  // The listed values are the enum's leading run (a retired value may follow, unlisted).
  const options = f.options ?? [];
  let listed = options.length;
  while (listed && !f.values.startsWith(options.slice(0, listed).join(', '))) listed--;
  const list = options.slice(0, listed).join(', ');
  const values = list ? f.values.slice(list.length).replace(/^[.\s—-]+/, '') : f.values;
  const description = values ? `${f.label}: ${values}.` : `${f.label}.`;
  return f.type
    ? { name: f.param, in: 'query', description, schema: { type: f.type } }
    : { name: f.param, in: 'query', description, ...repeated(f.options) };
}

// filters.ts -> query parameters. A job facet comes with its `_exclude` twin and, outside
// geography, its `_mode`; the company listing reads its filters bare.
export function filterParameters(filters) {
  const job = [];
  for (const f of filters.FILTER_FACETS) {
    job.push(rowParameter(f), {
      name: `${f.param}_exclude`,
      in: 'query',
      description: `Leaves out postings matching any of these \`${f.param}\` values.`,
      ...repeated(f.options),
    });
    if (!GEOGRAPHY.has(f.param)) {
      job.push({
        name: `${f.param}_mode`,
        in: 'query',
        description: `\`and\` requires every listed \`${f.param}\` value instead of any.`,
        schema: strings(['and']),
      });
    }
  }
  job.push(...filters.FILTER_EXTRAS.map(rowParameter));
  return { job, company: filters.COMPANY_FILTERS.map(rowParameter) };
}

// OpenAPI 3.1 -> 3.0.3 for the constructs the contract emits: a nullable is
// `anyOf: [X, {type: null}]` in 3.1 and `nullable: true` in 3.0. Drops the x-ogen, x-raw-json and
// x-audience extensions, which describe the server, not the API.
function downgrade(node) {
  if (Array.isArray(node)) return node.map(downgrade);
  if (!node || typeof node !== 'object') return node;
  // OpenAPI 3.0 ignores siblings of a reference; keep schema annotations via allOf.
  if (node.$ref && Object.keys(node).length > 1) {
    const { $ref, ...annotations } = node;
    return downgrade({ ...annotations, allOf: [{ $ref }] });
  }
  const { anyOf, ...rest } = node;
  if (anyOf?.length === 2 && anyOf.some((s) => s.type === 'null')) {
    const schema = anyOf.find((s) => s.type !== 'null');
    return downgrade(
      schema.$ref ? { ...rest, allOf: [schema], nullable: true } : { ...rest, ...schema, nullable: true },
    );
  }
  if (node.type === 'null' || Array.isArray(node.type))
    throw new Error('openapi.yaml: a 3.1 type this downgrade does not convert');
  const out = {};
  for (const [k, v] of Object.entries(node))
    if (!k.startsWith('x-ogen') && k !== 'x-audience' && k !== 'x-raw-json') out[k] = downgrade(v);
  return out;
}

// The importer's limits (developers.openai.com/api/docs/actions/production): 300
// characters for an operation's summary or description, 700 for a parameter's.
function checkLimits(paths, shared) {
  for (const item of Object.values(paths)) {
    for (const op of Object.values(item)) {
      for (const field of ['summary', 'description']) {
        if ((op[field] ?? '').length > 300)
          throw new Error(`openapi.yaml: ${op.operationId} ${field} is over 300 characters`);
      }
      for (const p of (op.parameters ?? []).map((q) => (q.$ref ? shared[q.$ref.split('/').pop()] : q))) {
        if ((p.description ?? '').length > 700)
          throw new Error(`openapi.yaml: ${op.operationId} ${p.name} is over 700 characters`);
      }
    }
  }
}

// Every component schema an object reaches through $ref, transitively.
function reachable(root, schemas) {
  const seen = new Set();
  (function walk(node) {
    if (Array.isArray(node)) return node.forEach(walk);
    if (!node || typeof node !== 'object') return;
    const name = node.$ref?.match(/^#\/components\/schemas\/(.+)$/)?.[1];
    if (name && !seen.has(name)) {
      seen.add(name);
      walk(schemas[name]);
    }
    Object.values(node).forEach(walk);
  })(root);
  return seen;
}

// Pure: (merged contract, filters module, info prose) -> the ChatGPT Actions schema.
export function renderChatGptSchema(doc, filters, description) {
  const filterParams = filterParameters(filters);
  const shared = {};
  const paths = {};
  for (const [contractId, publicId] of Object.entries(OPERATIONS)) {
    const { path, method, op } = findOperation(doc, contractId);
    const parameters = (op.parameters ?? []).map((p) =>
      p.$ref ? doc.components.parameters[p.$ref.split('/').pop()] : p,
    );
    if (FILTERED[contractId]) {
      const named = new Set(parameters.map((p) => p.name));
      for (const p of filterParams[FILTERED[contractId]]) {
        if (named.has(p.name)) continue;
        // Shared under components, so the three job operations do not repeat ~110 definitions.
        const key = `${FILTERED[contractId]}.${p.name}`;
        shared[key] = p;
        parameters.push({ $ref: `#/components/parameters/${key}` });
      }
    }
    const responses = { ...op.responses };
    for (const [status, errorDescription] of Object.entries(CLIENT_ERRORS[contractId] ?? {})) {
      responses[status] = { ...op.responses.default, description: errorDescription };
    }
    if (DATA_SCHEMAS[contractId]) responses['200'] = typedData(op, doc, DATA_SCHEMAS[contractId]);
    const { security: _security, ...rest } = op;
    (paths[path] ??= {})[method] = {
      ...rest,
      operationId: publicId,
      ...(parameters.length && { parameters }),
      responses,
      'x-openai-isConsequential': false,
    };
  }
  checkLimits(paths, shared);
  const keep = reachable([paths, shared], doc.components.schemas);
  const used = new Set(Object.values(paths).flatMap((item) => Object.values(item).flatMap((op) => op.tags ?? [])));
  return downgrade({
    openapi: '3.0.3',
    info: {
      title: 'freehire public API',
      version: '1.1.0',
      description,
      license: { name: 'freehire API data licence', url: 'https://freehire.me/terms#data-licence' },
    },
    servers: [{ url: doc.servers[0].url }],
    // Every operation answers without a key; a key only selects the API plan's budget.
    security: [{}, { PersonalApiKey: [] }],
    tags: doc.tags.filter((t) => used.has(t.name)),
    paths,
    components: {
      securitySchemes: { PersonalApiKey: doc.components.securitySchemes.PersonalApiKey },
      parameters: shared,
      schemas: Object.fromEntries(Object.entries(doc.components.schemas).filter(([name]) => keep.has(name))),
    },
  });
}
