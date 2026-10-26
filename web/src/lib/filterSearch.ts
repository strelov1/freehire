import {
  COUNTRY_OPTIONS,
  EMPLOYER_CREDENTIALS,
  FACETS,
  JOB_COLLECTION,
  SOURCE_LABELS,
  type FacetDef,
  type FacetOption,
} from './facets';
import { CATEGORY_GROUPS, RAIL, type RailGroup } from './filterSections';
import { EXPERIENCE_PRESETS, FRESHNESS_PRESETS } from './filterControls';
import { fuzzyMatch } from './fuzzy';
import { SKILL_LABELS } from './generated/contracts';

export const FILTER_SEARCH_MIN_QUERY = 2;
export const FILTER_SEARCH_LIMIT = 48;

export interface FilterSearchHit {
  id: string;
  label: string;
  group: string;
  railKey: string;
  railLabel: string;
  param?: string;
  value?: string;
  aliases?: readonly string[];
  action?: string;
}

function railLabel(key: string): string {
  return RAIL.find((e) => e.key === key)?.label ?? key;
}

function optionHit(
  railKey: string,
  param: string,
  group: string,
  opt: Pick<FacetOption, 'value' | 'label' | 'group'>,
  aliases?: readonly string[],
): FilterSearchHit {
  return {
    id: `${railKey}:${param}:${opt.value}`,
    label: opt.label,
    group: opt.group ?? group,
    railKey,
    railLabel: railLabel(railKey),
    param,
    value: opt.value,
    aliases,
  };
}

function jumpHit(railKey: string, label: string, group = label, aliases?: readonly string[]): FilterSearchHit {
  return { id: `jump:${railKey}:${label}`, label, group, railKey, railLabel: railLabel(railKey), aliases };
}

function actionHit(railKey: string, group: string, label: string, action: string): FilterSearchHit {
  return { id: `action:${action}`, label, group, railKey, railLabel: railLabel(railKey), action };
}

function facetOptions(param: string): { def: FacetDef | undefined; options: FacetOption[] } {
  const def = FACETS.find((d) => d.param === param);
  return { def, options: def?.options ?? [] };
}

function pushFacet(hits: FilterSearchHit[], railKey: string, param: string, options?: FacetOption[], group?: string) {
  const found = facetOptions(param);
  const list = options ?? found.options;
  const heading = group ?? found.def?.label ?? railLabel(railKey);
  if (list.length === 0) {
    if (found.def) hits.push(jumpHit(railKey, found.def.label, heading));
    return;
  }
  for (const opt of list) {
    hits.push(optionHit(railKey, param, heading, opt, found.def?.searchAliases?.[opt.value]));
  }
}

function ageLabel(kind: 'Posted' | 'Open', label: string): string {
  return label === 'Today' ? `${kind} today` : `${kind} within ${label.toLowerCase()}`;
}

let jobCatalog: FilterSearchHit[] | null = null;

function buildJobCatalog(): FilterSearchHit[] {
  const hits: FilterSearchHit[] = [];
  for (const entry of RAIL) {
    hits.push(
      jumpHit(
        entry.key,
        entry.label,
        entry.label,
        entry.key === 'category' ? ['specialization', 'specializations', 'category', 'role', 'roles'] : undefined,
      ),
    );
  }

  const categoryAliases = FACETS.find((d) => d.param === 'category')?.searchAliases;
  for (const section of CATEGORY_GROUPS) {
    for (const opt of section.options) {
      hits.push(optionHit('category', 'category', section.name, opt, categoryAliases?.[opt.value]));
    }
  }

  pushFacet(hits, 'category', 'ai_archetype');
  pushFacet(hits, 'experience', 'seniority');
  pushFacet(hits, 'experience', 'role_type');
  for (const preset of EXPERIENCE_PRESETS) {
    if (preset.years == null) continue;
    hits.push(actionHit('experience', 'Years of experience', preset.label, `experience:${preset.years}`));
  }
  for (const preset of FRESHNESS_PRESETS) {
    if (preset.days == null) continue;
    hits.push(actionHit('posted', 'Posted within', ageLabel('Posted', preset.label), `posted:${preset.days}`));
    hits.push(actionHit('posted', 'Open within', ageLabel('Open', preset.label), `open:${preset.days}`));
  }
  pushFacet(hits, 'posted', 'reality');
  pushFacet(hits, 'location', 'regions');
  pushFacet(hits, 'location', 'countries', COUNTRY_OPTIONS, 'Countries');
  hits.push(jumpHit('location', 'City', 'City'));
  pushFacet(hits, 'work', 'work_mode');
  pushFacet(hits, 'work', 'employment_type');
  for (const [value, label] of Object.entries(SKILL_LABELS)) {
    hits.push(optionHit('skills', 'skills', 'Skills', { value, label }));
  }
  pushFacet(hits, 'industry', 'domains');
  pushFacet(hits, 'industry', 'company_type');
  pushFacet(hits, 'industry', 'collections', JOB_COLLECTION, 'Collection');
  for (const [value, label] of Object.entries(SOURCE_LABELS)) {
    hits.push(optionHit('source', 'source', 'Source', { value, label }));
  }
  pushFacet(hits, 'salary', 'salary_currency');
  pushFacet(hits, 'language', 'english_level');
  hits.push(jumpHit('language', 'Job language', 'Job language'));
  pushFacet(hits, 'relocation', 'relocation');
  pushFacet(hits, 'relocation', 'collections', EMPLOYER_CREDENTIALS, 'Employer credentials');
  hits.push(actionHit('relocation', 'Visa', 'Offers visa sponsorship', 'visa'));
  hits.push(
    actionHit('relocation', 'AI interviews', 'Hide employers reported to interview with AI', 'hide-ai'),
  );
  hits.push(actionHit('relocation', 'Auto-apply', 'Auto-apply available', 'auto-apply'));
  hits.push(actionHit('relocation', 'Security clearance', 'Security clearance: Hide', 'clearance:hide'));
  hits.push(actionHit('relocation', 'Security clearance', 'Security clearance: Only', 'clearance:only'));
  return hits;
}

