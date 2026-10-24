// Field operators typed into the header search box: `title:`, `company:`, `source:`.
//
// They are a typing shortcut for filters the feed already has, so they are translated
// in the box and never reach the URL as text — `company:Google` becomes the
// `company_slug` facet, the same one a picked suggestion sets. One spelling of a filter
// in the URL is what keeps saved searches and digests reading it the same way.
//
// Pure by design: no Svelte, no DOM, no network.

import { quoteForTitleSearch, type ApplyPlan } from './apiSuggestions';
import { sourceLabel, type FacetOption } from './facets';
import { namedCompanies } from './dropdownRows';
import type { ApiSuggestion, CompanyListItem } from './types';

const OPERATOR_KEYS = ['title', 'company', 'source'] as const;
type OperatorKey = (typeof OPERATOR_KEYS)[number];

/** The typed text split into free text and each operator's values. */
interface ParsedOperators {
  text: string;
  title: string[];
  company: string[];
  source: string[];
}

/** One row of the contextual dropdown inside an operator's value. `value` is what
 *  picking it writes into the box; `id` keeps rows apart (a slug or a source key). */
export interface OperatorOption {
  key: OperatorKey;
  value: string;
  label: string;
  id: string;
  count?: number;
}

/** One operator as typed, with where it sits in the text. `start` is the first
 *  character of the key, `end` one past the value (closing quote included). */
export interface OperatorToken {
  key: OperatorKey;
  value: string;
  start: number;
  end: number;
}

// A key only counts at the start of the text or after whitespace, so `node.js:` or a
// URL's `https:` never reads as one. The value is a "quoted phrase" — an unclosed quote
// runs to the end, which is what the box holds mid-typing — or a run of non-space.
const OPERATOR = /(^|\s)(title|company|source):(?:"([^"]*)"?|(\S*))/gi;

function tokens(text: string): OperatorToken[] {
  const out: OperatorToken[] = [];
  for (const m of text.matchAll(OPERATOR)) {
    const [whole, lead = '', key = '', quoted, bare] = m;
    out.push({
      key: key.toLowerCase() as OperatorKey,
      value: (quoted ?? bare ?? '').trim(),
      start: m.index + lead.length,
      end: m.index + whole.length,
    });
  }
  return out;
}

/** Split the text into free text and operator values. An empty value is dropped: a
 *  bare `company:` names nobody. */
export function parseOperators(text: string): ParsedOperators {
  const parsed: ParsedOperators = { text: '', title: [], company: [], source: [] };
  let rest = '';
  let at = 0;
  for (const t of tokens(text)) {
    rest += text.slice(at, t.start) + ' ';
    at = t.end;
    if (t.value !== '') parsed[t.key].push(t.value);
  }
  rest += text.slice(at);
  parsed.text = rest.replace(/\s+/g, ' ').trim();
  return parsed;
}

/** True when the text carries at least one operator, empty or not. */
export function hasOperators(text: string): boolean {
  return tokens(text).length > 0;
}

/** The operator whose VALUE the caret is in — from right after the colon to the end
 *  of the value. The key itself does not count: the visitor is still writing it. */
export function activeOperator(text: string, caret: number): OperatorToken | null {
  for (const t of tokens(text)) {
    const valueStart = t.start + t.key.length + 1;
    if (caret >= valueStart && caret <= t.end) return t;
  }
  return null;
}

/** Spell one operator: quoted when the value has a space, since an unquoted space
 *  would end it. A `"` inside the value is dropped — it is the delimiter. */
function formatOperator(key: OperatorKey, value: string): string {
  const v = value.replaceAll('"', '');
  return /\s/.test(v) ? `${key}:"${v}"` : `${key}:${v}`;
}

/** Replace a token with its picked value and a trailing space, so the visitor can keep
 *  typing. Returns the caret position just past that space. */
export function replaceToken(
  text: string,
  token: OperatorToken,
  value: string,
): { text: string; caret: number } {
  const head = text.slice(0, token.start) + formatOperator(token.key, value) + ' ';
  return { text: head + text.slice(token.end).trimStart(), caret: head.length };
}

/** Turns a typed value into the facet value it names, or null when nothing does. */
export interface OperatorResolvers {
  company(value: string): string | null;
  source(value: string): string | null;
}

/** The first operator whose value names nothing, or null when every one resolves.
 *  Committing stops here rather than applying the rest: a half-applied query is a
 *  feed filtered by something other than what was typed, and the box showing the one
 *  token that still needs fixing is the clearer answer. An empty value is skipped —
 *  it names nothing to resolve. */
