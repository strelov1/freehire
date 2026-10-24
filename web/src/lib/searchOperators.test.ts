import { describe, expect, it } from 'vitest';
import { sourceLabel } from './facets';
import {
  activeOperator,
  companyOptions,
  sourceOptions,
  titleOptions,
  matchCompany,
  matchSource,
  operatorPlan,
  parseOperators,
  replaceToken,
  unresolvedToken,
} from './searchOperators';
import type { ApiSuggestion, CompanyListItem } from './types';

const known: Record<string, Record<string, string>> = {
  company: { google: 'google' },
  source: { greenhouse: 'greenhouse' },
};
const resolve = {
  company: (v: string) => known.company?.[v.toLowerCase()] ?? null,
  source: (v: string) => known.source?.[v.toLowerCase()] ?? null,
};

describe('parseOperators', () => {
  it('leaves text with no operator alone', () => {
    expect(parseOperators('senior rust')).toEqual({
      text: 'senior rust',
      title: [],
      company: [],
      source: [],
    });
  });

  it('pulls each operator out of the free text', () => {
    expect(parseOperators('company:Google source:greenhouse rust')).toEqual({
      text: 'rust',
      title: [],
      company: ['Google'],
      source: ['greenhouse'],
    });
  });

  it('reads a quoted phrase as one value', () => {
    expect(parseOperators('company:"Goldman Sachs" java').company).toEqual(['Goldman Sachs']);
  });

  it('runs an unclosed quote to the end of the text', () => {
    const p = parseOperators('java title:"Backend Eng');
    expect(p.title).toEqual(['Backend Eng']);
    expect(p.text).toBe('java');
  });

  it('matches the key case-insensitively', () => {
    expect(parseOperators('Company:Acme').company).toEqual(['Acme']);
  });

  it('treats any other word-colon as text', () => {
    expect(parseOperators('c++: senior').text).toBe('c++: senior');
    expect(parseOperators('node.js:backend').text).toBe('node.js:backend');
    expect(parseOperators('https://example.com').text).toBe('https://example.com');
  });

  it('does not read a key glued to a preceding word', () => {
    expect(parseOperators('xcompany:acme').company).toEqual([]);
  });

  it('drops an empty value', () => {
    expect(parseOperators('company: java')).toEqual({
      text: 'java',
      title: [],
      company: [],
      source: [],
    });
    expect(parseOperators('company:"" java').company).toEqual([]);
  });

  it('keeps every value of a repeated key', () => {
    expect(parseOperators('company:a company:b').company).toEqual(['a', 'b']);
  });
});

describe('activeOperator', () => {
  it('finds the operator the caret is inside', () => {
    const text = 'java company:goo';
    expect(activeOperator(text, text.length)).toEqual({
      key: 'company',
      value: 'goo',
      start: 5,
      end: text.length,
    });
  });

  it('counts the caret right after the colon as inside', () => {
    expect(activeOperator('source:', 7)).toEqual({ key: 'source', value: '', start: 0, end: 7 });
  });

  it('is inside a quoted value with spaces', () => {
    const text = 'company:"Goldman Sa';
    expect(activeOperator(text, text.length)?.value).toBe('Goldman Sa');
  });

  it('is null when the caret is in plain text', () => {
    expect(activeOperator('company:acme java', 17)).toBeNull();
    expect(activeOperator('java', 4)).toBeNull();
  });

  it('is null in the key itself, before the colon', () => {
    expect(activeOperator('company:acme', 3)).toBeNull();
  });
});

function tokenAt(text: string, caret: number) {
  const token = activeOperator(text, caret);
  if (!token) throw new Error(`no operator at ${caret} in ${text}`);
  return token;
}

describe('replaceToken', () => {
  it('writes the picked value back and moves the caret past it', () => {
    const text = 'java company:goo';
    const token = tokenAt(text, text.length);
    expect(replaceToken(text, token, 'Google')).toEqual({
      text: 'java company:Google ',
      caret: 20,
    });
  });

  it('quotes a value with a space and keeps the text after the token', () => {
    const text = 'company:gold rust';
    const token = tokenAt(text, 12);
    const out = replaceToken(text, token, 'Goldman Sachs');
    expect(out.text).toBe('company:"Goldman Sachs" rust');
    expect(out.caret).toBe('company:"Goldman Sachs" '.length);
  });
});