export function jobFilterSearchItems(opts?: {
  exclude?: readonly string[];
  railKeys?: readonly string[];
  openWithin?: boolean;
}): FilterSearchHit[] {
  jobCatalog ??= buildJobCatalog();
  const exclude = new Set(opts?.exclude ?? []);
  const rails = opts?.railKeys ? new Set(opts.railKeys) : null;
  return jobCatalog.filter((hit) => {
    if (rails && !rails.has(hit.railKey)) return false;
    if (hit.param && exclude.has(hit.param)) return false;
    if (!opts?.openWithin && hit.action?.startsWith('open:')) return false;
    return true;
  });
}

export function railFacetSearchItems(
  facets: readonly FacetDef[],
  groups: readonly RailGroup[],
  opts?: { skills?: boolean },
): FilterSearchHit[] {
  const hits: FilterSearchHit[] = [];
  for (const group of groups) {
    hits.push({
      id: `jump:${group.key}`,
      label: group.label,
      group: group.label,
      railKey: group.key,
      railLabel: group.label,
    });
    for (const param of group.params) {
      const def = facets.find((d) => d.param === param);
      if (!def) continue;
      if (param === 'skills' && opts?.skills) {
        for (const [value, label] of Object.entries(SKILL_LABELS)) {
          hits.push({
            id: `${group.key}:${param}:${value}`,
            label,
            group: def.label,
            railKey: group.key,
            railLabel: group.label,
            param,
            value,
          });
        }
        continue;
      }
      const options = def.options ?? [];
      if (options.length === 0) {
        hits.push({
          id: `jump:${group.key}:${param}`,
          label: def.label,
          group: def.label,
          railKey: group.key,
          railLabel: group.label,
        });
        continue;
      }
      for (const opt of options) {
        hits.push({
          id: `${group.key}:${param}:${opt.value}`,
          label: opt.label,
          group: opt.group ?? def.label,
          railKey: group.key,
          railLabel: group.label,
          param,
          value: opt.value,
          aliases: def.searchAliases?.[opt.value],
        });
      }
    }
  }
  return hits;
}

export function matchFilterSearch(
  items: readonly FilterSearchHit[],
  query: string,
  limit = FILTER_SEARCH_LIMIT,
): FilterSearchHit[] {
  const q = query.trim().toLowerCase();
  if (q.length < FILTER_SEARCH_MIN_QUERY) return [];
  const allowTypo = q.length > 2 || /\s/.test(q);
  const scored: { hit: FilterSearchHit; rank: number; index: number }[] = [];
  const groupOnly = new Map<string, number>();
  const groupFuzzy = new Map<string, boolean>();
  for (let index = 0; index < items.length; index++) {
    const hit = items[index];
    if (!hit) continue;
    const label = hit.label.toLowerCase();
    let rank: number | null = null;
    if (label.includes(q)) rank = 0;
    else if (hit.aliases?.some((alias) => alias.toLowerCase().includes(q))) rank = 1;
    else if (allowTypo && fuzzyMatch(hit.label, q)) rank = 2;
    else if (allowTypo && hit.aliases?.some((alias) => fuzzyMatch(alias, q))) rank = 2;
    else if (label !== hit.group.toLowerCase()) {
      const seen = groupOnly.get(hit.group) ?? 0;
      if (seen >= 8) continue;
      let matched = groupFuzzy.get(hit.group);
      if (matched == null) {
        matched = fuzzyMatch(hit.group, q);
        groupFuzzy.set(hit.group, matched);
      }
      if (!matched) continue;
      rank = 3;
    }
    if (rank == null) continue;
    if (rank === 3) groupOnly.set(hit.group, (groupOnly.get(hit.group) ?? 0) + 1);
    scored.push({ hit, rank, index });
  }
  scored.sort((a, b) => a.rank - b.rank || a.index - b.index);
  return scored.slice(0, limit).map((row) => row.hit);
}