export function unresolvedToken(text: string, resolve: OperatorResolvers): OperatorToken | null {
  for (const t of tokens(text)) {
    if (t.key === 'title' || t.value === '') continue;
    if (resolve[t.key](t.value) === null) return t;
  }
  return null;
}

/** What committing the parsed text does, once every value resolves.
 *
 *  A `title:` makes the WHOLE query title-only. Meilisearch applies
 *  `attributesToSearchOn` to the entire query, so "this word in titles, that word
 *  anywhere" is not one request; folding the free text into the title phrase is the
 *  plan a picked title suggestion already produces. */
export function operatorPlan(parsed: ParsedOperators, resolve: OperatorResolvers): ApplyPlan {
  const plan: ApplyPlan = { facets: [] };
  const facets = [
    ['company', 'company_slug'],
    ['source', 'source'],
  ] as const;
  for (const [key, param] of facets) {
    for (const value of parsed[key]) {
      const resolved = resolve[key](value);
      if (resolved) plan.facets.push([param, resolved]);
    }
  }
  if (parsed.title.length) {
    plan.q = quoteForTitleSearch([...parsed.title, parsed.text].join(' ').trim());
    plan.qFields = ['title'];
  } else {
    plan.q = parsed.text;
  }
  return plan;
}

/** The source key a typed value names: the key itself or its display name, ignoring
 *  case. Matched against the live distribution, so it only ever names a source the
 *  catalogue carries. */
export function matchSource(value: string, options: readonly FacetOption[]): string | null {
  const v = value.toLowerCase();
  const hit = options.find(
    (o) => o.value.toLowerCase() === v || sourceLabel(o.value).toLowerCase() === v,
  );
  return hit?.value ?? null;
}

/** The slug of the company a typed value names EXACTLY — its name or its slug, ignoring
 *  case. Not a prefix: a near miss would filter the feed to somebody else's postings,
 *  and the contextual dropdown is where a partial name gets completed. The slug rule
 *  itself lives in Go (normalize.CompanySlug) and is not copied here; the companies
 *  endpoint supplies the candidates. */
export function matchCompany(value: string, items: readonly CompanyListItem[]): string | null {
  const v = value.toLowerCase();
  const hit = items.find((c) => c.name.toLowerCase() === v || c.slug === v);
  return hit?.slug ?? null;
}

/** Company rows for `company:<value>`: named by prefix first, the precision rule the
 *  ordinary Companies section uses. When no name starts with the value, the endpoint's
 *  own fuzzy order is offered instead — unlike that section, here the visitor has SAID
 *  they are picking a company, so `company:"Google Inc."` showing Google is the help
 *  they need, not a stray row. An empty value lists what the endpoint returns for no
 *  query, so `company:` alone already offers some. */
export function companyOptions(
  items: readonly CompanyListItem[],
  value: string,
  limit: number,
): OperatorOption[] {
  // Busiest first among the names that match: `goo` means Google far more often than
  // a two-posting namesake, and the endpoint's order is relevance, not size.
  const named = namedCompanies(items, value, items.length).sort((a, b) => b.job_count - a.job_count);
  const picked = named.length ? named.slice(0, limit) : items.slice(0, limit);
  return picked.map((c) => ({
    key: 'company',
    value: c.name,
    label: c.name,
    id: c.slug,
    count: c.job_count,
  }));
}

/** Source rows for `source:<value>`, busiest first. Picking writes the KEY — stable,
 *  spaceless, and what `matchSource` resolves without a lookup. */
export function sourceOptions(
  dist: readonly FacetOption[],
  value: string,
  limit: number,
): OperatorOption[] {
  const v = value.trim().toLowerCase();
  return dist
    .filter((o) => o.value.toLowerCase().startsWith(v) || sourceLabel(o.value).toLowerCase().startsWith(v))
    .sort((a, b) => (b.count ?? 0) - (a.count ?? 0))
    .slice(0, limit)
    .map((o) => ({ key: 'source', value: o.value, label: sourceLabel(o.value), id: o.value, count: o.count }));
}

/** Title rows for `title:<value>`: the suggestion endpoint's completions that end in a
 *  title. A row completing to a skill or a category is a facet, not a title, and
 *  writing its text after `title:` would search titles for a word the row did not mean. */
export function titleOptions(rows: readonly ApiSuggestion[]): OperatorOption[] {
  return rows
    .filter((r) => r.parts.at(-1)?.kind === 'title')
    // The index is in the id because two rows CAN share a text, and a duplicate {#each}
    // key takes the block down — the same guard `fromApi` keeps.
    .map((r, i) => ({ key: 'title', value: r.text, label: r.text, id: `${i}:${r.text}`, count: r.jobs }));
}