describe('operatorPlan', () => {
  it('turns resolved operators into facets and the rest into q', () => {
    expect(operatorPlan(parseOperators('company:Google source:greenhouse rust'), resolve)).toEqual({
      facets: [['company_slug', 'google'], ['source', 'greenhouse']],
      q: 'rust',
    });
  });

  it('makes the whole query title-only when title: is present', () => {
    expect(operatorPlan(parseOperators('title:"Backend Engineer" rust'), resolve)).toEqual({
      facets: [],
      q: '"Backend Engineer rust"',
      qFields: ['title'],
    });
  });
});

describe('unresolvedToken', () => {
  it('names the first value that resolves to nothing', () => {
    const text = 'company:Google company:Nosuchco java';
    expect(unresolvedToken(text, resolve)).toEqual({
      key: 'company',
      value: 'Nosuchco',
      start: 15,
      end: 31,
    });
  });

  it('is null when everything resolves, and never blames title: or an empty value', () => {
    expect(unresolvedToken('title:anything company: source:greenhouse', resolve)).toBeNull();
  });
});

describe('matchSource', () => {
  const options = [
    { value: 'greenhouse', label: 'Greenhouse' },
    { value: 'hh', label: 'hh' },
  ];

  it('matches the key or its display label, ignoring case', () => {
    expect(matchSource('GREENHOUSE', options)).toBe('greenhouse');
    expect(matchSource(sourceLabel('hh'), options)).toBe('hh');
  });

  it('is null for a source the catalogue does not carry', () => {
    expect(matchSource('nosuch', options)).toBeNull();
  });
});

describe('matchCompany', () => {
  const items = [
    { slug: 'google', name: 'Google' },
    { slug: 'google-cloud', name: 'Google Cloud' },
  ] as CompanyListItem[];

  it('takes the company whose name or slug equals the value', () => {
    expect(matchCompany('google cloud', items)).toBe('google-cloud');
    expect(matchCompany('GOOGLE', items)).toBe('google');
    expect(matchCompany('google-cloud', items)).toBe('google-cloud');
  });

  it('does not take a mere prefix', () => {
    expect(matchCompany('goog', items)).toBeNull();
  });
});

describe('option builders', () => {
  it('lists companies by name prefix, writing the name and keeping the slug', () => {
    const items = [
      { slug: 'google', name: 'Google', job_count: 40 },
      { slug: 'hooli', name: 'Hooli googlers', job_count: 3 },
    ] as CompanyListItem[];
    expect(companyOptions(items, 'goo', 5)).toEqual([
      { key: 'company', value: 'Google', label: 'Google', id: 'google', count: 40 },
    ]);
  });

  it('puts the busiest matching company first', () => {
    const items = [
      { slug: 'goo-res', name: 'Goo Resultados', job_count: 2 },
      { slug: 'google', name: 'Google', job_count: 2678 },
    ] as CompanyListItem[];
    expect(companyOptions(items, 'goo', 5).map((o) => o.id)).toEqual(['google', 'goo-res']);
  });

  it('falls back to the endpoint order when no name starts with the value', () => {
    const items = [{ slug: 'google', name: 'Google', job_count: 40 }] as CompanyListItem[];
    expect(companyOptions(items, 'Google Inc.', 5).map((o) => o.id)).toEqual(['google']);
  });

  it('lists the busiest companies for an empty value', () => {
    const items = [{ slug: 'a', name: 'A', job_count: 1 }] as CompanyListItem[];
    expect(companyOptions(items, '', 5).map((o) => o.id)).toEqual(['a']);
  });

  it('lists sources by key or label prefix, busiest first, writing the key', () => {
    const dist = [
      { value: 'greenhouse', label: 'greenhouse', count: 10 },
      { value: 'grnet', label: 'grnet', count: 50 },
      { value: 'lever', label: 'lever', count: 99 },
    ];
    expect(sourceOptions(dist, 'gr', 5).map((o) => o.value)).toEqual(['grnet', 'greenhouse']);
    expect(sourceOptions(dist, '', 5).map((o) => o.value)).toEqual(['lever', 'grnet', 'greenhouse']);
    expect(sourceOptions(dist, 'gr', 5)[1]?.label).toBe(sourceLabel('greenhouse'));
  });

  it('keeps only the title completions', () => {
    const rows = [
      { text: 'Backend Engineer', jobs: 7, parts: [{ kind: 'title', text: 'Backend Engineer' }] },
      { text: 'Backend', jobs: 9, parts: [{ kind: 'category', slug: 'backend', text: 'Backend' }] },
    ] as ApiSuggestion[];
    expect(titleOptions(rows)).toEqual([
      { key: 'title', value: 'Backend Engineer', label: 'Backend Engineer', id: '0:Backend Engineer', count: 7 },
    ]);
  });
});
