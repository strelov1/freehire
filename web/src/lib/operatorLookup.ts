import { api } from '#lib/api.js';
import type { FacetOption } from '#lib/facets.js';
import {
  companyOptions,
  matchCompany,
  matchSource,
  parseOperators,
  sourceOptions,
  titleOptions,
  type OperatorOption,
  type OperatorResolvers,
  type OperatorToken,
} from '#lib/searchOperators.js';
import { loadSourceDistribution } from '#lib/sourceDictionary.js';
import type { CompanyListItem } from '#lib/types.js';

/** The lookups HeaderSearch's `title:`, `company:` and `source:` operators answer from (see
 *  #lib/searchOperators), and what they have already resolved. One per search box. */
export class OperatorLookup {
  // Company names this box has seen resolved to a slug — picked rows and exact matches —
  // so committing a picked `company:Google` does not ask the endpoint again. Keyed by
  // the lower-cased name. Not reactive: it is read at commit time only.
  readonly #knownCompanies = new Map<string, string>();
  // The live source distribution, fetched once, the first time a `source:` appears. A
  // failed fetch is forgotten so the next attempt asks again.
  #sourceDist: Promise<FacetOption[]> | null = null;
  readonly #opts: { companiesFetch: number; optionLimit: number; minChars: number };

  /** `companiesFetch` is how many companies one lookup asks for, `optionLimit` how many
   *  rows an operator offers, `minChars` the shortest title worth completing. */
  constructor(opts: { companiesFetch: number; optionLimit: number; minChars: number }) {
    this.#opts = opts;
  }

  /** Remember a company row the visitor picked, so committing it needs no lookup. */
  rememberCompany(name: string, slug: string) {
    this.#knownCompanies.set(companyKey(name), slug);
  }

  /** The rows for the operator the caret is in; null when nothing was asked (a title too
   *  short to complete). */
  async optionsFor(op: OperatorToken, signal: AbortSignal): Promise<OperatorOption[] | null> {
    const { optionLimit, minChars } = this.#opts;
    if (op.key === 'company') {
      return companyOptions(await this.#companiesNamed(op.value, signal), op.value, optionLimit);
    }
    if (op.key === 'source') return sourceOptions(await this.#sources(), op.value, optionLimit);
    if (op.value.length < minChars) return null;
    return titleOptions(await api.suggest(op.value, optionLimit, signal));
  }

  /** Resolve every operator value in `text`, asking the companies endpoint for the
   *  names this box has not seen. A failed lookup resolves to nothing, which stops the
   *  commit on that token rather than guessing. */
  async resolversFor(text: string): Promise<OperatorResolvers> {
    const parsed = parseOperators(text);
    const unseen = parsed.company.filter((v) => !this.#knownCompanies.has(companyKey(v)));
    const [dist] = await Promise.all([
      parsed.source.length ? this.#sources().catch(() => []) : Promise.resolve([]),
      ...unseen.map((v) => this.#companiesNamed(v).catch(() => [])),
    ]);
    return {
      company: (v) => this.#knownCompanies.get(companyKey(v)) ?? null,
      source: (v) => matchSource(v, dist),
    };
  }

  #sources(): Promise<FacetOption[]> {
    this.#sourceDist ??= loadSourceDistribution().catch((err: unknown) => {
      this.#sourceDist = null;
      throw err;
    });
    return this.#sourceDist;
  }

  async #companiesNamed(value: string, signal?: AbortSignal): Promise<CompanyListItem[]> {
    const { items } = await api.listCompanies(value, this.#opts.companiesFetch, 0, undefined, signal);
    const exact = matchCompany(value, items);
    if (exact) this.#knownCompanies.set(companyKey(value), exact);
    return items;
  }
}

// The cache key for a typed company value: what `formatOperator` writes back into the
// box drops `"`, so the key must too, or a picked name with a quote never hits it.
function companyKey(value: string): string {
  return value.replaceAll('"', '').toLowerCase();
}
